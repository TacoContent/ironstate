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
- [x] Phase 6 — hardening: `.goreleaser.yaml`'s `signs:` block (keyless cosign, signing
      `checksums.txt` via GitHub Actions OIDC) added alongside the existing `sboms:`
      block; `.github/workflows/release.yml` (tag-triggered, installs `syft`+`cosign`,
      runs `goreleaser release --clean`, then `actions/attest-build-provenance` over
      every archive+checksums file); `.github/dependabot.yml` (gomod + github-actions,
      weekly). Full local lint pass: installed `golangci-lint`/`govulncheck`/`goreleaser`
      locally and drove `golangci-lint run ./...` from 40 findings down to 0 (see
      "Gotchas" below for the interesting ones - a real zip-slip fix, not just
      suppressions) and `govulncheck ./...` clean (bumped `go.mod`'s `go` directive
      `1.25.0` → `1.26.7` to clear 5 flagged stdlib CVEs, all already fixed upstream by
      that patch level). `goreleaser check`/`build --snapshot`/`release --snapshot
      --skip=publish` all verified locally (the last one fails only at the `syft`-needing
      SBOM stage when run locally without `syft` installed, as expected - CI's
      `release.yml` installs it). README got a light-touch "Go binary (preview)" section
      (build/run/introspection commands + the exit-code contract); the full
      README rewrite and marking `ironstate.ps1` legacy is deliberately deferred to
      Phase 7 (cutover), not attempted here — see "Next concrete step" below.
- [x] Cross-platform facts: `internal/facts.Gather()` now also returns `platform`
      (`runtime.GOOS`), `arch` (`runtime.GOARCH`), `os_family` (new `osFamily()` helper:
      windows→"windows", linux/darwin/bsd-family→"unix", else→goos itself).
- [x] CLI output/UX overhaul (additive polish, not a phase from the master plan): new
      `internal/ui` package (ANSI color helpers gated by `ui.Enabled`, auto-detected via
      `NO_COLOR`/`IRONSTATE_NO_COLOR` env vars + `golang.org/x/term.IsTerminal`, override
      via new `--no-color` flag; per-module emoji map; `PrintFacts` host-facts panel; a
      Windows-only `console_windows.go` that calls `SetConsoleOutputCP(65001)` at init so
      the emoji/box-drawing glyphs don't render as mojibake in a real Windows console).
      `internal/engine`: `Options.Verbose` (prints a dim skip line per already-satisfied
      leaf when set — previously totally silent); per-leaf dispatch lines now show a
      module emoji + colored intent verb (cyan "would install/remove" for dry-run, bold
      "installing/removing" then a bold-green "installed/removed" completion line for a
      real apply); new `Danger` var (bold red, `os.Stderr`) for genuine leaf failures,
      kept distinct from `Warn`'s plain-yellow lesser warnings (missing PATH command,
      etc.); `Info` moved from `os.Stdout` to `os.Stderr` — **this was a real
      pre-existing bug fix**, not just polish: `Info` writing dispatch commentary to
      stdout interleaved with (and corrupted) `--output json`'s result array on the same
      stream, defeating the entire point of a machine-parseable output mode. `internal/
      engine/output.go`: `PrintTable` rewritten to hand-roll column-width padding (NOT
      `text/tabwriter`, which counts raw ANSI escape bytes toward cell width and breaks
      alignment across differently-colored rows) with an emoji + colored STATUS column
      (bright green "installed/removed", cyan "would install/remove", dim gray "skip",
      bold red "failed" — "changed reads brighter than unchanged, failures read as
      danger" per the request); new `Stats`/`ComputeStats`/`PrintSummary` for a final
      colored stats block with elapsed wall-clock time. `internal/cli/root.go`: prints
      `ui.PrintFacts` right after `facts.Gather()`, wires `cfg.Verbose` into
      `engine.Options.Verbose`, times the run and calls `engine.PrintSummary` after
      dispatch. The facts panel and summary are both skipped entirely in `--output json`
      mode so that stream stays clean/pipeable (`results | jq` etc.) — verified live.
- [ ] Phase 7 — cutover

## Gotchas / findings worth knowing before continuing

- **The `create_file` tool sometimes duplicates the first `package X` line** when
  creating a brand-new Go file — happened again for `internal/ui/ui.go` and
  `internal/ui/ui_test.go` during the CLI output overhaul. Always read back the first
  few lines right after `create_file` and fix if doubled; `gofmt`/`go build` will fail
  loudly with "expected declaration, found 'package'" if missed.
- **Never build a colored table with `text/tabwriter`** — it counts raw ANSI escape
  code bytes as part of a cell's width, so two rows with different-length color codes
  (e.g. bold-red "failed" vs. plain-green "installed") misalign visually even though the
  *visible* text lengths differ as expected. Pad the plain (uncolored) string to the
  target width first, THEN wrap the already-padded string in the color function. Same
  pitfall bit `PrintSummary`'s "Failed" row the first pass (fixed by writing a small
  `statLine` closure that pads-then-colors).
