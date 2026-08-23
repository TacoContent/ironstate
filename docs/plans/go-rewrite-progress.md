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
- [x] Phase 3 — `internal/engine`: port `ironstate.ps1`'s `Invoke-Tasks`/`Invoke-PackageItem`
      main flow. `internal/conditions` (Test-WhenClause/Test-Condition, with the
      bool-stringify gotcha handled at the `TestWhen` boundary), `internal/engine`
      (Context/Handler/ExecResult/Result types, registry+userFacts+commandAvailability
      threading via `*State`, facts-first two-phase `Run`, the two
      dry-run-forces-real-execution exceptions for `assert`/embedded-shell `fact`,
      missing-handler/missing-command "no result row" behavior, `FilterByTags`,
      `PrintTable`/`PrintJSON`), `internal/handlers` (`log`, `path` [Windows-only via
      `golang.org/x/sys/windows/registry`, build-tagged; non-Windows stub errors clearly],
      `fact`, `assert`, `file`, `copy`, `symlinks`, `blockinfile`, `ssh_host_block`, `zip`).
      `internal/cli/root.go` wired end-to-end (load hierarchy → soft-resolve → flatten →
      tag-filter → `engine.Run` → table/JSON output) and smoke-tested in dry-run against
      the real `site.yml` (see "Phase 3 smoke test" note below).
- [x] Phase 4 — `internal/exec` (Runner abstraction, mockable via a package-level var);
      package-manager handlers `winget`/`chocolatey`/`pipx`/`npm`/`cargo`/`go`/`gem`/`eget`
      (argv-tested against fake runners); `shell` (per-state present/absent/latest
      field-by-field fallback with `absent` deliberately excluded from the top-level
      fallback, host presets, `creates`-gated idempotency shared with `zip`); `registry`
      (Windows-only, `golang.org/x/sys/windows/registry` directly — HKCR/HKU/HKCC need no
      PSDrive-mounting equivalent in Go, unlike the original); `scheduled_task`
      (Windows-only, generates a Task Scheduler XML definition and shells out to
      `schtasks.exe` — see "Phase 4 deviations" below for the resulting idempotency
      trade-off); `internal/templateengines` (`jinja` natively ported on top of
      `internal/expr`, `gotemplate` via Go stdlib `text/template`); `template` module
      wired to both; `fact`'s embedded `shell` and `blockinfile`'s `template` field now
      delegate to the real `shell`/`template` implementations, closing the two Phase 3
      stand-in gaps. All new code build-tag-clean on both `windows` and (cross-compiled)
      `linux`, full test suite green including a real-HKCU registry round-trip test.
- [x] Phase 5 — `internal/filters`'s script-filter adapter: `embed/shim.ps1` (embedded via
      `go:embed`, extracted to a temp file once per process) speaks a JSON-over-stdio
      protocol (`{"value":...,"args":[...]}` in, `{"result":...}`/`{"error":"..."}` out,
      one line each) to an unmodified target `modules/Filters/*.ps1` file, so every
      existing filter script needs zero changes. `scriptWorker` is a persistent,
      lazily-started interpreter subprocess kept warm for the process lifetime (not
      spawned per call), pooled per filter name by `Pool`. `DiscoverScriptFilters` scans
      a directory and registers each recognized script under its base name — but only
      if `Registry.Has` reports no built-in (or earlier-discovered) filter of that name
      already claims it. `DefaultInterpreters` maps `.ps1 -> pwsh -NoProfile -File`
      (the only shipped interpreter/shim pair today — see "Next concrete step" below for
      the multi-language extension point). `ironstate filters list --dir <dir>` and
      `ironstate doctor --filters-dir <dir>` both report discovered script filters;
      `internal/cli/root.go`'s real run wires discovery in too (`cfg.FiltersDir`/
      `cfg.FilterInterpreters`, resolved relative to the site file's directory). Verified
      against a live `pwsh`-backed round trip (a net-new custom filter dropped in a temp
      dir, actually invoked end-to-end — not just discovery), not just fakes; unit tests
      use an injectable `startWorkerProcess` var (in-memory `io.Pipe` fakes) so the suite
      never depends on `pwsh` being installed.
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

Start Phase 6 (hardening: full test matrix, SBOM/provenance/signing wired into a real
`release.yml`, docs updated). Key things the next agent should know before starting:

