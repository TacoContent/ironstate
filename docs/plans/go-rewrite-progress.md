# Go Rewrite — Implementation Progress / Handoff Notes

This is a working handoff document for whichever agent/machine picks up the Go rewrite
next. It mirrors the plan's phases (see [go-rewrite.md](go-rewrite.md) §10) and records
concrete implementation state, gotchas, and next steps that aren't in the plan itself.
Update this file (checkboxes + notes) as work progresses, and fold anything
plan-level/durable back into `go-rewrite.md` proper.

Module path: `github.com/TacoContent/ironstate`. Go toolchain: developed against 1.24.3
locally; `go.mod`'s `go` directive is currently `1.25.0` (see notes below on why). Repo
root layout is unchanged (`site.yml`, `hosts/`, `packages/`, `roles/`, `variables/`,
`tasks/`).

## Status

- [x] Phase 0 — scaffolding: `go.mod`, `cmd/ironstate`, cobra/viper, CI skeleton, goreleaser
- [x] Phase 1 — `internal/expr`, `internal/template`, `internal/filters` (built-ins)
- [x] Phase 2 — `internal/model`, `internal/packages` (hierarchy/include/env/paths),
      `internal/tasks` (`Expand-TaskTree` port), `internal/facts` (windows/other build
      split). Validated against REAL `site.yml` + `hosts/krayt.yml` + `hosts/camalot` +
      the whole `roles/*`/`packages/*` tree (181 flattened leaves, zero errors) in
      `internal/tasks/realfixture_test.go`.