- **A Windows console needs `SetConsoleOutputCP(65001)` for real UTF-8 rendering** —
  without it, PowerShell/cmd renders emoji and box-drawing characters as mojibake
  (`ΓöÇ` for `─`, `≡ƒºí` for an emoji) even though the underlying bytes are correct
  UTF-8. Note this only fixes *real interactive console* sessions — redirected/piped
  output (`ironstate.exe ... | Select-Object`) is decoded by the *parent* process's own
  encoding assumptions and can still show mojibake in that case; that's expected and not
  something the child process can control.

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
  3. `internal/filters`'s `exists` filter called `os.Stat` directly on the raw string,
     never expanding a leading `~` - but the original `exists.ps1`'s `Test-Path` resolves
     `~` automatically. Found via a **side-by-side real dry-run diff** (build the Go
     binary, run it and `ironstate.ps1` against the exact same real `site.yml`, diff the
     console output) rather than a unit test - `site.yml`'s own bootstrap assert
     (`facts.local_bin_path | exists`) passed under PowerShell but failed under Go,
     stopping every dry-run dead after just a handful of fact leaves. Fixed by resolving
     via `pathutil.ResolveUserPath` first (see `TestExistsFilterExpandsTilde`).
     **Lesson: any filter/handler doing a real `os.Stat`/file-open on a YAML-authored
     path must resolve `~` first** - `dirname`/`basename` don't need this (pure string
     ops, no filesystem access, matching the originals), but anything touching the
     filesystem does. This side-by-side technique is worth repeating before any future
     phase sign-off, not just trusting green unit tests.
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

Start Phase 7 (cutover). Key things the next agent should know before starting:

- **Phase 6 is hardening only, deliberately not a full README rewrite or
  `ironstate.ps1` deprecation** — the master plan's Phase 6 exit criteria mentions
  "docs updated (README.md rewritten for the Go binary, PowerShell version marked
  legacy...)" but that's really Phase 7 cutover work (it changes what's authoritative
  for real users of this repo, not just hardening the release pipeline) - deferred on
  purpose. What Phase 6 actually did: `.github/workflows/release.yml` (tag-triggered
  `goreleaser release`, installs `syft` via `anchore/sbom-action/download-syft` and
  `cosign` via `sigstore/cosign-installer`, then `actions/attest-build-provenance` over
  every archive + `checksums.txt`), `.goreleaser.yaml`'s new `signs:` block (keyless
  cosign blob-signing of `checksums.txt`, `artifacts: checksum` - covers every archive
  transitively via the hash list, so only the one file needs signing), and
  `.github/dependabot.yml` (gomod + github-actions, weekly).
- **A real security fix, not just lint suppression**: `golangci-lint run ./...` found a
  genuine zip-slip path-traversal gap in `internal/handlers/zip.go`'s archive
  extraction (`filepath.Join(dest, entry.Name)` with no containment check against a
  malicious `../`-containing zip entry name) - fixed with a new `safeJoin` helper that
  rejects any entry resolving outside `dest`. Everything else in the 40 → 0 lint cleanup
  was either a real (if minor) fix - `internal/filters/embed.go`'s shim-extraction
  write path now actually checks `f.Close()`'s error, `internal/handlers/registry_windows.go`'s
  `toByteSlice` now rejects out-of-range Binary byte values instead of silently
  truncating them - or a documented `//nolint` justification for a conversion that's
  provably safe given the actual value ranges involved (every Windows registry hive
  constant, DWord/QWord width casts matching the original PowerShell's own casts, the
  UTF-16LE byte-splitting in `scheduledtask_windows.go`'s `writeUTF16File`). Don't
  re-add blanket `errcheck`/`gosec` disables to `.golangci.yml` to make future findings
  go away quietly - fix or justify per-callsite, matching this established pattern.
- **`go.mod`'s `go` directive bumped `1.25.0` → `1.26.7`**: `govulncheck ./...` flagged 5
  vulnerabilities, but all 5 were in the **Go standard library itself** (net/url, tls,
  asn1, idna/http), not this project's own code - and all 5 are already fixed in
  upstream Go patch releases at or before 1.26.7. The prior `1.25.0` pin would have made
  CI's `setup-go@v5` (which reads `go-version-file: go.mod`) install an *older*,
  *more* vulnerable toolchain than what was already on this dev machine - always
  re-run `govulncheck ./...` locally after any dependency bump and bump the `go`
  directive itself when the flagged CVEs are stdlib-only and already patched upstream,
  rather than trying to "fix" application code that was never actually vulnerable.
- **Local validation commands that now work and are worth reusing**: `go install
  github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest` (matches CI's
  `lint` job's config exactly, since it reads the same `.golangci.yml`), `go install
  golang.org/x/vuln/cmd/govulncheck@latest`, `go install
  github.com/goreleaser/goreleaser/v2@latest` (then `goreleaser check`, `goreleaser
  build --snapshot --clean`, `goreleaser release --snapshot --clean --skip=publish` -
  the last one fails only at the SBOM stage without `syft` on PATH, exactly as
  expected locally; CI's `release.yml` installs it). All three took real (if slow)
  `go install`/network time but worked without any other tooling.
- **Re-run the `eps`/`herestring` and `shell.host: pwsh` native-merge grep audits**
  mentioned in the master plan's §10/§11 one more time immediately before any Phase 7
  cutover decision, in case either gained real usage since Phase 4/6's own re-checks -
  they hadn't as of this session.
- All Phase 6 changes are config/docs/lint-only - no engine/handler *behavior* changed
  except the zip-slip fix above (a bug fix, not a feature change) - so re-running the
  full `go build ./...`/`go vet ./...`/`gofmt -l .`/`go test ./...` (all green on
  `windows`, cross-compiled `linux`/`darwin`) plus the new `golangci-lint run ./...`/
  `govulncheck ./...` (both clean) was the whole verification loop this session, no
  new fixtures needed.

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