- **Phase 5 smoke test**: verified against a *live* `pwsh`-backed round trip, not just
  fakes — a net-new custom filter (`reverse.ps1`, reversing a string) dropped into a
  scratch directory was discovered via `ironstate filters list --dir <dir>` (reported as
  `script`, distinct from the 21 `built-in` names — none of the real
  `modules/Filters/*.ps1` files ever show as `script`, since the Go built-ins always win)
  and then actually invoked end-to-end (`filters.DiscoverScriptFilters` +
  `Registry.Apply("reverse", "hello", nil)` → `"olleh"`), proving the embedded shim +
  persistent worker + JSON protocol all work against a real `pwsh`, not just the
  in-memory-pipe fakes the unit tests use.
- **`internal/filters/script.go`'s `shimPathFor` only knows `.ps1` today** — extracts the
  single embedded `embed/shim.ps1` to a temp file once per process (`sync.Once`). Adding
  a second scripting language (e.g. Python) needs BOTH a new `DefaultInterpreters()`
  entry (extension → interpreter argv prefix) AND a new shim script written in that
  language's own idiom (a `.ps1` shim can't run a `.py` target file) — `shimPathFor`'s
  `switch` is the one place to extend, not `DefaultInterpreters` alone. No real second
  language is needed today (only `.ps1` filters exist in this repo), so this is a
  documented extension point, not a gap to close.
- **`internal/config.Config.FilterInterpreters` is nil unless a user's
  `ironstate.yaml`/`.ironstate.yaml` sets `filters.interpreters`** (via
  `viper.UnmarshalKey` — deliberately not hardcoded into the viper defaults, to keep
  `internal/config` free of an `internal/filters` import). Every caller (`root.go`,
  `filters.go`, `doctor.go`) falls back to `filters.DefaultInterpreters()` when empty —
  keep doing that rather than baking a default into `config.Load` itself.
- **`internal/filters`'s script-filter worker pool is the one place in this whole
  codebase with real concurrency** (docs/plans/go-rewrite.md §4.5/§6) — CI's `race` job
  already scopes `go test ./internal/filters/... -race` (see `.github/workflows/ci.yml`,
  set up back in Phase 0). `-race` does **not** work on a Windows dev machine without a
  C toolchain (`CGO_ENABLED=1` required) — this was re-confirmed this session; don't
  waste time trying to run it locally on Windows, trust CI's `ubuntu-latest` leg instead.
  `TestPoolConcurrentDistinctWorkersDoNotBlockEachOther` in
  `internal/filters/script_test.go` is the test that actually exercises this.
- **Re-run the `eps`/`herestring` and `shell.host: pwsh` native-merge grep audits**
  mentioned in the master plan's §10/§11 as part of Phase 6 sign-off, in case either
  gained real usage since Phase 4 landed — they hadn't as of Phase 4's own re-check.
- A recurring tool quirk hit again this session (Phase 5): creating a **new** file
  sometimes gets the tool to emit a duplicated leading `package foo` line — same
  quirk noted in the Phase 3/4 notes above, not scoped to any particular file-content
  shape. Every new file this session (`internal/filters/script.go`, `pool.go`,
  `embed.go`) was checked immediately after creation and none were affected this time,
  but keep checking every time regardless.
- All Phase 5 code is build-tag-clean on both `windows` (native build) and `linux`
  (cross-compiled via `$env:GOOS="linux"; go build ./...`), full test suite green.