- [ ] Phase 3 — `internal/engine`: port `ironstate.ps1`'s `Invoke-Tasks`/`Invoke-PackageItem`
      main flow. Needs, in order:
      1. `internal/conditions` (or fold into `internal/expr`) — port `Conditions.psm1`'s
         `Test-WhenClause`/`Test-Condition`. A leaf's `When`/`FailedWhen` are `[]any`
         (string or bool entries), unevaluated. For a bool entry, stringify as Go's own
         lowercase `"true"`/`"false"` (**not** PowerShell's `"True"`/`"False"`) before
         feeding `internal/expr.Parse` — this is the correct/equivalent behavior; see
         the "Gotchas" section below for why.
      2. Registry threading: facts pass first, in document order, then everything else,
         threading one growing id-registry + userFacts + commandAvailability map forward
         — exactly like `ironstate.ps1`'s `Invoke-Tasks`.
      3. `Handler` interface + a handful of low-risk handlers to prove the loop
         end-to-end: `log`, `path`, `fact`, `assert` (`assert` and an embedded-shell
         `fact` are the two dry-run-forces-real-execution exceptions — must not skip
         those under dry-run). Then widen to `file`/`copy`/`blockinfile`/
         `ssh_host_block`/`symlinks`/`zip` per the plan's Phase 3 list.
- [ ] Phase 4 — package-manager handlers (`winget`/`chocolatey`/`pipx`/`npm`/`cargo`/`go`/
      `gem`/`eget`) + `registry`/`scheduled_task` (Windows-only, build-tagged) + `shell`
      (incl. per-state present/absent/latest fallback) + `template` module w/ `jinja`
      (build-vs-buy spike first) + new `gotemplate` engine (Go stdlib `text/template`)
- [ ] Phase 5 — filter plugin system: external-script filter loader + JSON-over-stdio
      shim + persistent worker pool, `filters list`/`doctor` wiring for discovered
      script filters
- [ ] Phase 6 — hardening: full test matrix green, wire `.goreleaser.yaml`'s `sboms:`
      block into an actual `release.yml`, signing (cosign),
      `actions/attest-build-provenance`
- [ ] Phase 7 — cutover

## Gotchas / findings worth knowing before continuing

- **`include: { name: <path> }` resolves to `<repoRoot>/<path>/main.yml`** — ONE unified
  mechanism. `packages/`, `roles/`, `hosts/<sub>/`, `tasks/wsl-shim/` are just naming
  conventions, not distinct code paths. `PackagesRoot` passed to `Expand-TaskTree` is
  `$PSScriptRoot` (repo root), not `packages/`.
- `ironstate.ps1`'s `Get-PackageManagerHandlers` includes `gem` (`RubyGem.psm1`) — but
  `site.schema.json` has NO `gem` entry, and the README's module table omits it too.
  Real usage exists (`roles/languages/ruby/main.yml`). This is a pre-existing drift bug
  in the original repo (schema/docs vs. code) — audit and fix the schema in Phase 1's
  reconciliation task, don't just silently copy the gap into the Go port's docs.
- Template resolution is a **three**-pass system: (1) whole-document `-Soft` pass
  (facts/vars namespaces only) before flattening; (2) per-leaf strict pass during
  dispatch (facts+vars+package+inputs+registry); (3) `Handlers/Template.psm1`'s own
  extra self-referential `Resolve-TemplateRenderContext` pass over the merged render
  context, needed because `vars:` content itself can contain unresolved `${{ }}` that
  neither pass 1 nor 2 ever revisits.
- Dry-run is **not** fully inert: a `fact` leaf with an embedded `shell`, and every
  `assert` leaf, are forced to actually execute even without `-Apply`
  (`ironstate.ps1`'s `Invoke-Tasks`:
  `-Apply:($Apply -or $hasEmbeddedShell -or ($module -eq 'assert'))`).
- A leaf whose package-manager command isn't on PATH, or whose module has no
  registered handler, is silently `continue`d — it doesn't even appear in the results
  table (not marked Skip/Failed), just a warning.
- In the **original PowerShell** `Expressions.psm1`, `<expr> is null` can never actually
  parse — the tokenizer lexes the bare word `null` as the null-literal keyword (never
  as an `ident`), but `Parse-ExpressionMembership` requires an `ident` token after `is`.
  Ported this quirk byte-for-byte in `internal/expr` rather than silently fixing it (see
  `TestIsNullTestNameIsUnreachable...` in `internal/expr/eval_test.go`). Only `is none`
  actually works.
- **Phase 3's 'when'-boolean gotcha**: a YAML `when: true` boolean literal must
  round-trip correctly through the expression grammar. PowerShell gets this to work by
  accident (parameter-binding stringifies `$true` to `"True"`, and PowerShell's `switch`
  keyword matching is case-insensitive, so `"True"` still tokenizes as the `true`
  keyword). Go's tokenizer is intentionally case-sensitive (matches only lowercase
  `true`/`false`/`and`/`or`/`not`/`in`/`is`). The correct fix is **not** to make the
  tokenizer case-insensitive — it's to stringify a Go `bool` as lowercase `"true"`/
  `"false"` (Go's own `fmt`/manual mapping already does this naturally) before handing
  it to `internal/expr.Parse`. Get this right in the Phase 3 Conditions port from the
  start; don't reach for the tokenizer as the fix.
- Found and fixed two real bugs during Phase 1/2 porting, both caught by tests/fuzzing,
  not by inspection:
  1. `internal/expr`'s `ScanSpans` originally computed byte offsets from `[]rune(text)`
     + `utf8.RuneLen(r)` per rune — wrong for invalid UTF-8 input (an invalid byte
     decodes to U+FFFD via `[]rune` conversion, but `utf8.RuneLen(0xFFFD)=3` while the
     original invalid byte only consumed 1 byte), producing out-of-bounds spans. Fixed
     by ranging over the string directly (`for i, r := range text`), which gives
     correct byte offsets regardless of invalid UTF-8. Caught by
     `go test -fuzz FuzzScanSpans`.
  2. `internal/packages`' relative-path resolution originally used `filepath.IsAbs`,
     which on Windows requires a drive letter/volume — but the original PowerShell uses
     `[System.IO.Path]::IsPathRooted`, which returns true for ANY path starting with
     `/` or `\` (no drive letter needed) or a `C:`-style prefix. A Unix-style
     `/already/absolute` src path was being wrongly rewritten to a repo-relative path.
     Fixed with a hand-rolled `isPathRooted()` in `internal/packages/paths.go` matching
     .NET's actual (looser) semantics. Caught by `TestLoadFileResolvesRelativePaths`.
     **Lesson: never assume Go's path stdlib matches .NET's `Path.*` semantics on
     Windows — verify each one used.**
- Filters needing special test treatment (non-deterministic to golden-test against the
  live PowerShell originals): `lookup` (`httpGet`/`readFile` are package vars in
  `internal/filters/lookup.go`, override in tests) and `json_query` (`lookPath`/`runJQ`
  are package vars in `internal/filters/json.go`, override to test both jq-present and
  jq-absent branches deliberately).
- gosec policy actually applied (not just documented): G204 (subprocess w/ variable),
  G304 (file path w/ variable), G401/G505 (sha1), G703 (path check w/ variable) all
  suppressed per-callsite with `//nolint:gosec` + a one-line justification, per
  `go-rewrite.md` §6. golangci-lint v2 requires `version: "2"` at the top of
  `.golangci.yml` or it fails to load entirely.
- `go.mod`'s `go` directive got bumped from `1.24` to `1.25.0` by
  `go get golang.org/x/sys` (needed for `internal/facts_windows.go`'s
  elevation/`RtlGetVersion` check) — expected/fine; Go's toolchain auto-download handles
  it, and GH Actions' `setup-go` with `go-version-file: go.mod` will match it
  automatically.
- Port reference map so far: `Merge-PackagesData` → `packages.MergeDocuments`;
  `Get-TaskList` → `model.TaskList`; `Get-Prop`/duck-typing → `model.Prop`/`PropOr`/
  `AsMap`/`AsList`/`AsStringSlice`; `include:` resolution → `packages.LoadIncludedPackage`
  (returns `(nil, nil)` for a missing/misconfigured include, matching the original's
  warn-and-skip, not an error); task-tree flattening → `tasks.Expand` (returns
  `[]tasks.Leaf`).
- Known, deliberate, documented micro-gaps (commented in code, not silent):
  1. A leaf with >1 recognized module key (already an invalid/warn-worthy document)
     picks the "first" key by `internal/tasks.Options.ModuleNames`'s Go-side ordering,
     not original YAML key order (Go's `map[string]any` has no key order) — see
     `internal/tasks/tasks.go`'s `Options` doc comment.
  2. `internal/packages.LoadIncludedPackage`'s template soft-resolution is a strict
     superset of the original (also handles a bare-list package root, which the
     original PowerShell couldn't reliably handle at all) — see
     `internal/packages/include.go`'s `resolveTemplatesInPlace` comment.

## Working locally

- Build/test loop: `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`.
- Fuzz: `go test ./internal/expr/... -run xxx -fuzz FuzzParse -fuzztime 10s` (also
  `FuzzScanSpans`).
- Lint (no local binary installed; this downloads+builds golangci-lint each time, slow
  but works): `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...`
- `go test -race` does **not** work on a Windows dev machine without a C toolchain
  (CGO_ENABLED=1 required) — only runs in CI (`ubuntu-latest`, see
  `.github/workflows/ci.yml`'s `race` job).

## Repo state

As of this note, Phase 0+1 were committed/pushed by the user on branch `time-to-go`.
Phase 2 (`internal/model`, `internal/packages`, `internal/tasks`, `internal/facts`) was
implemented but had **not yet been committed** — check `git status`/`git log` on
`time-to-go` to see current reality before assuming anything above is uncommitted; don't
commit/push without being asked.

## Next concrete step

Start Phase 3 (`internal/engine`) per the ordered sub-steps above — the 'when'-boolean
gotcha is the one detail most likely to get silently reintroduced if skipped.
