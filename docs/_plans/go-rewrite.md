# IronState Go Rewrite — Master Plan

Status: IN PROGRESS (reviewed once via rubber-duck subagent, see [Revision Log](#revision-log))
Owner: (fill in)
Target: replace `ironstate.ps1` + `modules/*.psm1` with a single Go binary, byte-for-byte
compatible with the current `main.yml`/`hosts/`/`variables/`/`packages/`/`roles/` document
model and CLI behavior.

## Implementation status

The phased breakdown lives in [§10](#10-migration--rollout-plan-phased); this table is
just a quick at-a-glance progress marker kept in sync with it.

| Phase | Status |
| --- | --- |
| 0 — Scaffolding | ✅ Done — `go.mod`, `cmd/ironstate`, cobra/viper CLI (`version`/`filters list`/`doctor` + root flags), `.goreleaser.yaml`, `.github/workflows/ci.yml`+`codeql.yml`, `.golangci.yml` |
| 1 — Core engine, no I/O | ✅ Done — `internal/expr` (lexer/parser/AST/evaluator + fuzz tests), `internal/template` (span scan/expand, soft-vs-strict, boundary keys), `internal/filters` (all 21 built-ins) |
| 2 — Document loading & flattening | ✅ Done — `internal/model` (generic YAML shape + helpers), `internal/packages` (hierarchy load/merge, `include` resolution, `.env` loader, path resolution), `internal/tasks` (`Expand-TaskTree` port: loops, `parent.item`, tags/when cascading), `internal/facts` (Windows-real / other-stub build split). Validated against the **real** `main.yml` + `hosts/krayt.yml` → `hosts/camalot` → the full `roles/*`/`packages/*` tree (181 flattened leaves, zero errors) |
| 3 — Engine + low-risk handlers | ✅ Done — `internal/conditions` (Test-WhenClause/Test-Condition port), `internal/engine` (Invoke-Tasks/Invoke-PackageItem port: registry/user-facts/command-availability threading, facts-first two-phase `Run`, the two dry-run-forces-execution exceptions, missing-handler/missing-command no-result-row, tag filtering, table/JSON output), `internal/handlers` (`log`, `path` [Windows-only, build-tagged], `fact`, `assert`, `file`, `copy`, `symlinks`, `blockinfile`, `ssh_host_block`, `zip`). CLI (`internal/cli/root.go`) now wired end-to-end and smoke-tested (dry-run) against the real `main.yml` |
| 4 — Package-manager handlers + `shell` + template engines | ✅ Done — `internal/exec` (Runner abstraction), package-manager handlers (`winget`, `chocolatey`, `pipx`, `npm`, `cargo`, `go`, `gem`, `eget`), `shell` (per-state present/absent/latest fallback, host presets, `creates` - pwsh host is always a subprocess in Go, no in-process native-object merge, an audited-unused v1 gap), `registry` (Windows-only, `golang.org/x/sys/windows/registry` directly, no PSDrive mounting needed), `scheduled_task` (Windows-only, generates a Task Scheduler XML definition and shells out to `schtasks.exe` rather than the ScheduledTasks PowerShell module/CIM - keeps `shell.host: pwsh` the only pwsh dependency, at the documented cost of existence+enabled-only idempotency, no deep drift detection), `internal/templateengines` (native `jinja` port reusing `internal/expr`, plus additive `gotemplate` via Go stdlib `text/template`; `eps`/`herestring` are hard errors), `template` module wired to both engines, `fact`'s embedded `shell` and `blockinfile`'s `template` field now delegate to the real `shell`/`template` implementations (closing the two Phase 3 stand-in gaps) |
| 5 — Filter plugin system (script filters) | ✅ Done — `internal/filters`'s script-filter adapter: a generic, embedded PowerShell shim (`embed/shim.ps1`) speaks a JSON-over-stdio protocol to an unmodified `filters/*.ps1` file; a persistent per-filter worker process (`Pool`/`scriptWorker`) is kept warm rather than spawned per call; `DiscoverScriptFilters` registers a discovered script under its own name only when no built-in already claims it. `ironstate filters list`/`doctor` report discovered script filters; `internal/cli/root.go`'s real run wires discovery in too. Verified against a live `pwsh`-backed round trip, not just fakes. |
| 6 — Hardening (SBOM/packaging/signing) | ✅ Done — `.github/workflows/release.yml` (tag-triggered `goreleaser release`, `syft`/`cosign` installed via actions), `.goreleaser.yaml`'s `signs:` block (keyless cosign signing of `checksums.txt` via GitHub OIDC) alongside the existing `sboms:` block, `actions/attest-build-provenance` on every release artifact, `.github/dependabot.yml` (gomod + github-actions, weekly). `golangci-lint run ./...` reduced from 40 findings to 0 (real zip-slip path-traversal fix in `zip.go`'s extraction, `go.mod`'s `go` directive bumped `1.25.0` → `1.26.7` to clear 5 stdlib CVEs `govulncheck` found, plus errcheck/gosec/unused/staticcheck cleanup — see the progress doc's Phase 6 notes for the full list). `go build`/`go vet`/`gofmt`/`go test ./...` green on `windows`, cross-compiled `linux`/`darwin`. README got a light-touch "Go binary (preview)" section + documented exit-code contract; the full README rewrite/PowerShell-legacy marking is deliberately deferred to Phase 7 (cutover), not done here. |
| 7 — Cutover | ✅ Done — `ironstate` (Go binary) is now the primary/only tool; `ironstate.ps1`/`modules/*.psm1` removed outright (not deprecated-in-place - git history preserves them if ever needed, and this isn't a full release yet, so no fallback-compatibility window was kept); README has no PowerShell-version references at all. A real-host side-by-side `-Apply` validation run remains an open item, tracked in the progress doc's "Next concrete step" rather than blocking this phase |

Additive work beyond the phase list above (see progress doc for full detail): cross-platform
`platform`/`arch`/`os_family` facts, a CLI output/UX overhaul (new `internal/ui` package —
colors, emoji, `--no-color`, Windows console UTF-8 fix; `engine.Options.Verbose`; colored/emoji
`PrintTable`; `PrintFacts`/`PrintSummary` panels; moved `Info` to stderr so `--output json`
stays pipeable), an `ironstate init` scaffolding command, and the default script-filters
directory renamed from `modules/Filters` to `filters` (config key `filters.dir`, `ironstate
init`'s scaffold, `filters list --dir`/`doctor --filters-dir` defaults all updated together).

Detailed engineering notes on what was built, deviations found, and bugs caught during
implementation are kept in session/repo memory (not duplicated into this document) —
ask to have them folded in here if this plan is being handed off.

## 1. Goals & non-goals

**Goals**

- Single statically-linked Go binary (`ironstate.exe` / `ironstate`), no PowerShell runtime
  required to *run* the tool itself.
- Full behavioral compatibility with the current PowerShell engine: same YAML schema, same
  merge order (`main.yml` → `hosts/<COMPUTERNAME>.yml` → `variables/<USERNAME>.yml`), same
  task-tree flattening rules (tags cascade, `when` cascades AND, `with`/`items` looping,
  `parent.item`), same fact-gathering-first phase, same `id` registry semantics, same
  `${{ }}` expression grammar and filter pipeline, same dry-run-by-default / `-Apply` model.
- CLI built on **cobra**, configuration/flags/env layered with **viper**.
- Filters stay extensible and can remain implemented as PowerShell scripts (current
  `modules/Filters/*.ps1`), with a plugin contract general enough to add other script
  types (Python, Node, etc.) later without another rewrite.
- Automated test suite (unit + fixture/golden + compatibility-vs-PowerShell) run in CI.
- SBOM generated for every release artifact.
- Reproducible, signed, checksummed binary packaging for Windows (primary), with the core
  engine kept OS-agnostic so Linux/macOS builds are possible for non-Windows-only handlers.
- CI/CD entirely on GitHub Actions.

**Non-goals (explicitly out of scope for v1)**

- Porting `eps` or `herestring` (see [§4.7](#47-template-engines)) — a repo-wide audit
  found zero real usage of either engine in `main.yml`/`hosts/*`/`packages/*`/`roles/*`/
  `tasks/*` (only `jinja` is actually used, in `roles/development/git/main.yml` and
  `tasks/wsl-shim/main.yml`); both are dropped outright rather than ported or shelled out
  to `pwsh`. `template.engine`/`blockinfile.template.engine` accepting `eps`/`herestring`
  becomes an explicit, documented error in the Go binary — see §5/§11 for the removal
  decision record.
- Cross-platform parity for Windows-only handlers (`winget`, `chocolatey`, `registry`,
  `scheduled_task`) — these stay Windows-only; the engine itself is portable.
- Changing the YAML schema/semantics. Any deviation found necessary during implementation
  must be flagged and decided on explicitly, not silently introduced.
- A GUI or TUI. CLI output stays table/log based (with an optional `--output json` for
  scripting/tests, additive, not required by current users).

## 2. Current system summary (reference)

(Full detail lives in the existing [README.md](../../README.md); this is the condensed
model the Go rewrite must reproduce.)

- **Entry point**: `ironstate.ps1 [-PackagesFile <path>] [-Apply] [-Tags a,b]`, dry-run
  unless `-Apply`.
- **Document model**: a YAML doc is either `{ tasks: [...], vars: {...} }` or a bare list.
  Every task-list item is either a *grouping task* (`actions:`), an `include:` (loads
  `<PackagesRoot>/<name>/main.yml`), or a *leaf* (exactly one module key: `winget`,
  `chocolatey`, `pipx`, `npm`, `cargo`, `go`, `gem`, `eget`, `zip`, `symlinks`, `file`,
  `copy`, `template`, `shell`, `blockinfile`, `ssh_host_block`, `log`, `path`, `fact`,
  `assert`, `registry`, `scheduled_task`).
- **`include` is one generic, repo-root-relative resolver, not a "packages vs. roles"
  special case.** `ironstate.ps1` passes `PackagesRoot = $PSScriptRoot` (the repo root);
  `Import-IncludedPackage` always resolves `<PackagesRoot>/<name>/main.yml` verbatim, with
  no folder-specific logic at all. `name` is just a path fragment — real usage in this
  repo includes `roles/wsl`, `hosts/camalot` (from `hosts/krayt.yml`/`hosts/kresh.yml`),
  and nested package/role paths like `roles/development/git`. The Go port must implement
  a single root-relative `<name>/main.yml` resolver and be tested against `hosts/`- and
  `roles/`-rooted includes, not just `packages/`.
- **Flattening** (`modules/Tasks.psm1`): recursive walk accumulating `tags` (union) and
  `when` (list, AND'd) top-down; `with`/`items` materializes a task once per loop value
  *before* anything else on it is evaluated, exposing `${{ item }}` / `${{ parent.item }}`
  chains; `include` loads and merges a child document with its own `vars`/`inputs`/
  `package` namespace, isolated from the parent's `PackageVars`/loop context.
  Non-evaluated `when` strings and unresolved `${{ }}` are carried on each flattened leaf.
- **Expression language** (`modules/Expressions.psm1`): shared tokenizer/parser/evaluator
  used by both `when:` (`Conditions.psm1`) and `${{ }}` (`Templates.psm1`). Grammar:
  or/and/not, comparisons (`== != < <= > >=`, case-sensitive string compare), `in`/`not in`,
  `is`/`is not` type tests (`mapping`, `boolean`, `string`, `number`, `list`, `defined`,
  `none`, plus aliases), a `|` filter pipeline, dotted/indexed paths (`a.b[0].c`), list
  literals, bare/parenthesized calls. String literals may embed `${{ }}` spans themselves.
- **Filters** (`modules/Filters/*.ps1`): one file per filter, loaded by filename at import
  time, each `param($Value, [object[]] $ArgValues)`. Current set: `default`, `toggle`,
  `ternary`, `enabled`, `upper`, `lower`, `trim`, `quote`, `length`, `concat`, `join`,
  `split`, `prefix`, `dirname`, `basename`, `resolve`, `exists`, `sha1`, `lookup`,
  `from_json`, `json_query`.
- **Facts** (`modules/Facts.psm1`): small fixed set gathered fresh every run
  (`computer_name`, `user_name`, `home`, `os_version`, `os_build`, `is_admin`,
  `pwsh_version`). User-defined `fact` leaves run in their own pass, in document order,
  *before* any other leaf, and are namespaced under `facts.<name>`.
- **Dry-run has two real-execution exceptions.** `Invoke-Tasks` forces actual execution
  (`Apply:($Apply -or $hasEmbeddedShell -or ($module -eq 'assert'))`) regardless of
  `-Apply` for (a) a `fact` leaf with an embedded `shell`, and (b) every `assert` leaf —
  both are side-effect-free by design, so previews stay accurate. A fact's embedded-shell
  `value` is additionally removed from the item *before* the strict per-leaf template
  pass, the shell runs, then `value` is resolved against a context that includes that same
  shell's own bare `rc`/`stdout`/etc. (self-reference, same convention as `failed_when`),
  and only then committed to `userFacts`. A Go port that gates all execution behind
  `--apply`, or that resolves a fact's `value` before its embedded shell has run, silently
  changes behavior.
- **A leaf with no registered handler, or whose backing CLI isn't on PATH, produces no
  result row at all** (`Write-Warning; continue` — not `Skip`, not `Failed`, simply absent
  from the final results table). The compatibility harness (§5) must diff on absence, not
  just on state, to catch a regression here.
- **`$script:ModuleCommandNames` remaps exactly one module today** (`chocolatey → choco`);
  every other module name is assumed to equal its own CLI command. Don't over-build a
  large remapping table in Go for a problem that currently has one instance.
- **`.env`/`.secrets` loading**: `ironstate.ps1` loads `<repoRoot>/.env` and
  `<repoRoot>/.secrets` into the process environment before fact gathering, where
  `$repoRoot` is *two* directories above the script's own folder
  (`Split-Path (Split-Path $PSScriptRoot -Parent) -Parent`), not the script's own
  directory. This is load-bearing for any handler reading `$env:*` (e.g. a scheduled
  task's password) and needs an explicit, documented decision in the Go port rather than
  an assumed "same directory as the binary" default — especially once the binary may be
  distributed separately from a repo clone (see §11).
- **`pwsh` self-relaunch**: under Windows PowerShell 5.1, `ironstate.ps1` relaunches
  itself under `pwsh` if found on PATH, else warns and continues on 5.1. This has no Go
  equivalent need (the engine itself needs no PowerShell runtime, and §1/§4.7 mean
  template rendering needs no PowerShell either now that `eps` is dropped), but it
  quietly guaranteed *which* `pwsh` build backed `shell: { host: pwsh }` — once that
  becomes a plain subprocess invocation from Go (§4.8), there is no more such guarantee,
  and no automatic fallback/warning. Surface this as an explicit `ironstate doctor` check
  instead.
- **Exit codes today are coarser than they look**: `exit 1` covers both a deliberate
  `failed_when`-stopped run *and* any uncaught PowerShell error (`Set-StrictMode` +
  `$ErrorActionPreference = 'Stop'` mean a YAML parse failure, for instance, crashes with
  a stack trace and a nonzero code) — these two cases are indistinguishable today. Decide
  explicitly whether the Go binary keeps that ambiguity (`1` for both) or introduces
  distinct codes (e.g. `2` for load/parse errors, `1` for a stopped run) — either choice
  is fine, but it must be a recorded decision, not an accident of how `main()` happens to
  return.
- **Execution** (`ironstate.ps1`'s `Invoke-Tasks`): sequential dispatch in document order;
  each leaf's `when`/remaining `${{ }}` resolve immediately before it runs against a flat
  context (facts, package vars, site vars, growing `id`/`fact` registry — precedence in
  that order, last write wins); a handler's `Test`/`Describe`/`Install`/`Uninstall`
  determine `Skip`/`Install`/`Uninstall`; results normalize to
  `{ rc, stdout, stdout_lines, stderr, stderr_lines }` plus any merged native properties
  (`shell` under `host: pwsh`); `id` registers the result (looped tasks accumulate
  `.results[]`); `failed_when`/`continue_on_error` control failure/stop semantics.
- **Template engines** (`modules/TemplateEngines/*.psm1`): `jinja` (sandboxed,
  block-capable), `eps` (PowerShell Gallery EPS — full PowerShell), `herestring`
  (interpolation-only, no blocks) — used by the `template` module and
  `blockinfile.template`. **Only `jinja` has any real usage in this repo today** (verified
  by grep: `roles/development/git/main.yml`, `tasks/wsl-shim/main.yml`; README examples
  aside, `eps`/`herestring` appear nowhere else) — see §4.7/§11 for the resulting v1
  scope decision.
- **Handlers** (`modules/Handlers/*.psm1`): uniform `{ Test; Describe; Install; Uninstall }`
  script-block shape per module.

## 3. Target repository layout

```
ironstate/                        (repo root, unchanged: main.yml, hosts/, variables/, packages/, roles/, tasks/)
├── cmd/
│   └── ironstate/
│       └── main.go               # cobra root command wiring only
├── internal/
│   ├── cli/                      # cobra commands (root, version, filters list, doctor)
│   ├── config/                   # viper-backed flag/env/config-file layering
│   ├── model/                    # YAML document types (ordered maps, task/leaf shapes)
│   ├── expr/                     # ported Expressions.psm1: lexer, parser, AST, evaluator
│   ├── template/                 # ${{ }} span scanning + expansion (uses internal/expr)
│   ├── filters/                  # built-in Go filters + external-script plugin loader
│   ├── facts/                    # fact gathering (per-OS files: facts_windows.go, ...)
│   ├── tasks/                    # tree flattening, loop expansion, tag/when cascading
│   ├── packages/                 # file hierarchy loading/merging, include resolution
│   ├── engine/                   # Invoke-Tasks equivalent: registry, dispatch loop
│   ├── handlers/                 # one subpackage per module + Handler interface/registry
│   │   ├── winget/ choco/ pipx/ npm/ cargo/ gomod/ eget/ zip/ symlinks/ file/ copy/
│   │   ├── template/ shell/ blockinfile/ sshhostblock/ log/ path/ fact/ assert/
│   │   └── registry/ scheduledtask/
│   ├── templateengines/          # jinja (native), gotemplate (native, Go stdlib text/template)
│   ├── exec/                     # command runner abstraction (mockable in tests)
│   └── output/                   # result table / json rendering
├── testdata/                     # fixture YAML docs + golden expected output
├── docs/
│   └── plans/
│       └── go-rewrite.md         # this document
├── .github/
│   └── workflows/                # ci.yml, release.yml, codeql.yml
├── .goreleaser.yaml
├── go.mod / go.sum
└── ironstate.ps1                 # kept during migration, see §9 rollout plan
```

Rationale for `internal/` over `pkg/`: nothing here is meant to be imported by other Go
modules; `internal/` prevents accidental external coupling while the design is still
moving.

## 4. Component design

### 4.1 CLI (cobra) + configuration (viper)

Root command mirrors current flags exactly, plus additive ones:

```
ironstate [--playbook main.yml] [--apply] [--tags a,b] [--output table|json] [-v|--verbose]
ironstate version
ironstate filters list                # introspection: built-in + discovered script filters
ironstate doctor                      # PATH checks for winget/choco/etc.
```

- `--playbook` defaults to `./main.yml` (was `-PackagesFile`); a hidden alias flag keeps
  `-PackagesFile`-shaped invocations from breaking any existing muscle memory/scripts,
  but the documented primary flag becomes `--playbook`.
- Viper layers, highest precedence first: CLI flags → environment variables
  (`IRONSTATE_*`) → optional `ironstate.yaml`/`.ironstate.yaml` config file (new,
  additive — lets a user pin `--tags`/`--playbook` defaults) → built-in defaults. This is
  purely ergonomic; it must never change what `main.yml`'s own `vars`/merge behavior does.
- Cobra's `PersistentPreRunE` wires viper→typed config struct once; all subcommands read
  the same struct, no direct `viper.Get*` calls scattered through business logic (keeps
  `internal/engine` etc. testable without viper in the loop at all).

### 4.2 Data model (`internal/model`)

YAML decoding must preserve **key order** where the original relies on it implicitly
(mainly for readable diagnostics, not for behavior — the PowerShell implementation uses
`ConvertFrom-Yaml -Ordered`, but only *document order of tasks* is semantically load-bearing,
not map key order within a single leaf). Use `gopkg.in/yaml.v3` (`yaml.Node` or a custom
`UnmarshalYAML` into a `map[string]any`-like structure) so:

- Untyped nested structures decode into a generic `Value` type (`map[string]any` /
  `[]any` / scalars) mirroring PowerShell's `[ordered]` hashtable + `IList` duck-typing,
  since the schema is intentionally loose (see `ironstate.schema.json`) and a strict Go struct
  per module would fight the dynamic `${{ }}`/`when` substitution model.
- A thin typed `Leaf`/`Task` view is derived *after* flattening (module name, item map,
  tags, when, id, failed_when, continue_on_error, looped, package vars/inputs/package),
  matching `Tasks.psm1`'s `Expand-TaskTree` output shape.
- `ironstate.schema.json` is treated as **documentation to reconcile, not an assumed source of
  truth** — it is already out of sync with runtime behavior today (e.g. it has no `gem`
  entry despite `ironstate.ps1` registering a working `gem` handler used by
  `roles/languages/ruby/main.yml`). Phase 1 includes an explicit schema-vs-`ironstate.ps1`
  audit (every module key, every field) before the schema is trusted as a validator input;
  discrepancies found get fixed in the schema itself (small, low-risk PRs against the
  existing PowerShell-era repo) rather than silently baked into the Go model. Once
  reconciled, add a `go generate`-able validator (`internal/model/schema_test.go`) that
  validates every fixture and real `main.yml`/overlay file against it in CI.

### 4.3 Expression engine (`internal/expr`)

Direct, disciplined port of `Expressions.psm1`:

- Lexer: same token set (`( ) [ ] , . | == != < <= > >= and or not in is true false null
  string number ident`), same string-escape rules (`\n \r \t`), same number handling
  (float64-backed, matching PowerShell's untyped numeric literal).
- Parser: same precedence chain (`or_expr > and_expr > not_expr > comparison > membership >
  pipeline > primary`), producing a small AST (Go struct types / discriminated union via
  a sealed interface, not `map[string]any` — this is the one place where a stronger,
  compiler-checked Go representation is strictly better than 1:1 mirroring the PowerShell
  hashtable AST).
- Evaluator: same case-sensitive string comparisons, same truthy-cast rules, same `is`/
  `is not` semantics (a `map[string]any` counts as `mapping`, `bool` as `boolean`, etc.),
  same dotted/indexed path resolution through `map[string]any`/`[]any`.
- Filter pipeline calls into `internal/filters`' registry (interface, not a hardcoded
  switch), so `internal/filters` can add script-backed filters without `internal/expr`
  knowing anything changed.
- **Port strategy**: write a table-driven Go test per existing PowerShell filter/operator
  behavior *before* porting logic (translate the prose in `Expressions.psm1`'s docstring
  and README's grammar table into test cases first), then implement until green. This is
  the highest-risk subtlety area and needs the most fixture coverage from real
  `main.yml`/`packages/*`/`roles/*` usage:
  - The `enabled()` filter's mapping-descends/boolean-short-circuits walk
    (`modules/Filters/enabled.ps1`) gates nearly every role/package inclusion in this repo
    (`roles/*/main.yml`, `hosts/camalot/main.yml`) — a bug here has the widest blast
    radius of any single filter and deserves its own named, exhaustive test suite, not
    just "one of many filters."
  - `Templates.psm1`'s `Test-ExpressionNamespaceKnown` treats `facts.*` differently from
    every other namespace during the soft pass: `vars`/`package`/`inputs` are complete the
    moment their namespace key exists, so "top segment present" is enough to decide
    "resolvable now" — but user-defined facts populate progressively, so `facts.*` must
    resolve **the full path**, not just check the top segment, before counting as known.
    A mechanical Go port that treats all namespaces uniformly will incorrectly defer or
    incorrectly commit `facts.*` references; this needs a dedicated test category (not a
    footnote) covering a fact referenced before/after it's gathered.
  - Fuzz-test the tokenizer/parser (`go test -fuzz`) — the grammar is small and
    well-defined, and string-literal escaping / unterminated `${{`/`}}` span handling is
    exactly the class of bug fuzzing finds fastest; cheap to add, not optional.

### 4.4 Template expansion (`internal/template`)

Port of `Templates.psm1`: span-scanning (`${{ ... }}` occurrences, quote-aware so a
filter argument's `}}`-free string literal can't prematurely end a span), whole-value vs.
embedded substitution, the "omit key entirely when the whole value is one unresolved
reference" marker behavior (`TemplateOmitMarker`), and the `-Soft` two-pass resolution
model (`package`/`inputs`/`facts`/`vars`/a package's own local vars resolve early;
anything touching a not-yet-known `id`/`fact` registry defers to the per-leaf dispatch
pass). Represent the "soft, still unresolved" case as a distinct Go type/sentinel rather
than reusing `nil`, mirroring the PowerShell `Handled=$false` vs. `Value=$null` distinction
exactly (these are not the same thing and a naive port easily conflates them).

Two more mechanisms are easy to miss in a surface-level port and must be included:

- **This is really a three-pass system, not two.** `Handlers/Template.psm1`'s
  `Resolve-TemplateRenderContext` runs a **third**, self-referential `${{ }}` pass over
  the merged render context immediately before handing it to `jinja`/`gotemplate`.
  It exists because a `vars:` value can reference a *sibling* var directly (e.g. one vars
  key pointing at bare `development.work` rather than `vars.development.work`) — a
  reference the whole-document soft pass never touches (it only knows `facts`/`vars` as
  namespaces, not the content already living inside `vars:`) and the per-leaf strict pass
  never revisits either (it only re-resolves a leaf's own module fields). Without porting
  this third pass, a rendered template can contain literal, unexpanded `${{ }}` text.
  Needs a dedicated fixture: a `vars:` entry containing `${{ }}` that itself feeds a
  `template:` task.
- **Loop "boundary keys"**: `Expand-TemplateNode`'s `-BoundaryKeys` mechanism stops an
  outer loop's template pass from reaching into a *nested* loop's own fields and
  resolving them against the wrong (outer) `item` binding — only the loop-selector field
  itself (`items`/`with`) resolves in the enclosing scope; every sibling field is left
  completely untouched for the inner loop's own later pass. `internal/tasks`' loop
  materialization needs an equivalent explicit "stop descending here" signal in its tree
  walker — this doesn't fall out for free from a generic recursive expander and must be
  designed in from the start, not patched in after nested-loop fixtures start failing.

### 4.5 Filters (`internal/filters`) — extensibility design

This is the piece the user explicitly called out, so it gets its own contract:

- **`Filter` interface**: `Apply(value any, args []any) (any, error)`.
- **Built-in filters**: every current `.ps1` filter reimplemented natively in Go
  (`default`, `toggle`, `ternary`, `enabled`, `upper`, `lower`, `trim`, `quote`, `length`,
  `concat`, `join`, `split`, `prefix`, `dirname`, `basename`, `resolve`, `exists`, `sha1`,
  `lookup`, `from_json`, `json_query`), registered in a static `map[string]Filter` at
  package init — matches current "add a file, no registration step" ergonomics for the
  *built-in* set as closely as Go's lack of dynamic file-drop-in allows (new built-ins are
  a Go source file + one line in an init-time list, documented in `internal/filters/README.md`).
- **External script filters (compatibility + extensibility)**: a `ScriptFilter` adapter
  implements the same `Filter` interface by shelling out. Discovery: a configurable
  directory (default `modules/Filters/`, same as today) is scanned at startup; each file
  found is registered under its base name *if* a Go built-in of that name doesn't already
  exist, so today's `modules/Filters/*.ps1` continue to work unmodified during migration,
  and net-new custom filters can be dropped in the same way without recompiling the binary.
  - **Calling convention** (new, since PowerShell script-blocks can't be invoked directly
    from Go): a small stable JSON protocol over stdio —
    `{"value": <json>, "args": [<json>...]}` on stdin, `{"result": <json>}` or
    `{"error": "..."}` as the single line of stdout. A thin shim script per supported
    interpreter adapts this to the language's native calling convention:
    - `modules/Filters/shim.ps1` (or a small embedded wrapper generated per file) reads
      stdin JSON, calls the existing `param($Value, [object[]] $ArgValues)` script with
      `-Value`/`-ArgValues` bound from the decoded JSON, JSON-encodes the return value.
      This keeps every existing `.ps1` filter file **completely unmodified** — the shim is
      the only new artifact, and it's generic (one shim, parameterized by target script
      path), not one per filter.
    - The interpreter used per extension is configurable
      (`filters.interpreters: { ".ps1": ["pwsh", "-NoProfile", "-File"], ".py": ["python3"] }`)
      via viper, with `.ps1 → pwsh` as the shipped default — this is the "eventually
      support other script types" hook the user asked for, wired up now even though only
      PowerShell ships initially.
    - Process-per-call has real overhead (`pwsh` cold start ~100–300ms). Mitigate with a
      **persistent worker pool per script file**, opened lazily on first use and kept
      warm for the process lifetime (stdin/stdout kept open, one JSON request/response
      per call over the same pipe) rather than spawning fresh each invocation — needed
      because filters like `default`/`upper` can run hundreds of times across a large
      `main.yml`. Fall back to spawn-per-call only if the interpreter doesn't tolerate a
      long-lived REPL-style loop.
  - Errors from a script filter surface as Go `error` and propagate the same as a thrown
    PowerShell exception did (aborts expression evaluation with a clear message including
    which filter/file).
- **Test strategy**: golden-test every built-in Go filter against the *current*
  PowerShell script's behavior for the same inputs (generate the golden values by
  actually invoking the existing `.ps1` files once during migration, freeze as test
  fixtures) for every filter **except** the two that are inherently non-deterministic to
  generate goldens from: `lookup('url', ...)` hits a live network endpoint (non-hermetic,
  and unsafe to bake into a CI-run golden-generation step), and `json_query`'s behavior
  branches on whether `jq` happens to be installed on whatever machine generates the
  fixture. Both instead get **hand-written spec tests**: `lookup` with a mocked/injectable
  HTTP transport and a local-file fixture for the `file` action; `json_query` with both
  the `jq`-present and `jq`-absent code paths deliberately exercised (don't assume the CI
  runner's `jq` presence one way or the other).
- **Concurrency**: the persistent worker pool (above) is the one place in the whole Go
  codebase with real concurrency (multiple filter calls potentially in flight against the
  same long-lived script process). Require `go test -race` on `internal/filters` in CI,
  not just as a general suggestion.

### 4.6 Facts (`internal/facts`)

Same fixed set (`computer_name`, `user_name`, `home`, `os_version`, `os_build`, `is_admin`,
`pwsh_version`). `pwsh_version` becomes "the newest PowerShell found on PATH, or empty" once
the runner itself is no longer PowerShell — document this one intentional semantic note
(see [§8 open questions](#8-risks--open-questions)). Build-tag-split per OS
(`facts_windows.go` for `is_admin` via Windows token elevation check, `computer_name` via
`os.Hostname()`/`COMPUTERNAME`) so the package compiles (with reduced fact set) on
non-Windows for engine-only testing.

### 4.7 Template engines (`internal/templateengines`)

Only `jinja` is ported from the current system; `eps` and `herestring` are dropped
outright (§1, §11) since a repo-wide grep found no real usage of either — the Go binary
gains a new `gotemplate` engine instead, using Go's standard `text/template` directly
(no external dependency, always available):

| Engine | Strategy |
| --- | --- |
| `jinja` | **Build-vs-buy spike required before committing.** `modules/TemplateEngines/Jinja.psm1` is a small (~250-line), hand-rolled *subset* interpreter that already reuses `internal/expr`'s evaluator — porting it natively (no external dependency) may be lower-risk/-effort than adopting a third-party Jinja2-subset library of unknown parity/license (candidates if buy wins: `github.com/nikolalohinski/gonja/v2`, `github.com/noirbizarre/gonja`). Phase 4 spike must first enumerate every Jinja construct actually used across `roles/development/git/main.yml`/`tasks/wsl-shim/main.yml` (`{{ }}`, `{% if %}`, `{% for %}`, `{% set %}` — confirm no others are in real use) before choosing either path. |
| `gotemplate` (new) | Native Go `text/template` (stdlib, no dependency), run against the same merged render context as `jinja` (facts/vars/registry, plus this task's own `vars`, after the same third self-referential `${{ }}` pass in §4.4). Additive engine, not a compatibility requirement — offered because it's effectively free once the engine is Go and gives users a more powerful/familiar option (native `range`/`with`/`define`/pipeline funcs) than the current sandboxed `jinja` subset. Document clearly that `gotemplate`'s own `{{ }}` delimiter is unrelated to and coexists with ironstate's own `${{ }}` expression syntax (the render *context* is still built the same way; only the template body's own syntax differs per engine). |
| ~~`eps`~~ / ~~`herestring`~~ | **Dropped, not ported.** `template.engine: eps`/`herestring` (or `blockinfile.template.engine`) is a hard, documented error in the Go binary. See §11 for the audit that justified this and the migration note owed to any future author who reaches for either. |

### 4.8 Handlers (`internal/handlers/*`)

`Handler` interface, one Go package per module:

```go
type Handler interface {
    Test(item map[string]any, name string, ctx Context) (bool, error)
    Describe(item map[string]any, action Action) (string, error)
    Install(item map[string]any, name string, ctx Context) (ExecResult, error)
    Uninstall(item map[string]any, name string, ctx Context) (ExecResult, error)
}
```

- `internal/exec` wraps process invocation behind an interface (`Runner.Run(exe string,
  args []string) (ExecResult, error)`) so every CLI-backed handler (`winget`, `chocolatey`,
  `pipx`, `npm`, `cargo`, `go`, `gem`, `eget`) is unit-testable with a fake runner asserting
  the exact argv built, without touching a real package manager — this is the #1 enabler
  for meaningful CI tests of handler logic on non-Windows runners too.
- Modules with no external CLI (`symlinks`, `zip`, `copy`, `shell`, `blockinfile`,
  `ssh_host_block`, `log`, `path`, `fact`, `registry`, `scheduled_task`, `file`, `template`,
  `assert`) port their pure-Go logic directly; `registry`/`scheduled_task` are Windows-only
  (build-tagged, return a clear "unsupported on this OS" error elsewhere).
- **`zip` and `shell` share a `creates`-glob idempotency primitive**
  (`Resolve-CreatesPatterns`/`Test-CreatesPresent`/`Remove-CreatesPatterns` in
  `Common.psm1`) with real subtlety worth porting as one shared Go helper, not
  reimplemented twice: an empty/absent `creates` list means "can't tell" → always
  not-installed; a glob pattern whose parent directory doesn't exist is treated as
  not-matched (not an error).
- **`shell`'s per-state override block** (`present`/`absent`/`latest`, each optionally
  carrying its own `command`/`script`/`args`/`host`/`extension`) falls back **field-by-
  field**, not block-by-block, to the top-level config — except `absent`, which
  deliberately has **no** fallback to the top-level (present-oriented) block at all, to
  avoid ever accidentally re-running an install command on uninstall. `fact`'s embedded
  `shell` reuses this same resolution and must reproduce the identical fallback rule.
- **Symlink/hardlink handling** (`symlinks` as a thin wrapper over `file`'s `link`/`hard`
  types) needs its own normalization logic ported
  (`ConvertTo-NormalizedPathString` in `Common.psm1`): Windows always reports a real
  symlink's target with backslashes regardless of how the YAML wrote it, so comparison
  must normalize separators/trailing-slash before comparing, or every symlink will appear
  perpetually out-of-state.
- **`shell.host: pwsh` native-object merge audit — done, not deferred**: a repo-wide grep
  for `host: pwsh` and for any consumer dotting into a non-reserved field off a `shell`
  task's `id` (the `${{ pf.ProgramFilesDir }}` pattern from the README) found **zero real
  usages** in `main.yml`/`hosts/*`/`packages/*`/`roles/*` today — this feature is currently
  demonstrated only in documentation, not exercised anywhere in this repo. Given that,
  `shell`'s Go port can drop `Merge-ShellNativeResult` entirely for v1 (documented as a
  compatibility gap, see §8) without a JSON-bridging redesign, and only needs to be
  revisited if a future `main.yml` change starts relying on it — re-run this grep as part
  of Phase 3/4 sign-off in case usage was added since this plan was written.
- Handler registry mirrors `Get-PackageManagerHandlers`/`$script:NoCommandCheckModules`/
  `$script:ModuleCommandNames`: a `Registry` type built from a static list at `main.go`
  wiring time (`internal/handlers.All()` returning `map[string]Handler` plus per-module
  "needs a command on PATH"/"CLI name differs from module name" metadata), so the whole
  set stays a one-place-to-look list like today, not scattered `init()` side effects.

### 4.9 Task flattening & packages (`internal/tasks`, `internal/packages`)

Direct ports of `Tasks.psm1`/`Packages.psm1`: `Get-TaskList`/`Expand-TaskTree` (including
loop materialization order-of-evaluation subtleties — a loop's own `items`/`with`
resolves in the *enclosing* scope, everything else in the *new* per-iteration scope) and
`Import-PackagesHierarchy`/`Merge-VarsData`/`Merge-PackagesData`/`Import-IncludedPackage`
(vars deep-merge by key, task lists append, `include` isolates `PackageVars`/loop context;
see §2's correction that `include` is one root-relative `<name>/main.yml` resolver, not a
packages-vs-roles special case). Both get dedicated fixture-driven tests using real
`packages/*/main.yml`/`roles/*/main.yml`/`hosts/*/main.yml` content as input.

`Common.psm1`'s `Merge-FlatContext` precedence (`PackageVars` → site `Vars` → `Registry`,
last write wins, with `facts`/`inputs`/`package` nested rather than flattened) is the one
rule the whole "package default vs. site override vs. registry" system depends on and is
genuinely surprising (a site-level var can shadow a package's own local default of the
same bare name) — give it a first-class named fixture (package-local default, shadowed by
a site var, shadowed again by an `id`-registered result of the identical name) rather than
leaving it implied by other tests.

### 4.10 Engine / dispatch loop (`internal/engine`)

Direct port of `ironstate.ps1`'s `Invoke-PackageItem`/`Invoke-Tasks`/main flow: two-phase
run (facts pass, then everything else) threading one growing `Registry`/`UserFacts`/
`CommandAvailability` state forward; per-leaf `when` + remaining `${{ }}` resolution
immediately before dispatch; `failed_when`/`continue_on_error` semantics; looped-`id`
`.results[]` accumulation; command-availability caching per module (only `chocolatey`
remaps to a different CLI name today, per §2 — don't over-build this table). Also port,
explicitly (not as an afterthought): the two dry-run-forces-real-execution exceptions
(embedded-shell `fact`, every `assert`) and the fact `value` remove-before-run/
resolve-after-run choreography described in §2; and the "no handler / command not on
PATH" behavior producing **no result row at all**, not a `Skip` row. This package has the
most cross-cutting behavior and gets the most integration-level (not just unit) tests,
driven by whole fixture documents rather than isolated calls.

## 5. Compatibility strategy

1. **Schema is reconciled, then frozen.** Per §4.2, `ironstate.schema.json` is first audited
   against actual `ironstate.ps1` behavior (fixing any drift, e.g. the missing `gem`
   entry) before it's trusted as a Go-loader validation input; once reconciled, it does
   not change further without an explicit decision.
2. **Golden compatibility harness**: for every fixture under `testdata/` (curated subset
   of real `main.yml`/`hosts/*.yml`/`variables/*.yml`/`packages/*/main.yml`/`roles/*/main.yml`
   shapes, plus synthetic edge cases for every documented grammar corner — loops,
   `parent.item`, `failed_when`, `is`/`is not`, filter chains, `--tags` filtering, an
   embedded-shell `fact`, an `assert`, a fact-value self-reference, a `vars:`-referencing-
   `vars:` template case, a `hosts/`-rooted and a `roles/`-rooted `include`), run **both**
   `ironstate.ps1 -Tags ... ` (dry-run) and the Go binary in an equivalent dry-run mode,
   normalize both outputs (module/package/state/action columns, including **row absence**
   for a skipped-missing-command/no-handler leaf, not just state; `Exec.rc`/stdout are
   necessarily different in dry-run so they're excluded from the diff), and assert they
   match. This harness needs a Windows self-hosted or `windows-latest` GitHub Actions
   runner with PowerShell 7 present (already true for `windows-latest`).
3. **Known, called-out gaps** (tracked, not silently absorbed): `template.engine`/
   `blockinfile.template.engine` values `eps`/`herestring` are unsupported and error
   clearly (§4.7 — audited as unused anywhere in this repo today, so dropped rather than
   ported); `shell.host: pwsh` no longer merges a returned native object's properties
   in-process (§4.8 — audited as currently unused in this repo, so accepted as a v1 gap);
   `pwsh_version` fact's meaning shifts once the runner isn't
   PowerShell. Each gets a line in the README and a decision recorded before Phase 3/4
   sign-off (see [§8](#8-risks--open-questions)).
4. **No new required config** for existing users: `main.yml`/`hosts/`/`variables/` files
   need zero edits to work under the Go binary on day one.

## 6. Testing strategy

| Layer | Tooling | What it covers |
| --- | --- | --- |
| Unit | `go test` + table-driven cases | `internal/expr`, `internal/template`, `internal/filters` (incl. golden-vs-PowerShell fixtures, except `lookup`/`json_query` which use hand-written mocked tests — see §4.5), `internal/tasks`, `internal/packages`, per-handler `Test`/`Describe`/argv-building logic via fake `Runner` |
| Fuzz | `go test -fuzz` | `internal/expr` tokenizer/parser (string-literal escaping, unterminated `${{`/`}}` spans) — cheap given the grammar's small size, added from Phase 1 |
| Race | `go test -race` | Required specifically for `internal/filters` (persistent worker pool, §4.5) — the one package with real concurrency |
| Schema | `go test` against `ironstate.schema.json` | Every fixture + real repo YAML files validate cleanly, after the Phase 1 schema-vs-runtime reconciliation (§4.2/§5.1) |
| Integration | `go test` at `internal/engine` level | Whole fixture documents through the real dispatch loop (facts pass ordering, registry threading, loop `.results[]`, `failed_when`/`continue_on_error`, `--tags` filtering, the two dry-run-forces-execution exceptions, missing-handler row absence) with faked handlers/runner — no real installs |
| End-to-end (built binary) | `go test` invoking the compiled binary as a subprocess, or a small `bats`/PowerShell Pester script in CI | `--file`/`--apply`/`--tags` flag parsing, exit codes (per the decision recorded in §2/§11), `--output json` shape, `filters list`, `doctor` |
| Compatibility harness | GitHub Actions job on `windows-latest` | PS-vs-Go dry-run diff across `testdata/` fixtures (§5.2) |
| Lint/vet/security | `golangci-lint` (with an explicit policy for `gosec` G204 subprocess-with-variable-input findings, which will fire often given how much of this codebase shells out), `go vet`, `govulncheck` | Style, correctness, known-vulnerable dependency detection — required check before merge |
| Coverage | `go test -coverprofile` uploaded as a CI artifact (and optionally to Codecov) | No blanket minimum-% gate initially, **except** `internal/expr` and `internal/template` (the highest-risk packages per §4.3/§4.4), which get a targeted coverage floor from day one |

Windows-only handler tests (`registry`, `scheduled_task`, `winget`, `chocolatey`) are
build-tagged (`//go:build windows`) and only actually execute on the `windows-latest` leg
of the unit-test matrix — the `ubuntu-latest` leg exercises the OS-agnostic packages only,
not a symmetric full suite; state this explicitly in `ci.yml` rather than implying parity.

Mutation testing was considered and deliberately deprioritized: the golden-vs-PowerShell
compatibility harness already provides strong behavioral pinning for the highest-value
target (matching existing behavior), which is what mutation testing would otherwise be
used to validate test-suite strength for — revisit only if the harness itself proves
insufficient in practice.

All of the above run on every PR via `ci.yml`, **except** the compatibility harness, which
is gated by changed paths (see §9) rather than a label, so it can't silently stop running
as the project matures.

## 7. SBOM generation

- Use **`anchore/sbom-action`** (wraps `syft`) in the release workflow to generate a
  CycloneDX (`.cdx.json`) SBOM per built artifact, plus a repo-level SBOM from `go.sum`.
- Alternative/complement: **GoReleaser's built-in `sboms:` block** (uses `syft` under the
  hood) generates one SBOM per archive as part of the same release run — preferred over a
  separate action since it keeps SBOM-to-artifact mapping automatic and versioned
  together; use `anchore/sbom-action` only if a standalone repo-wide SBOM (not tied to a
  release artifact) is also wanted for continuous scanning.
- Attach SBOMs to the GitHub Release alongside binaries/checksums.
- Run `govulncheck` (Go-specific) separately in CI on every PR — catches vulnerable
  *code paths*, which SBOM+dependency-scanning alone doesn't.
- **Build provenance/attestation**, not just SBOM: cosign keyless-signing the
  checksums file (§8) proves the checksums came from *this* GitHub Actions run, but says
  nothing about *which* source commit/workflow produced the binary or whether the build
  is reproducible. Given this tool runs with elevated/administrative capability on
  end-user machines (registry writes, scheduled tasks, package installs), add
  **`actions/attest-build-provenance`** (GitHub-native SLSA-style provenance, low
  incremental effort on top of the release workflow already planned) alongside SBOM
  generation rather than treating SBOM + checksum-signing as sufficient supply-chain
  coverage on its own.
- Optionally feed the generated SBOM into GitHub's Dependency Graph /
  `github/codeql-action` or Grype for release-gate vulnerability scanning (stretch goal,
  not required for v1).

## 8. Binary packaging & release

- **GoReleaser** (`.goreleaser.yaml`) drives cross-compilation and packaging:
  - Targets: `windows/amd64`, `windows/arm64` (primary, matches current Windows-only
    scope); `linux/amd64`, `darwin/arm64`/`darwin/amd64` builds too, clearly documented as
    "engine-only / limited handler support" builds (Windows-only handlers return a clear
    unsupported-OS error rather than being compiled out, so the binary still runs and
    reports sensibly against a cross-platform subset of `main.yml`).
  - Archives: `.zip` for Windows, `.tar.gz` for Unix-likes; embed `README.md`,
    `LICENSE`, `ironstate.schema.json`.
  - Checksums file (`checksums.txt`, SHA256) signed with **cosign** (keyless/OIDC via
    GitHub Actions identity) for supply-chain verification — stretch but low-cost given
    GoReleaser has a built-in `signs:` block.
  - Version info injected via `-ldflags "-X main.version=... -X main.commit=... -X
    main.date=..."`, surfaced by `ironstate version`.
  - `sboms:` block (see §7) attaches a per-archive SBOM automatically.
- Release trigger: pushing a `vX.Y.Z` semver tag runs `release.yml`, which runs
  GoReleaser in `release` mode (not `--snapshot`), publishing to GitHub Releases.
- A `--snapshot` build runs on every `main` push (or nightly) purely to catch
  packaging/build breakage early, without publishing anything.
- Future stretch: winget/Chocolatey manifest auto-update PR against this tool's own
  package metadata once it's stable enough to self-host distribution that way — explicitly
  deferred, not part of v1.

## 9. GitHub Actions workflows

```
.github/workflows/
├── ci.yml            # on: push, pull_request
│   ├── lint (golangci-lint)
│   ├── vet + govulncheck
│   ├── unit tests: windows-latest (full suite, incl. //go:build windows-tagged
│   │     registry/scheduled_task/winget/chocolatey tests) + ubuntu-latest (OS-agnostic
│   │     packages only — not a symmetric full-suite run, see §6) + coverage artifact
│   ├── schema validation test (post schema-vs-runtime reconciliation, §4.2/§5.1)
│   └── build (goreleaser --snapshot, all targets) as a build-breakage smoke check
├── compat.yml        # on: pull_request (path-filtered: internal/expr/**, internal/template/**,
│   │                 #     internal/tasks/**, internal/packages/**, internal/engine/**),
│   │                 #     push to main, nightly schedule — required check on matching PRs,
│   │                 #     not label-gated (a label is too easy to forget as the project matures)
│   └── windows-latest: run PS-vs-Go dry-run diff harness across testdata/
├── release.yml       # on: tag push `v*`
│   ├── goreleaser release (build, archive, checksum, sign, SBOM)
│   ├── actions/attest-build-provenance (§7)
│   └── publish GitHub Release
└── codeql.yml        # on: push, pull_request, weekly schedule — Go CodeQL analysis
```

Supporting files: `dependabot.yml` (go modules + GitHub Actions weekly updates),
branch protection requiring `ci.yml` + `codeql.yml` green before merge, plus `compat.yml`
green specifically for PRs touching the path-filtered directories above.

## 10. Migration / rollout plan (phased)

| Phase | Scope | Exit criteria |
| --- | --- | --- |
| 0 — Scaffolding | `go.mod`, `cmd/ironstate`, cobra/viper wiring, CI skeleton (`ci.yml` running lint+vet+empty test suite), `.goreleaser.yaml` snapshot build | `ironstate version` builds and runs in CI on all target platforms |
| 1 — Core engine, no I/O | `internal/expr`, `internal/template`, `internal/filters` (built-ins only), `internal/model`, **schema-vs-`ironstate.ps1` reconciliation audit** (fix drift, e.g. missing `gem` entry, before trusting `ironstate.schema.json` as a validator input), schema validation test | Expression/template unit tests green (incl. fuzz targets); parity fixtures ported from README grammar examples; schema audit findings resolved or explicitly tracked |
| 2 — Document loading & flattening | `internal/packages`, `internal/tasks`, `internal/facts` | Real `main.yml` + all `hosts/`/`variables/` overlays (including `hosts/`-rooted and `roles/`-rooted `include`s) load, merge, and flatten to the expected leaf list (fixture-asserted) |
| 3 — Engine + no-op/low-risk handlers | `internal/engine` (incl. dry-run execution exceptions for embedded-shell `fact`/`assert`, missing-handler row absence), handlers: `log`, `path`, `fact`, `assert`, `file`, `copy`, `blockinfile`, `ssh_host_block`, `symlinks`, `zip` | Full dry-run of real `main.yml` across all hosts produces a stable, reviewed plan; §4.8's `shell.host: pwsh` audit re-confirmed against current repo state; `.env`/`.secrets` resolution behavior decided (§2/§11) |
| 4 — Package-manager handlers + `shell` + template engines | `winget`, `chocolatey`, `pipx`, `npm`, `cargo`, `go`, `gem`, `eget`, `registry`, `scheduled_task`, `shell` (incl. per-state fallback rules), `template` module w/ `jinja` (build-vs-buy spike first) and new `gotemplate` (Go stdlib `text/template`) | Dry-run parity harness (§5.2) green across all fixtures; any (currently nonexistent) real `eps`/`herestring` usage discovered late is migrated to `jinja`/`gotemplate` before this phase exits |
| 5 — Filter plugin system | External-script filter loader + shim + persistent worker pool (§4.5, race-tested), `filters list`/`doctor` commands | Existing `modules/Filters/*.ps1` run unmodified through the Go binary; a sample non-PowerShell script filter proves the interpreter-config hook works |
| 6 — Hardening | Full test matrix green, SBOM + provenance attestation + packaging + signing wired, docs updated (`README.md` rewritten for the Go binary, PowerShell version marked legacy, exit-code contract documented) | `release.yml` produces a signed, SBOM'd, attested `v0.x.0` pre-release artifact |
| 7 — Cutover | `-Apply` runs on a real (low-risk) host validated side-by-side against the PowerShell version; `ironstate.ps1` either removed or kept as a deprecated thin wrapper for one release cycle | Owner sign-off **and** a defined rollback trigger (see §11) with no unresolved diffs across N real-host `-Apply` runs; PowerShell modules archived/removed per decision recorded in §11 |

Each phase lands as its own PR/branch merged behind CI green, not one giant rewrite PR —
keeps review tractable and gives natural checkpoints to re-run the rubber-duck review.

## 11. Risks & open questions

- **`shell.host: pwsh` native-object merge** (§4.8): audited as **unused** anywhere in this
  repo's current YAML today, so v1 drops `Merge-ShellNativeResult` outright rather than
  building a JSON-bridging convention for it. Re-run the audit at Phase 3/4 sign-off in
  case usage was added since this plan was written; only build the JSON-bridging design
  if that re-audit finds a real consumer.
- **`eps`/`herestring` removal** (§1, §4.7): a repo-wide grep for `engine:\s*(eps|herestring)`
  found zero matches in any real `main.yml`/`hosts/*`/`packages/*`/`roles/*`/`tasks/*`
  content — only `jinja` is used (`roles/development/git/main.yml`,
  `tasks/wsl-shim/main.yml`). Both engines are dropped rather than ported or shelled out;
  `template`/`blockinfile.template` reject them with a clear "unsupported engine, migrate
  to jinja or gotemplate" error. Re-run this exact grep immediately before Phase 4 exit as
  a final safety check in case new `eps`/`herestring` usage was added to the repo since
  this plan was written — if so, that content must be migrated to `jinja`/`gotemplate`
  first, not used to reverse this decision.
- **Jinja library fidelity**: current `Jinja.psm1` is a *sandboxed subset* implementation,
  not a full Jinja2 engine — the chosen Go library may support more or less than what's
  actually used, or it may be lower-risk to port the existing small hand-rolled
  interpreter natively instead of adopting a third-party library (see §4.7's
  build-vs-buy framing). Needs a Phase 4 spike enumerating every Jinja construct actually
  present in the two real consumers (`roles/development/git/main.yml`,
  `tasks/wsl-shim/main.yml`) before picking either path.
- **`pwsh` availability assumption**: `eget`, `go install`, etc. still assume their own
  CLI is on PATH exactly like today (unchanged risk). With `eps` dropped and `jinja`/
  `gotemplate` both native Go, `shell.host: pwsh` is now the **only** remaining feature
  that needs `pwsh` present — call this out clearly in `ironstate doctor` and the README
  as the sole optional runtime dependency left.
- **YAML library behavioral differences**: `powershell-yaml` (libyaml-backed) vs.
  `gopkg.in/yaml.v3` may differ on edge cases (anchors/merge keys, number vs. string
  inference, multi-document files). Needs explicit fixture coverage for every YAML
  feature actually used in this repo's own files.
- **Windows admin/elevation + registry/scheduled_task**: Go's `golang.org/x/sys/windows`
  covers the needed APIs, but needs verification against the exact current behavior
  (`registry` module's multi-value writes, `scheduled_task`'s update-vs-recreate logic) —
  treat as a dedicated Phase 4 sub-task with its own fixture tests, not an afterthought.
- **Schema drift is a pre-existing condition, not something the rewrite introduces**:
  `ironstate.schema.json` is missing at least the `gem` module today. Treat the Phase 1
  reconciliation audit (§4.2/§10) as mandatory before the schema is used as a validation
  source of truth for the Go loader — otherwise the rewrite silently inherits and
  possibly amplifies an existing documentation gap.
- **Exit code contract is currently ambiguous** (`exit 1` covers both a deliberate
  `failed_when`-stopped run and any uncaught PowerShell error) — decide explicitly
  whether the Go binary preserves that ambiguity or introduces distinct codes, and treat
  a change here as a documented compatibility break like the `shell.host: pwsh` gap, not
  an implicit side effect of how `main()` happens to be written.
- **`.env`/`.secrets` resolution and binary distribution are coupled risks**: today's
  `<repoRoot>` (two directories above the script) dotenv convention only makes sense
  because `ironstate.ps1` is always run from inside a repo clone. Once a standalone Go
  binary can be distributed separately from a repo clone, "where do `.env`/`.secrets`
  live relative to the binary" and "how do users obtain the binary" (the pre-existing
  distribution open question) need a single combined decision, not two independent ones
  made in different phases.
- **Rollback trigger for Phase 7 (adopted, then superseded)**: this plan originally
  called for keeping `ironstate.ps1`/`modules/*.psm1` in the repo, deprecated in place,
  as a rollback path in case a real-host `-Apply` run turned up an unintended diff. The
  repo owner decided against that once cutover actually happened: this isn't a full
  release yet, so there's no compatibility window to protect, and git history already
  preserves the old implementation if it's ever needed again - `ironstate.ps1`/`modules/`
  were deleted outright rather than kept as a fallback. If a real-host `-Apply` run
  later turns up an unintended diff, the fix is to `git checkout` the old implementation
  from history for comparison and patch the Go handler, not to revert default usage to a
  live fallback binary that no longer exists in the working tree.
- **Distribution of the binary itself**: once `ironstate.ps1` is gone, how do users
  *get* the new binary on a fresh machine (today `ironstate.ps1` is just a file in a
  cloned repo)? Recommend: keep repo-clone-and-run as the primary path (binary built
  locally via a short `go install`/Makefile target, or checked into Releases and a tiny
  bootstrap fetch script) rather than requiring winget/Chocolatey self-packaging in v1.

## Revision log

- v0.1 — Initial draft.
- v0.2 — Incorporated rubber-duck subagent feedback (full critique preserved in session
  notes; not duplicated here). Key corrections and additions:
  - Fixed factual error: `include` is one generic, repo-root-relative `<name>/main.yml`
    resolver (verified against `Packages.psm1`/`ironstate.ps1` and real `hosts/`/`roles/`
    includes in this repo), not a packages-vs-roles special case.
  - Added the missing `gem`/RubyGem handler everywhere the module list appears, and
    flagged that `ironstate.schema.json` is already out of sync with runtime behavior (missing
    `gem`) — added an explicit Phase 1 schema-reconciliation task rather than assuming the
    schema is a safe source of truth as-is.
  - Documented previously-omitted subtleties required for real compatibility: the two
    dry-run-forces-real-execution exceptions (embedded-shell `fact`, `assert`) and the
    fact `value` remove-before-run/resolve-after-run choreography; the "no handler /
    command not on PATH" no-result-row (not `Skip`) behavior; the `facts.*`
    namespace-known special case in the soft template pass; the third,
    self-referential template-resolution pass in `Handlers/Template.psm1`; loop
    "boundary keys" isolation; `shell`'s per-state (`present`/`absent`/`latest`)
    field-by-field config fallback (with `absent` deliberately excluded); the shared
    `creates`-glob primitive between `zip`/`shell`; symlink path normalization;
    `Merge-FlatContext`'s precedence chain; `.env`/`.secrets` two-parent-directories-up
    loading convention; the `pwsh` self-relaunch behavior; and the current ambiguous
    exit-code contract.
  - Performed the `shell.host: pwsh` native-object-merge usage audit immediately
    (grepped the repo) instead of deferring it — found zero real usages today, so v1 can
    drop `Merge-ShellNativeResult` outright rather than designing a JSON-bridging
    workaround up front.
  - Strengthened testing (§6): added fuzzing for `internal/expr`, required `-race` for
    the filter worker pool, replaced non-hermetic golden-generation for `lookup`/
    `json_query` with hand-written mocked tests, clarified the Windows/Linux test-matrix
    asymmetry, added a targeted coverage floor for `internal/expr`/`internal/template`,
    and recorded mutation testing as considered-and-deprioritized rather than omitted.
  - Strengthened supply chain (§7): added GitHub-native build provenance attestation
    (`actions/attest-build-provenance`) alongside SBOM + checksum signing.
  - Changed `compat.yml`'s trigger from label-gated to path-filtered-required on PRs
    touching the core engine packages, so it can't silently stop running over time.
  - Added a `jinja` build-vs-buy framing (native port vs. third-party library) instead of
    defaulting straight to a library dependency.
  - Added a concrete Phase 7 rollback trigger instead of relying on "owner sign-off"
    alone.
- v0.3 — Dropped `eps`/`herestring` per explicit user decision, confirmed safe by a
  repo-wide grep for `engine:\s*(eps|herestring)` across every `.yml` file, which found
  zero real usages (only `jinja` is used, in `roles/development/git/main.yml` and
  `tasks/wsl-shim/main.yml`) — neither engine is a "core system" dependency. Changes:
  - Removed both engines from goals/non-goals (§1), the current-system summary (§2),
    the repo layout (§3), §4.7's engine table, the compatibility known-gaps list (§5),
    the Phase 4 migration row (§10), and the risks section (§11); `template.engine`/
    `blockinfile.template.engine` now treat `eps`/`herestring` as a hard, documented
    error in the Go binary rather than something ported or shelled out to `pwsh`.
  - Kept `jinja`, unchanged in scope/build-vs-buy framing, since it's the one engine
    with real production usage.
  - Added a new **`gotemplate`** engine (Go stdlib `text/template`, no external
    dependency) as an additive, non-compatibility-required option for new templates,
    alongside `jinja` (§4.7).
  - Updated the `pwsh` self-relaunch note (§11) and the "sole remaining runtime
    dependency" framing: with `eps` gone and both surviving engines native Go,
    `shell.host: pwsh` is now the only feature left that needs `pwsh` on the target
    machine (script-filter shims for `.ps1` filters, §4.5, are a separate, already-
    documented dependency).