- **Phase 4 deviations (documented, not silent)**:
  1. **`shell`'s `pwsh` host is always a subprocess in Go** (`pwsh -NoLogo -NoProfile
     -File <tempfile> args...`), never the original's in-process execution — Go has no
     way to run PowerShell script content in-process. This means `Merge-ShellNativeResult`
     (merging a captured native .NET object's properties into a shell task's registered
     result, e.g. `${{ pf.ProgramFilesDir }}`) is **not ported** — already audited in the
     master plan (§4.8/§8/§11) as unused in this repo's real YAML today, so this is an
     accepted v1 gap, not a regression to chase.
  2. **`scheduled_task` uses `schtasks.exe` + generated Task Scheduler XML**, not the
     `ScheduledTasks` PowerShell module/CIM — a deliberate choice to keep `shell.host:
     pwsh` the *only* remaining pwsh dependency (per the master plan's §11 framing). The
     real consequence: **idempotency is reduced to existence + enabled-state checking
     only** (`internal/handlers/scheduledtask_windows.go`'s `scheduledTaskHandler` doc
     comment) — unlike the original, a registered task whose actions/triggers/principal/
     settings have drifted from the declared YAML is NOT auto-detected/corrected under
     `state: present`. Use `state: latest` to force re-registration when a task's shape
     may have changed. Only `packages/smartctl/main.yml` uses this module in this repo
     today (verified by grep), so this reduction is low real-world impact.
  3. **`registry` needed no PSDrive-mounting equivalent** — `golang.org/x/sys/windows/
     registry`'s `HKCR`/`HKU`/`HKCC` root-key constants work directly, unlike the original
     PowerShell's `New-PSDrive` requirement for those three hives.
  4. **`jinja` is a complete native port** of `modules/TemplateEngines/Jinja.psm1`'s
     subset (`{{ }}`, `{% if/elif/else/endif %}`, `{% for x in y %}` with nesting,
     `{% set %}`), reusing `internal/expr.Parse`/`Eval`/`Truthy`/`DisplayString` directly
     — no third-party library needed (the "build" side of the plan's §4.7 build-vs-buy
     spike won outright; buying was never attempted). `gotemplate` (Go stdlib
     `text/template`) is additive, per the plan.
  5. `internal/handlers/util.go`'s `runner ironexec.Runner` package var backs every
     package-manager handler's and `shell`'s CLI invocations (`runExternalCommand`
     wraps it, echoing captured stdout/stderr the same way `Invoke-ExternalCommand`
     did) — override this one var in tests (see `recordingRunner` in
     `internal/handlers/packagemanagers_test.go`), not `os/exec` directly.
- **`file`'s hard-link detection is a documented, deliberate gap**: Go's stdlib has no
  portable "is this path a hard link (vs. a ordinary file)" check the way PowerShell's
  `Get-Item .LinkType -eq 'HardLink'` does, so `filePathKind` never actually returns
  `"hard"` — a hard-linked path just looks like a plain `"file"`. `testFileItemPresent`'s
  `type: hard` case still works correctly despite this (it hashes both sides directly
  rather than relying on `filePathKind`), so real behavior is unaffected; only the
  (unused) `"hard"` classification itself is a stub.
- **`engine.Context` carries a `Filters expr.Filters` field** (added while porting
  `assert`, whose `that` conditions can themselves use `| filter(...)`, e.g. the real
  `facts.local_bin_path | length > 0` in `site.yml`; also needed by `template`'s jinja
  rendering) — this is a deviation from the master plan's §4.8 `Handler` interface sketch
  (which didn't show `Context` needing a filter registry at all). Keep this in mind if
  the plan's interface sketch is ever used as a literal checklist — the *implemented*
  `Context` shape is `{ Flat, Filters, Apply }`.
- Every handler (Phase 3 *and* Phase 4) lives in one flat `internal/handlers` package
  (not one subpackage per module as the master plan's §3 layout shows) — documented as a
  deliberate deviation in `internal/handlers/handlers.go`'s package doc comment, since
  most of them share small helpers (file-kind checks, blockinfile markers, the
  `creates`-glob primitive, the `runner`/`runExternalCommand` pair). This held up fine
  through the full package-manager handler count added in Phase 4; reconsider only if a
  later phase makes the single package unwieldy.
- A recurring tool quirk hit repeatedly this session (again, in Phase 4): creating a
  **new** file sometimes gets the tool to emit a duplicated leading `package foo` line
  — sometimes right before a `// Package foo ...` doc comment, but at least once (
  `internal/exec/exec.go`, `internal/templateengines/jinja.go`, a test file) with no
  comment involved at all, so it isn't strictly tied to the doc-comment pattern
  previously assumed. Breaks the build with `syntax error: non-declaration statement
  outside function body`. There is no reliable workaround other than: immediately
  `read_file` the first ~5-10 lines of every file right after `create_file` and fix a
  duplicated `package` line if present, every single time, regardless of what the
  content looks like.
- All Phase 4 code is build-tag-clean on both `windows` (native build) and `linux`
  (cross-compiled via `$env:GOOS="linux"; go build ./...`) — verified before considering
  the phase done, per the working-locally checklist below.
