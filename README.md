# Windows Package Management

`ironstate.ps1` is a declarative, Ansible-style task runner driven by YAML. It reconciles each leaf action's desired `state` against what is currently installed, printing what it would do by default (dry-run) and only making changes when `-Apply` is passed.

## Usage

```powershell
# Dry-run — show what would change
.\ironstate.ps1

# Apply changes
.\ironstate.ps1 -Apply

# Restrict to tasks/actions carrying a tag
.\ironstate.ps1 -Tags cli
.\ironstate.ps1 -Tags cli,security -Apply

# Verbose output (shows skipped items, gathered facts, and which overlay files are merged)
.\ironstate.ps1 -Verbose -Apply
```

`ironstate.ps1` prefers PowerShell 7+ (`pwsh`): if it's launched under Windows PowerShell 5.1, it relaunches itself under `pwsh` (passing through all parameters) as long as `pwsh` is found on `PATH`, and only continues under 5.1 - with a warning - if `pwsh` isn't installed. This also means the `shell` module's default `host: pwsh` (see below) runs on 7+ whenever it's available, since the whole runner process is `pwsh` by that point.

## File hierarchy

Files are loaded and merged in this order. Each subsequent file's `tasks` list is **appended** to the base file's list; `vars` **deep-merges** key-by-key (a host/user overlay can add or override individual vars without wiping out the base set).

``` tree
install/windows/
├── site.yml              ← base (always loaded)
├── hosts/<COMPUTERNAME>.yml  ← merged if the file exists
└── variables/<USERNAME>.yml  ← merged if the file exists
```

`COMPUTERNAME` and `USERNAME` come from the environment variables of the same name. Overlay files use the explicit `tasks:`/`vars:` mapping form (not the bare-list form) so they have somewhere to merge into.

### Machine-specific overrides — `hosts/`

Create a file named after the machine's hostname to add or change tasks for that machine only.

```powershell
# Find your hostname
$env:COMPUTERNAME   # e.g. KRAYT
```

```yaml
# hosts/KRAYT.yml
tasks:
  - tags: [gaming]
    winget:
      package: Valve.Steam
      source: winget
      state: present
      # override: '/DIR="path"' # passes override arguments to the installer

  - tags: [cloud, cli]
    eget:
      package: rclone/rclone
      state: present
      args:
        - --to=~/.local/bin/rclone.exe
        - --upgrade-only
```

### User-specific overrides — `variables/`

Create a file named after the Windows username to add tasks or vars for a specific user account on any machine.

```powershell
# Find your username
$env:USERNAME   # e.g. ryan
```

```yaml
# variables/ryan.yml
vars:
  editor: nvim

tasks:
  - tags: [http, cli]
    pipx:
      package: posting
      state: present
```

## Architecture

`ironstate.ps1` is just the runner: it loads YAML, merges overlays, flattens the task/action tree (accumulating `tags` and `when` - see below - and loading any `include`d packages inline as it goes), applies `-Tags` filtering, then dispatches each surviving leaf to a handler *sequentially*, in the order the tasks are written (Ansible-style; not grouped by module). "Sequentially" matters: each leaf's `when` and any remaining `${{ }}` references are resolved *immediately before that leaf runs*, against facts/vars plus a registry that grows as earlier leaves' `id`/`fact` results come in - not all evaluated up front - so a task can act on an earlier task's result (see [Registering results](#registering-results-id)). The actual per-module logic (how to test/install/uninstall/describe a leaf) lives in its own PowerShell module:

``` tree
install/windows/
├── ironstate.ps1              ← runner (loads modules, no module-specific logic)
└── modules/
    ├── Common.psm1            ← shared helpers (state machine, flat tag matching, path resolution, 'creates' globbing, flat context merging)
    ├── Facts.psm1              ← gathered host facts (Get-InstallFacts)
    ├── Expressions.psm1        ← shared expression tokenizer/parser/evaluator (variable paths, comparisons, `is` tests, `|` filter pipeline) - used by both Conditions.psm1 and Templates.psm1
    ├── Conditions.psm1         ← 'when' expression evaluation (thin consumer of Expressions.psm1)
    ├── Tasks.psm1              ← task/action tree normalization + flattening (also expands 'include' and 'with'/'items' loops; 'when' is accumulated, not evaluated)
    ├── Templates.psm1          ← '${{ inputs.* }}' / '${{ package.* }}' / '${{ facts.* }}' / '${{ vars.* }}' / bare id/fact expression expansion (thin consumer of Expressions.psm1)
    ├── Packages.psm1           ← YAML loading, hierarchy merge, 'include' package loading
    └── Handlers/
        ├── Winget.psm1
        ├── Chocolatey.psm1
        ├── Pipx.psm1
        ├── Npm.psm1
        ├── Cargo.psm1
        ├── Go.psm1
        ├── Eget.psm1
        ├── Zip.psm1
        ├── Symlinks.psm1          ← thin wrapper over File.psm1's 'link' type
        ├── File.psm1
        ├── Copy.psm1
        ├── Shell.psm1
        ├── BlockInFile.psm1
        ├── Log.psm1
        ├── Path.psm1
        ├── Fact.psm1
        ├── Registry.psm1
        └── ScheduledTask.psm1
    └── Filters/
        ├── default.ps1
        ├── toggle.ps1
        ├── ternary.ps1
        ├── upper.ps1
        ├── lower.ps1
        ├── trim.ps1
        ├── quote.ps1
        ├── length.ps1
        ├── concat.ps1
        ├── join.ps1
        ├── split.ps1
        ├── prefix.ps1
        ├── dirname.ps1
        ├── basename.ps1
        ├── resolve.ps1
        ├── exists.ps1
        ├── sha1.ps1
        ├── lookup.ps1
        ├── from_json.ps1
        └── json_query.ps1
```

Each `Handlers/*.psm1` file exports a single `Get-<Module>Handler` function returning a `Test`/`Describe`/`Install`/`Uninstall` set of script blocks - unchanged in shape from before. To add a new module, drop a new module in `Handlers/`, register it in `Get-PackageManagerHandlers` in `ironstate.ps1`, and add it to `$script:NoCommandCheckModules` if it isn't backed by an external CLI.

Each `Filters/*.ps1` file is a standalone script - just its own `param($Value, [object[]] $ArgValues)` block plus logic - named for the filter it implements (`upper.ps1` → the `upper` filter). `Expressions.psm1` loads every file in `Filters/` at import time, so adding a new `|` filter is just adding a file there - no registration step.

## Task/action model

A document is either the explicit form:

```yaml
tasks:
  - name: install age via winget
    tags: [security, cli]
    when: condition == true
    actions:
      - name: Log a message
        log:
          message: "Hello, world!"
          level: info
      - name: foo
        winget:
          package: FiloSottile.age
          source: winget
          state: present
```

or, when you don't need `vars` in that file, the document root can just **be** the list directly:

```yaml
- name: install age via winget
  tags: [security, cli]
  actions:
    - name: Log a message
      log: { message: "Hello, world!" }
    - name: foo
      winget: { package: FiloSottile.age, source: winget, state: present }
```

Every item in a task list is classified independently:

- **Has an `actions:` key** → it's a *grouping task*. Its `name`/`tags`/`when` scope the whole subtree (nested to any depth, like Ansible blocks).
- **Has an `include:` key** → same as `actions:`, except the subtree is loaded from `packages/<name>/main.yml` instead of written inline. See [Includes](#includes).
- **Neither** → it *is* a leaf action itself, directly. Its one recognized module key (`winget`, `copy`, `log`, ...) is what runs. This is why a bare list of standalone actions and a list of tasks are the same mechanism - a "task" with no `actions:` and a "bare action" are identical in shape:

```yaml
- name: Log a message
  log:
    message: "Hello, world!"
    level: info
- name: foo
  winget:
    package: FiloSottile.age
    source: winget
    state: present
```

**`tags` cascade down**: a leaf's effective tags are its own `tags` plus every ancestor task's `tags`, unioned. In the first example above, neither action declares its own tags, so both inherit `[security, cli]` from the enclosing task.

**`when` cascades via AND**: a leaf only runs if every ancestor task's `when` *and* its own `when` all evaluate true.

## Looping (`with`/`items`)

A task carrying `with` or `items` is materialized multiple times *before* anything else about it is looked at - name, tags, when, id, its module fields, or a nested `actions`/`include` (looping a whole block, like Ansible). Each copy gets `${{ item }}`/`${{ item.<key> }}` resolved against one value:

- **`items`** - a list; one materialized copy per entry (Ansible `loop`-style).
- **`with`** - a single value; exactly one copy, without iterating ("use this as a reference" rather than a loop). Ignored if `items` is also set.

```yaml
tasks:
  - name: install ${{ item.name }}
    winget:
      package: ${{ item.package }}
      source: ${{ item.source }}
      state: ${{ item.state }}
    with:
      name: WinToys
      package: 9P8LTPGCBZXD
      source: msstore
      state: present

  - name: install ${{ item.package }}
    eget:
      package: ${{ item.package }}
      state: ${{ item.state }}
      args: ${{ item.args }}
    items:
      - package: sharkdp/fd
        state: present
        args:
          - --to=~/.local/bin/fd.exe
      - package: another/tool
        state: absent
```

The second example's `another/tool` entry has no `args` - resolving `${{ item.args }}` for that iteration warns and **omits the field entirely** (falls back to `eget`'s own default), rather than substituting an empty string of the wrong type. This omit-on-unresolved behavior applies to any field whose *entire* value is a single unresolved `${{ }}` reference, not just `item.*` - see [Template expressions](#template-expressions).

Note that `with` never iterates, even if the value you give it happens to be a list - it's always exactly one copy, with `item` bound to that whole value. If you want one copy per list entry, that's what `items` is for.

If a looped task also has an `id`, every iteration shares that same name - see [`id` on a looped task](#id-on-a-looped-task).

### Nested loops (`parent`)

An `actions:` entry inside a loop can itself carry `with`/`items`, looping again per outer iteration. Doing so rebinds `item` to the inner value - the outer value would otherwise become unreachable - so it's exposed instead as `${{ parent.item }}` / `${{ parent.item.<key> }}`. A third level of nesting chains one more `parent`: `${{ parent.parent.item }}`, and so on for however many loops deep you go.

```yaml
tasks:
  - name: for each user
    items: "${{ vars.users }}"       # e.g. [{ name: alice, repos: [a, b] }, { name: bob, repos: [c] }]
    actions:
      - name: for each of this user's repos
        items: "${{ item.repos }}"
        log:
          message: "${{ parent.item.name }} owns ${{ item }}"
```

This prints `alice owns a`, `alice owns b`, `bob owns c`.

**Gotcha:** a loop task's own `items`/`with` expression is resolved *before* that loop exists - in the enclosing scope, where `item` still refers to the outer value (there's no `parent` to reach yet at that point, since the inner loop hasn't started). So the inner loop's own `items: ${{ item.repos }}` above is correct; writing `items: ${{ parent.item.repos }}` there would be wrong (and, with no third loop enclosing it, `parent` wouldn't even be defined). Everything *else* on that same task - `when`, `tags`, its module fields, or a further-nested `actions`/`include` - resolves afterward, inside the loop's own per-iteration pass, where `item` has already rebound and `parent` is available. In other words: a loop task's `items`/`with` line looks "outward" (current scope), everything else about that task looks "inward" (its own new scope, one level past `parent`).

## `when` conditions

`when` accepts a single condition string, or a list of strings (list = implicit AND, matching Ansible). Conditions are bare expressions - no `${{ }}` wrapper - evaluated against a flat context of gathered **facts**, user-defined **vars** (see below; vars win on name collision), and any **`id`-registered results**/**`fact`** values from earlier tasks (see [Registering results](#registering-results-id)).

Grammar (lowest to highest precedence):

```text
expr       := or_expr
or_expr    := and_expr ("or" and_expr)*
and_expr   := not_expr ("and" not_expr)*
not_expr   := "not" not_expr | comparison
comparison := membership (("==" | "!=" | "<" | "<=" | ">" | ">=") membership)?
membership := pipeline (("in" | "not in") primary | ("is" | "is not") IDENT)?
pipeline   := primary ("|" IDENT ("(" (expr ("," expr)*)? ")")?)*   # filter chain, e.g. value | default("x") | upper
primary    := STRING | NUMBER | "true" | "false" | "null"
            | "[" (expr ("," expr)*)? "]"
            | IDENT (("." IDENT) | ("[" NUMBER "]"))*   # dotted/indexed path, e.g. nested.value, results[0].rc
            | "(" expr ")"
```

This grammar lives in `modules/Expressions.psm1` and is shared with `${{ }}` template expressions (see [Template expressions](#template-expressions)) - anything documented here, including filters, also works inside `${{ }}`, and vice versa.

- String comparisons (`==`, `!=`, ordering) are **case-sensitive**, matching Ansible/Jinja.
- `in`/`not in`: right-hand side a list → membership check; a string → substring containment.
- A bare variable with no operator is truthy-checked directly (Ansible-style `when: some_var`).
- `is`/`is not` type-tests: `mapping` (alias `map`), `boolean` (alias `bool`), `string`, `number`, `list`, `defined`, `none` (alias `null`). Use these instead of `==`/truthy checks when you need to know a value's actual type rather than whether it casts to `true` - notably, `some_map == true` is **also true for any non-empty mapping**, since both go through the same truthy cast, so `is mapping`/`is boolean` are the only reliable way to tell "a boolean `true`" apart from "a map" (see `packages/languages/main.yml`, where a var can be either a blanket `true` or a per-key map).
- `value | filter(args)` pipes a value through a named filter, Jinja-style, and chains left-to-right (`value | trim | upper`). Filters bind tighter than comparisons, so `java_version | default("25") == "25"` filters first, then compares. Each filter is a standalone script in `modules/Filters/`, named for the filter itself (e.g. `upper.ps1` is the `upper` filter) and loaded automatically at import - add a new filter by adding a file there, nothing else to register. Unless noted otherwise, every filter is **null-in/null-out**: a `null` piped value passes straight through rather than erroring.

  | Filter | Arguments | Result |
  | --- | --- | --- |
  | `default` | `fallback` | The piped value if it's non-null, else `fallback`. |
  | `toggle` | `fallback` | The piped value if it's a **string**, else `fallback` - for a var that's normally an on/off boolean flag but may instead be set to a string to name a specific override, e.g. `jdk: true` vs. `jdk: 'Eclipse.Temurin.21'` (a bare `true`/`false` both fall back, same as `null` - see the `packages/languages/java` example below). |
  | `ternary` | `whenTrue, whenFalse` | `whenTrue` if the piped value is truthy, else `whenFalse` (does **not** pass `null` through - always returns one of its two arguments). |
  | `enabled` | `...keys` (each a plain string, optional) | Collapses the repeated `(X is mapping and X.Y) or (X is boolean and X)` on/off-toggle pattern into one call, at any depth: `productivity \| enabled('browsers', 'chrome')` walks `productivity.browsers.chrome` key by key. At each step, a **boolean** stops the walk immediately and *is* the answer - `true` enables everything below it, `false` disables everything below it regardless of a deeper key's own value (an explicit disable always wins); a **mapping** keeps descending. Once every key is consumed (or none were given - a bare `productivity \| enabled` checks `productivity` itself), the value reached counts as "on" if it's `true` **or still a mapping** - present/configured in some structured way, even with no specific leaf checked yet. That last part matters for a wrapping/gating task deciding whether to even look at a section at all, e.g. `productivity \| enabled('browsers')` is `true` as soon as `productivity.browsers` exists as a mapping of individual toggles, without needing one specific browser's key. Anything else (a missing key, a non-mapping in the way, or a plain string/number at the end) is `false`. Can't be written as `productivity.browsers.chrome \| enabled()` with no arguments - a filter never sees the surrounding variable context, only the value piped into it, so by the time that whole path resolves to one value, a boolean ancestor has already collapsed everything below it to `null`, losing whether it was `true` or `false`. |
  | `upper` / `lower` | none | Uppercase/lowercase the string, invariant culture. |
  | `trim` | none | Trims leading/trailing whitespace. |
  | `quote` | `quoteChar` (optional, default `"`) | Wraps the value in `quoteChar` on both sides. A blank/whitespace-only value becomes `null` instead of quoting an empty string. |
  | `length` | none | `.Length` of a string, or element count of an array; `0` if the value is `null`. |
  | `concat` | `delimiter, ...extraItems` | Joins the piped value with `delimiter`: an array value has its elements joined; a scalar value is treated as one element. Any `extraItems` after `delimiter` are appended before joining - e.g. `"hello" \| concat(" ", "world")` → `"hello world"`. Use this for joining a list into one delimited string - see `join` below for building a filesystem path instead. |
  | `join` | `...parts` | Combines the piped value and every argument as **filesystem path segments** via `[System.IO.Path]::Combine` (like Python's `os.path.join`), e.g. `"~/base" \| join("sub", "file.txt")`. Not a delimiter-join - use `concat` for joining a list into a string. |
  | `split` | `delimiter` | Splits a string into an array on a literal delimiter - the inverse of `concat`. Drops one trailing empty element, so a value `concat` produced round-trips. |
  | `prefix` | `text` | Prepends `"text "` (one space) to the value, or to every element if the value is an array - e.g. `item.key \| split("\n") \| prefix(item.hostnames \| concat(" "))` builds `"hostname key-line"` pairs from a multi-line key blob. |
  | `dirname` / `basename` | none | Parent directory / file name of a path string (`[System.IO.Path]::GetDirectoryName`/`GetFileName`). |
  | `resolve` | none | Expands the value through `Resolve-UserPath` (e.g. `~` expansion). |
  | `exists` | `expected` (optional bool, default `true`) | Tests whether the value - a path string, `FileInfo`, or `DirectoryInfo` - exists on disk. Returns `expected` when it does, `not expected` otherwise (including for `null`/blank). |
  | `sha1` | none | Lowercase hex SHA-1 hash of a string value; throws on a non-string, non-null value. Handy for a deterministic `blockinfile` `marker_name` derived from content that varies, e.g. `${{ item.public_key \| sha1 }}`. |
  | `lookup` | `action, ...pieces` | Fetches external content: `lookup('url', ...)` does a `GET` and returns the body; `lookup('file', ...)` reads a local file (`~` expanded). Every argument after `action` is concatenated together into the URL/path - if *any* piece is `null`/empty, the whole call returns `null` instead of composing a wrong target (e.g. a missing per-item value silently skips the lookup rather than requesting a broken URL). |
  | `from_json` | none | Parses a JSON string into a PowerShell object (`ConvertFrom-Json`) - typically piped into `json_query` next. |
  | `json_query` | `query` | Queries a PowerShell object (usually piped from `from_json`) for a nested value. Uses `jq` syntax (e.g. `.ssh_keys`) if `jq` is on `PATH`; otherwise falls back to `Select-Object -ExpandProperty <query>`, which only supports a single bare property name, not full `jq` filter syntax. |

```yaml
tasks:
  - name: only on this machine
    when: computer_name == "KRAYT"
    winget: { package: Valve.Steam, state: present }

  - name: only for admins, unless disabled
    when:
      - is_admin == true
      - not (vars_disabled == true)
    shell: { command: "..." }

  - name: any of several tags
    when: "'gaming' in tags or 'media' in tags"
    log: { message: "a gaming or media var is set" }

  - name: languages.rust - either a blanket "true" or a per-key map
    when:
      - (languages is mapping and languages.rust) or (languages is boolean and languages)
    include: { name: languages/rust, state: present }
```

### Facts

Gathered fresh every run; a deliberately small, easy-to-extend starter set (see `modules/Facts.psm1`):

| Fact | Description |
| --- | --- |
| `computer_name` | `$env:COMPUTERNAME` |
| `user_name` | `$env:USERNAME` |
| `home` | `$HOME` |
| `os_version` | `[System.Environment]::OSVersion.Version` |
| `os_build` | OS build number |
| `is_admin` | Whether the current process is running elevated |
| `pwsh_version` | `$PSVersionTable.PSVersion` |

### Vars

User-defined, under a top-level `vars:` mapping (merges across `site.yml`/`hosts/`/`variables/` - see [File hierarchy](#file-hierarchy)):

```yaml
vars:
  enable_extra_tools: true
  editor: nvim
```

Facts and vars share **one flat namespace** in `when` (bare `facts.<key>`/`vars.<key>`, vars win on collision at the top level). They're also available to `${{ }}` string templating with the same prefix: `${{ facts.computer_name }}`, `${{ vars.editor }}` (alongside the existing `${{ package.* }}`/`${{ inputs.* }}` used by modular packages - see [Template expressions](#template-expressions)).

`id`-registered results join this same flat namespace, **without** a prefix, in either `when` *or* `${{ }}` - see the next section. `fact` values are the one exception: they're registered under `facts.<name>` (see [`fact`](#fact)), not bare, since a user-defined fact is a counterpart to gathered facts, sharing that same namespace - `${{ facts.pwsh_system_profile }}`, not `${{ pwsh_system_profile }}`.

## Registering results (`id`)

Give any leaf action an `id:` and, once it runs, its result is registered under that name - matching Ansible's `register`, just spelled `id`:

```yaml
tasks:
  - name: check for a file
    id: foo_stat
    shell:
      command: Get-Item -Path "C:\foo\file.txt"

  - name: install foo parser
    when: foo_stat.rc != 0
    winget:
      package: foo.parser
      state: present
```

The registered value is shaped like an Ansible registered variable:

| Field | Description |
| --- | --- |
| `changed` | Whether this leaf resolved to `Install`/`Uninstall` (not `Skip`) |
| `failed` | Whether this leaf counted as failed - see [Failing a task](#failing-a-task-failed_when-continue_on_error) |
| `rc` | Exit code. Real for every CLI-backed module (`winget`/`chocolatey`/`pipx`/`npm`/`cargo`/`go`/`eget`) and `shell`; every pure-PowerShell module defaults to `0` unless it throws (then `1`) |
| `stdout` / `stdout_lines` | Captured stdout (joined / split on newlines). Real for the same modules as `rc`; `''`/`[]` otherwise |
| `stderr` / `stderr_lines` | Captured stderr, same as above (a thrown exception's message lands here) |

Reference it with a bare dotted path in `when` (`foo_stat.rc != 0`, no `${{ }}` wrapper - same grammar as facts/vars) or with `${{ }}` in a template (`${{ foo_stat.stdout }}`, or `${{ foo_stat }}` for the whole object).

### `id` on a looped task

An `id` on a task that also has `items` (see [Looping](#looping-withitems)) is shared by every iteration - naively overwriting the same registry entry each time would silently keep only the last iteration's result. Instead, the flat fields (`changed`/`rc`/`stdout`/...) reflect the **last** iteration, and every iteration's result is *additionally* available as `.results[N]` (0-indexed, in iteration order) - both `when` and `${{ }}` support `[N]` indexing into a list, interspersed with dotted access:

```yaml
tasks:
  - name: run for each message
    id: example_task
    shell:
      command: "echo ${{ item }}"
    items:
      - "Hello, world!"
      - "Goodbye, world!"

  - name: check the first iteration specifically
    when: example_task.results[0].rc == 0
    log:
      message: "first result was ${{ example_task.results[0].stdout }}, last was ${{ example_task.rc }}"
```

A task with `id` but no loop never gets a `.results` key - it stays exactly as it was before this existed.

**Ordering matters**: this only works because tasks are dispatched sequentially and each one's `when`/`${{ }}` are resolved *immediately before* it runs (not all up front) - see [Architecture](#architecture). Referencing an `id` that hasn't executed yet (written later in the file, skipped by an earlier `when`/`-Tags`, or never run because it's a `-Apply`-gated real command evaluated during a dry-run) resolves to `$null`/the zero-value defaults, not an error - write comparisons defensively (e.g. `foo_stat != null and foo_stat.rc != 0`) if that distinction matters. In dry-run, `changed` is still accurately predicted for every module (it doesn't require actually running anything), but `rc`/`stdout`/`stderr` stay at their zero-value defaults for every module since nothing actually runs without `-Apply`.

`fact` (see below) is the write-your-own-value counterpart to `id`: it always actually applies (dry-run included), since it's pure bookkeeping with no real system side effect - useful when you want a value available for later `when`/`${{ }}` previews without gating it behind an `id`'d real action. Unlike `id`, a fact registers under the `facts.<name>` namespace (see [Facts](#facts)/[Vars](#vars)), not bare.

### Native `pwsh` results

An `id`'d `shell` task under the default `host: pwsh` doesn't just capture text - if the command's only real output (ignoring `Write-Host`/`Write-Information`) is a single object, that object's own properties are merged directly into the registered result, right alongside `rc`/`stdout`/etc. (a real property is never overwritten by one of those reserved names). So a task that runs a plain PowerShell expression returning a rich object can have its fields referenced directly, no extra indirection:

```yaml
tasks:
  - name: get program files directory
    id: pf
    shell:
      command: (Get-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion")

  - name: set program files directory fact
    fact:
      name: program_files_dir
      value: ${{ pf.ProgramFilesDir }}   # -> referenced later as ${{ facts.program_files_dir }}
```

This only applies to `host: pwsh` (the only host with a real "object" concept - an external process like `cmd`/`node`/`npx tsx` only ever produces text) and only when exactly one such object comes out; a command producing plain text, zero objects, or more than one object just gets the usual `rc`/`stdout`/`stdout_lines`/`stderr`/`stderr_lines` shape.

## Failing a task (`failed_when`, `continue_on_error`)

By default, a leaf counts as failed exactly when its handler reports a non-zero `rc` (a thrown error also counts, as `rc: 1` with the exception's message in `stderr`). `failed_when` overrides that: same grammar as `when` (a single expression, or a list, implicit AND), evaluated against *this leaf's own* `rc`/`stdout`/`stdout_lines`/`stderr`/`stderr_lines`/`changed` as bare names - no `${{ }}` wrapper, no `id` needed:

```yaml
tasks:
  - name: run a health check
    failed_when: rc != 0 or 'ERROR' in stdout
    shell:
      command: C:\tools\healthcheck.exe
```

What happens next depends on `continue_on_error`:

- **Unset (default)**: a failed task stops the whole run immediately - remaining tasks never dispatch, and `ironstate.ps1` exits with a non-zero code. Matches Ansible's default (no `ignore_errors`).
- **`continue_on_error: true`**: the failure is logged and the run moves on to the next task - matches Ansible's `ignore_errors: true`. The task's result (and its `id`-registered value, if any) still reports `failed: true`, so a later task can react to it.

```yaml
tasks:
  - name: best-effort cleanup
    continue_on_error: true
    shell:
      command: Remove-Item C:\maybe\missing.tmp -ErrorAction Stop
```

## Supported modules

| Module | Manager / mechanism |
| --- | --- |
| `winget` | Windows Package Manager (`winget`) |
| `chocolatey` | Chocolatey (`choco`) |
| `pipx` | Python isolated tools (`pipx`) |
| `npm` | Node global packages (`npm -g`) |
| `cargo` | Rust crates (`cargo install`) |
| `go` | Go binaries (`go install`) |
| `eget` | GitHub release binaries (`eget`) |
| `zip` | Download + extract ZIP (no external tool) |
| `symlinks` | Symbolic links (no external tool) |
| `copy` | Copy a local file into place (no external tool) |
| `shell` | Run an inline PowerShell command or script file (no external tool) |
| `blockinfile` | Insert/update/remove a marker-delimited block of text in a file (no external tool, modeled on Ansible's `blockinfile`) |
| `log` | Print a message at a chosen level (no external tool) |
| `path` | Add/remove directories on the current user's `PATH` (no external tool) |
| `fact` | Set an arbitrary named value for later tasks to reference (no external tool) |
| `registry` | Write one or more named values under a registry key (no external tool) |
| `scheduled_task` | Register/update/remove a Windows Task Scheduler task (`ScheduledTasks` module) |
| `include` | Pull in another document's tasks from `packages/<name>/main.yml` (no external tool) |

Every leaf shares these envelope fields, which sit *beside* its module key, not inside it:

| Field | Values | Description |
| --- | --- | --- |
| `name` | string | Label used in output |
| `tags` | list of strings | Used with `-Tags`; cascades down from ancestor tasks |
| `when` | string or list of strings | Gate(s); AND'd with ancestor tasks' `when` |
| `id` | string | Registers this leaf's result for later tasks to reference - leaf actions only. See [Registering results](#registering-results-id) |
| `items` / `with` | list / any value | Materializes this task once per entry (`items`) or exactly once (`with`), templating `${{ item.* }}` first. See [Looping](#looping-withitems) |
| `failed_when` | string or list of strings | Overrides whether this leaf counts as failed. See [Failing a task](#failing-a-task-failed_when-continue_on_error) |
| `continue_on_error` | boolean, default `false` | Keep running past a failed leaf instead of stopping the run. See [Failing a task](#failing-a-task-failed_when-continue_on_error) |

Each module's own fields (documented inline in `site.yml`) still include `state` (`present`/`absent`/`latest`, default `present`).

### `zip`

| Field | Required | Description |
| --- | --- | --- |
| `src` | yes | URL of the ZIP archive |
| `dest` | yes | Directory to extract into (`~` expansion supported) |
| `creates` | yes | Glob patterns whose presence signals "already installed" |
| `include` | no | Filename patterns to extract (whitelist) |
| `exclude` | no | Filename patterns to skip (blacklist) |
| `sha256.cache` | no | Path to cache the downloaded archive's SHA256 hash; used by `state: latest` to skip unchanged archives |

### `symlinks`

A thin wrapper over `file` (`type: link`) - see below. Kept as its own module for the simpler `src`/`dest` shape.

| Field | Required | Description |
| --- | --- | --- |
| `src` | yes | Link target (`~` expansion supported) |
| `dest` | yes | Link path (`~` expansion supported) |
| `force` | no | Replace whatever already exists at `dest` if it isn't already the right symlink. Default `true` - unlike `file`'s own `force` (default `false`), this preserves the original always-replace behavior. Set `false` to warn and skip instead |

### `file`

Modeled on Ansible's [`file`](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/file_module.html): manages a path as a plain file, directory, symlink, or hard link.

| Field | Required | Description |
| --- | --- | --- |
| `path` | yes | Path to manage (`~` expansion supported) |
| `type` | no | `file` (default, creates an empty file if missing, no-op otherwise), `directory` (creates it and any missing parents), `link` (symlink to `src`), `hard` (hard link to `src`), `touch` (always updates the timestamp, creating an empty file first if missing - like Unix `touch`) |
| `src` | one of `type: link`/`type: hard` | Existing path the link points to |
| `force` | no | When `path` already exists as something other than `type`, replace it. Default `false` - warns and skips instead |

**Note:** `state` here is this codebase's usual `present`/`absent`/`latest` (see [Task/action model](#taskaction-model)) - unlike Ansible's own `file` module, which overloads `state` to also mean the target kind (`file`/`directory`/`link`/`hard`/`touch`) plus `absent`. That's what `type` is for instead, so this module's dispatch stays consistent with every other handler. `absent` removes whatever is at `path` - recursively for a real directory, but a link/hard link is only ever unlinked, never recursed into, so removing a directory symlink can't delete the target's contents.

```yaml
tasks:
  - name: custom PowerShell scripts directory
    file:
      path: ~/.config/powershell/custom
      type: directory

  - name: pin a specific config as the active one
    file:
      path: ~/.config/app/active.json
      type: link
      src: ~/.config/app/profiles/work.json
```

### `copy`

| Field | Required | Description |
| --- | --- | --- |
| `src` | yes | Source file or directory. Resolved relative to the install system directory (`install/windows`, or the owning package's own directory — see [Includes](#includes)) unless it's absolute or `~`-prefixed |
| `dest` | yes | Destination path (`~` expansion supported) - a directory when `src` is a directory |

`present`/`latest` copy `src` over `dest` whenever their SHA256 hashes differ; `absent` removes `dest`.

If `src` is a directory, every file under it is copied recursively, preserving subdirectory structure - rsync/Ansible style: a trailing `/` on `src` (e.g. `files/custom/`) copies its *contents* directly into `dest`, while no trailing slash (e.g. `files/custom`) nests it as `dest/custom/...` instead. `dest` is created as a directory either way. `present`/`latest` compare each file's SHA256 hash individually (extra files already in `dest` that aren't under `src` are left alone - this isn't a mirror); `absent` removes only the files this task copied, then prunes any subdirectories left empty as a result - never `dest` itself.

```yaml
tasks:
  - name: custom PowerShell source files
    copy:
      src: "files/home/.config/powershell/custom/"  # trailing slash: contents only
      dest: "~/.config/powershell/custom"
      state: present
```

### `template`

| Field | Required | Description |
| --- | --- | --- |
| `src` | yes | Template source file. Resolved the same way as `copy.src` |
| `dest` | yes | Destination path for the rendered output (`~` expansion supported) |
| `engine` | yes | Which template engine renders `src`: `jinja`, `eps`, or `herestring` (see below) |
| `vars` | no | Extra key/value pairs layered on top of facts/vars/registry for this render only (last-write-wins, shallow merge) |

`present`/`latest` render `src` and write it to `dest` whenever the freshly-rendered content differs from what's already there (a plain string compare - the template equivalent of `copy`'s SHA256 hash compare); `absent` removes `dest`. The render context is the same facts/vars/id-registry context `when`/`${{ }}` already resolve against, with this task's own `vars` layered on top.

Three engines, in increasing order of power (and decreasing sandboxing):

- **`jinja`** - a sandboxed, block-capable engine built on the same expression grammar as `when`/`${{ }}` (see [Template expressions](#template-expressions)): `{{ expr }}` output, `{% if %}`/`{% elif %}`/`{% else %}`/`{% endif %}`, `{% for x in iterable %}`/`{% endfor %}` (nesting supported), and `{% set x = expr %}`. No cmdlet/command invocation exists in the grammar at all, so a template can only read/compare/filter context values - never cause a side effect.
- **`eps`** - the real PowerShell Gallery [`EPS`](https://github.com/straightdave/eps) module: `<%= expr %>` (output), `<%# ... %>` (comment), and `<% ... %>` (scriptlet - arbitrary PowerShell statements, including control flow). Installs the `EPS` module on first use if it isn't already present (`Install-Module -Scope CurrentUser`). Unlike `jinja`/`herestring`, this is **full, unrestricted PowerShell** - there is no sandboxed mode in the upstream module (its `-Safe` switch only isolates variable scope, not what the template's code can do).
- **`herestring`** - looks like a PowerShell expandable here-string, but isn't one: bare `$Name.Path` and `$(...)` interpolation only, evaluated through the same sandboxed grammar as `jinja` - no block constructs. The simplest tier, for plain variable substitution.

```yaml
tasks:
  - name: Render global gitconfig
    template:
      src: templates/gitconfig.j2
      dest: ~/.gitconfig
      engine: jinja

  - name: Render per-profile gitconfig
    items: ${{ vars.git.profiles | default([]) }}
    template:
      src: templates/gitconfig.profile.j2
      dest: ~/.gitconfig-${{ item.name }}
      engine: jinja
      vars:
        profile: ${{ item }}
```

### `shell`

| Field | Required | Description |
| --- | --- | --- |
| `command` | one of `command`/`script` | Inline script content, written to a temp file and run. Use a YAML block scalar (`\|`) for multiline scripts |
| `script` | one of `command`/`script` | Path to an existing file to run instead, resolved the same way as `copy.src` |
| `host` | no | What runs `command`/`script`, like a shebang line. Default `pwsh` runs it directly, in-process. Presets `powershell`, `cmd`, `bash`, `sh`, `node`, `python` expand to their executable. Anything else is split on whitespace and used as exe + leading args - e.g. `npx tsx` - so any script runner on PATH works without code changes |
| `extension` | no | Overrides the temp file's extension for inline `command` under a non-`pwsh` host (defaults to a sensible one per preset, `.txt` otherwise) - e.g. `.ts` so `npx tsx` sees real TypeScript |
| `args` | no | List of arguments passed to the command/script |
| `creates` | no | Glob patterns whose presence means "already run". Without it, `present`/`latest` always re-run the command. For `absent`, these paths are removed instead of the command running again |

**Per-state command/script (scripted install/uninstall)**: `command`/`script`/`args`/`host`/`extension` can instead be nested one level deeper, under `present`/`absent`/`latest` keys, to run a different command per state:

```yaml
tasks:
  - name: scripted tool
    shell:
      present: { command: ./install.ps1 }
      absent:  { command: ./uninstall.ps1 }
      creates: [~/.tools/thing/installed.marker]
```

The block for the item's own `state` (default `present`) is picked, and falls back field-by-field to the flat top-level fields for anything that block doesn't set - so plain top-level `command`/`script` (no per-state blocks at all) keeps working exactly as before. `absent` is the one exception with **no** top-level fallback: the legacy behavior for `state: absent` has always been "remove `creates` entries, run nothing", so reusing the top-level (present-oriented) command by default would be a surprising, silent behavior change. Write a dedicated `absent` block to run something on uninstall; without one, `absent` still only removes `creates` entries, same as today. `creates` itself is always read from the top level regardless of any per-state block, since it's the one signal that decides which state to reconcile *to* in the first place.

Output is captured, not streamed live: stdout (`Write-Host`/`Write-Output`/real native stdout, all merged together) and stderr are captured, then echoed after the command finishes (stdout via `Write-Host`, stderr via `Write-Warning`) - so it still ends up on the console, just after completion rather than in real time. This is what backs an `id`'d shell task's `rc`/`stdout`/`stdout_lines`/`stderr`/`stderr_lines` (see [Registering results](#registering-results-id)).

Under the default `pwsh` host specifically, that capture preserves real object identity rather than flattening everything to text: if the command's only non-`Write-Host` output is a single object (e.g. `Get-ItemProperty`'s result), that object's own properties are merged directly into the registered result - see [Native `pwsh` results](#native-pwsh-results).

```yaml
tasks:
  - name: example
    shell:
      command: |
        Write-Host "hello"
        Write-Host "world"
      state: present # absent removes the `creates` entries instead of re-running
      args: [a, b=1]
      creates: [~/.foo/bar]

  # Non-default host: run an existing TypeScript file through npx tsx.
  - name: ts-example
    shell:
      host: npx tsx
      script: files/my-script.ts   # an existing file, resolved like copy.src

  - name: ts-inline-example
    shell:
      host: npx tsx
      extension: .ts               # so the temp file is real TypeScript, not .txt
      command: |
        console.log("hello from tsx")
```

### `blockinfile`

Modeled on Ansible's [`blockinfile`](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/blockinfile_module.html): manages a marker-delimited block of text inside a file, leaving everything else in the file untouched.

| Field | Required | Description |
| --- | --- | --- |
| `dest` | yes | File to manage (`~` expansion supported) |
| `block` | one of `block`/`template`, unless `state: absent` | Content to place between the markers. Use a YAML block scalar (`\|`) for multiline content |
| `template` | one of `block`/`template` | Renders a template and uses its output as the block's content instead of a literal `block` string - same `src`/`engine`/`vars` fields as the [`template`](#template) module (minus `dest`/`state`, which this task's own `dest`/`state` already cover). Wins if both `block` and `template` are given |
| `marker` | no | Template for both marker lines; `{mark}` is replaced with `marker_begin`/`marker_end`, `{name}` with the identifier from `marker_name` (see below). Default `# {mark} IRONSTATE MANAGED - {name}` |
| `marker_name` | no | Identifier substituted for `{name}` in `marker`, so multiple `blockinfile` tasks can target the same `dest` without one overwriting another's block. Defaults to this task's own `name`, then to `dest`'s file name, if not set |
| `marker_begin` | no | Substituted for `{mark}` in the opening marker line. Default `BEGIN` |
| `marker_end` | no | Substituted for `{mark}` in the closing marker line. Default `END` |
| `insertafter` | no | Where to insert the block when the markers aren't already present: `EOF` (default), `BOF`, or a regex - inserted after the last matching line (falls back to `EOF` if nothing matches). Ignored if `insertbefore` is set |
| `insertbefore` | no | Like `insertafter`, but inserts before the first matching line (`BOF` for beginning of file). Takes precedence over `insertafter` when both are set |
| `create` | no | Create `dest` (and its parent directory) if missing. Default `false` - if `dest` doesn't exist and this is `false`, the item is skipped with a warning |
| `backup` | no | Write a timestamped copy of `dest` (`dest.<yyyyMMddHHmmss>.bak`) before changing it. Default `false` |

If the marker lines are already present, the block between them is replaced in place. `present`/`latest` write the block whenever its current content doesn't already match; `absent` removes the marker lines and everything between them.

> **Note:** the default `marker` template gained a `{name}` token (see `marker_name` above). A block written before this change (no name in its marker) won't match the new default and will be treated as not-installed - the next `present`/`latest` run inserts a second, newly-marked block below the old one rather than replacing it in place. Either delete the old marker block by hand once, or pin `marker` to the old `# {mark} IRONSTATE MANAGED` template for that task to keep matching it.

```yaml
tasks:
  # Two blockinfile tasks targeting the same dest - each gets its own
  # marker pair (from its own 'name') so neither overwrites the other.
  - name: dev/ws aliases
    blockinfile:
      dest: ~/.config/powershell/profile.ps1
      create: true
      block: |
        function dev { Set-Location ~/Development }

  - name: shell environment
    blockinfile:
      dest: ~/.config/powershell/profile.ps1
      marker_name: "Override 123" # overrides the identifier instead of using the task's own name
      block: |
        $env:VIRTUAL_ENV_DISABLE_PROMPT = 1

  - name: gitconfig core section
    blockinfile:
      dest: ~/.gitconfig
      marker_begin: "managed-start"
      marker_end: "managed-end"
      insertafter: BOF

  - name: dev/ws aliases, rendered from a template instead of a literal block
    blockinfile:
      dest: ~/.config/powershell/profile.ps1
      create: true
      template:
        src: templates/dev-aliases.ps1.j2
        engine: jinja
        vars:
          dev_path: ~/Development
      block: |
        [core]
          autocrlf = false
```

### `log`

Reuses the present/absent state machine instead of being a special "always run" module kind: `state: present` (default) or `latest` always print the `install` message; `state: absent` always prints the `uninstall` message. Log has no real idempotent "already applied" concept - it always fires when reached; `state` only selects which phase's message prints.

| Field | Description |
| --- | --- |
| `message` | Shorthand for `install.message` - the common "just print something" case |
| `level` | `debug`, `verbose`, `info` (default), `warning`, or `error` |
| `install.message` / `install.level` | Nested form: message/level printed when this resolves to `Install` |
| `uninstall.message` / `uninstall.level` | Nested form: message/level printed when this resolves to `Uninstall` (`state: absent`) |

```yaml
tasks:
  - name: simple
    log:
      message: "Hello, world!"
      level: info

  - name: phase-aware
    log:
      install:
        message: Installing the package
      uninstall:
        message: The package is being uninstalled.
        level: warning
    state: absent   # -> prints the 'uninstall' message
```

### `path`

Adds/removes directories on the current user's persistent `PATH` (User scope - no admin required). Also patches the *current* process's `PATH` for entries it actually adds/removes, so later tasks in the same run (e.g. a binary an `eget` task just installed) are immediately on `PATH` without a new shell.

| Field | Required | Description |
| --- | --- | --- |
| `paths` | yes | Directories to ensure on PATH (`~` expansion supported) |

```yaml
tasks:
  - name: register local bins
    path:
      paths:
        - ~/.local/bin
        - ~/.cargo/bin
      state: present
```

### `fact`

Sets an arbitrary named value - the write-your-own-data counterpart to gathered facts, registered under the same `facts.<name>` namespace (see [Facts](#facts)/[Vars](#vars)), distinct from `id`-registered results (see [Registering results](#registering-results-id)). Reuses the present/absent state machine like `log`: `state: present` (default) or `latest` (re)sets the fact every time it's reached; `state: absent` unsets it. No real idempotency - a fact always fires when reached, and always actually applies (dry-run included, since it's pure bookkeeping with no real system side effect).

| Field | Required | Description |
| --- | --- | --- |
| `name` | yes | Name this fact is registered under, as `facts.<name>` (not to be confused with the `registry` module below - this is the in-memory fact namespace, not the Windows registry) |
| `shell` | no | Runs this command (same shape as [`shell`](#shell)) to compute the fact's value, instead of a literal `value`. See below |
| `value` | yes, unless `state: absent` or `shell` is given | Any YAML value - scalar, list, or nested mapping. May reference `${{ <id> }}`/`${{ facts.* }}`/`${{ vars.* }}`, resolved before being stored |

```yaml
tasks:
  - name: define a simple fact
    fact:
      name: bar
      value: foo

  - name: define a complex fact
    fact:
      name: bar
      value:
        items: [a, b]
        another-property:
          sub-item: foo

  - name: check for a file
    id: bar
    shell:
      command: Get-Item -Path "C:\foo\file.txt"

  - name: copy that result into a fact
    fact:
      name: bar
      value: "${{ bar }}"   # the whole { changed, rc, stdout, ... } object from the 'id: bar' task above
```

#### `fact` with an embedded `shell`

A fact can compute its own value by running a command directly, instead of referencing a separately-`id`'d `shell` task's result. This command **always actually runs, even without `-Apply`** - the same dry-run exception the fact registration itself already gets, since a fact has no real system side effect and a dry-run preview of a later `when`/`${{ }}` reference needs a real value to check, not a zero-result stand-in:

```yaml
tasks:
  - name: PowerShell profile directory fact
    fact:
      name: pwsh_profile_dir
      shell:
        command: Write-Output $PROFILE
      value: "${{ stdout | dirname }}"
```

If `value` is omitted, the fact is set directly to the command's trimmed stdout. If `value` is given, it's resolved *after* the command runs, against this same task's own bare `rc`/`stdout`/`stdout_lines`/`stderr`/`stderr_lines` (the same self-reference convention `failed_when` uses - see [Failing a task](#failing-a-task-failed_when-continue_on_error)) alongside the usual `facts`/`vars`/`id`-registry context, so it can also fall back to a var or combine with something else already registered. `failed_when` on the same task sees the same fields:

```yaml
  - name: PowerShell profile directory fact
    fact:
      name: pwsh_profile_dir
      shell:
        command: Write-Output $PROFILE
      value: "${{ stdout | dirname }}"
    failed_when: rc != 0 or (stdout | length == 0)
```

### `assert`

Fails this task unless every `that` condition holds. `that` uses the same bare-expression grammar as [`when` conditions](#when-conditions) - dotted/indexed identifiers, `== != < <= > >=`, `and`/`or`/`not`, `in`/`not in`, `is`/`is not`, and `|` filters - evaluated against facts/vars/id-registered results. There's no real idempotent "already applied" state (like `log`/`fact`): the check always fires when reached, and - unlike `log`/`fact` - it's always actually evaluated even without `-Apply`, since the check *is* the point and has no system side effect to skip in a dry run.

A failing `that` becomes this leaf's `rc: 1` (message in `stderr`); a passing one becomes `rc: 0` (message in `stdout`). From there it's an ordinary leaf: default `failed_when`/`continue_on_error` behavior applies exactly as it would to any other module (see [Failing a task](#failing-a-task-failed_when-continue_on_error)) - a failed assert stops the run unless `continue_on_error: true`.

| Field | Required | Description |
| --- | --- | --- |
| `that` | yes | Condition(s) that must all be true. A single expression string, or a list of expressions (implicit AND) - matches `when` |
| `fail_msg` | no | Error message used verbatim when one or more `that` conditions are false. Without it, a message is built from the task's `name` and every failing condition |
| `success_msg` | no | Message used verbatim when every `that` condition is true. Without it, a message is built from the task's `name` and the number of conditions checked |

```yaml
tasks:
  - name: Verify local bin path fact
    assert:
      that:
        - facts.local_bin_path is defined
        - facts.local_bin_path | length > 0
        - facts.local_bin_path | exists
      fail_msg: "Local bin path fact is not defined or empty. Please define 'facts.local_bin_path' in your inventory or host vars."
      success_msg: "Local bin path fact is defined and valid."
```

### `registry`

Writes one or more named values under a single registry key.

| Field | Required | Description |
| --- | --- | --- |
| `path` | yes | Registry key path. Supports hive shortcuts `HKLM`/`HKCU`/`HKCR`/`HKU`/`HKCC` and their `HKEY_*` full names (e.g. `HKEY_LOCAL_MACHINE\Software\...`), with or without a trailing `:` or forward slashes. `HKCR`/`HKU`/`HKCC` aren't mounted as PSDrives by PowerShell by default - this mounts them (once) on first use |
| `values` | yes | One or more `{ name, type, value }` entries to write under `path` |

Each entry in `values`:

| Field | Required | Description |
| --- | --- | --- |
| `name` | yes | Value name under the key |
| `type` | yes | One of `String`, `ExpandString`, `Binary`, `DWord`, `MultiString`, `QWord` (matched case-insensitively - `DWORD`/`QWORD` work too) |
| `value` | yes, unless `state: absent` | Matches `type`: a scalar for `String`/`ExpandString`/`DWord`/`QWord`, a list for `MultiString` (strings) or `Binary` (byte values 0-255) |

`present`/`latest` create the key if needed and set every listed value, correcting any that exist with the wrong type or data (not just skipping a mismatch). `absent` removes only the listed value names, never the key itself - a value that exists with the wrong type still gets removed.

```yaml
tasks:
  - name: configure MyApp
    registry:
      path: HKCU:\Software\MyApp
      state: present
      values:
        - name: InstallPath
          type: String
          value: C:\Program Files\MyApp
        - name: Version
          type: DWORD
          value: 3
        - name: AllowedUsers
          type: MultiString
          value: [alice, bob]
```

### `scheduled_task`

Registers/updates/removes a Windows Task Scheduler task, built entirely on the `ScheduledTasks` module (`Get`/`New`/`Register`/`Set`/`Unregister`/`Enable`/`Disable-ScheduledTask`) - no `schtasks.exe` fallback is used, since no functional gap turned up needing one.

| Field | Required | Description |
| --- | --- | --- |
| `name` | yes | Task name |
| `path` | no | Task folder, e.g. `\MyApps\`. Default `\` (root). Normalized to always start/end with `\` |
| `description` | no | |
| `enabled` | no | Default `true`. Applied via `Enable-ScheduledTask`/`Disable-ScheduledTask` after every (re)registration |
| `actions` | yes, unless `state: absent` | One or more `{ execute, arguments, working_directory }` - programs to run, in order |
| `triggers` | no | Omit for a manual/on-demand-only task. See below |
| `principal` | no | Which account the task runs as - omit to run as the current user with standard rights |
| `settings` | no | Task-level settings - partial-declare like `registry`'s `values` (only listed keys are managed/compared) |

Each entry in `actions`:

| Field | Required | Description |
| --- | --- | --- |
| `execute` | yes | Path to the executable/script |
| `arguments` | no | Arguments passed to `execute` |
| `working_directory` | no | |

Each entry in `triggers` has a `type`, which selects which other fields apply:

| `type` | Fields |
| --- | --- |
| `logon` | `user_id?` (omit for any user), `delay?`, `random_delay?` |
| `startup` | `delay?`, `random_delay?` |
| `once` | `at` (full date+time), `repetition_interval?`, `repetition_duration?`, `random_delay?` |
| `daily` | `at` (time-of-day), `days_interval?` (default 1), `repetition_interval?`, `repetition_duration?`, `random_delay?` |
| `weekly` | `at` (time-of-day), `days_of_week` (required list, e.g. `[Monday, Wednesday]`), `weeks_interval?` (default 1), `repetition_interval?`, `repetition_duration?`, `random_delay?` |

`delay`/`random_delay`/`repetition_interval`/`repetition_duration` accept either an ISO 8601 duration (`PT30S`, `P1D`) or a plain .NET TimeSpan string (`00:00:30`, `1.00:00:00`).

**Registering a task with a `logon` or `startup` trigger requires an elevated (Run as Administrator) session** - verified against both `Register-ScheduledTask` and raw `schtasks.exe`, so it's a genuine Task Scheduler privilege requirement, not a gap either tool can route around. `once`/`daily`/`weekly` triggers, and a task with no triggers at all, register fine as a standard user.

`principal`:

| Field | Description |
| --- | --- |
| `user_id` | Account to run as (a username, `SYSTEM`, `NT AUTHORITY\SYSTEM`, ...). Mutually exclusive with `group_id` |
| `group_id` | Group to run as instead of a single user |
| `logon_type` | `None`, `Password`, `S4U`, `Interactive`, `Group`, `ServiceAccount`, or `InteractiveOrPassword` |
| `run_level` | `Limited` (default) or `Highest` (\"Run with highest privileges\") |
| `password_env` | Name of an environment variable (populated via this repo's own `.env`/`.secrets` loading - see [File hierarchy](#file-hierarchy)) holding the account's password. Only used when `logon_type` is `Password` - never write a plaintext password into YAML |

`logon_type: Password` additionally requires `user_id` and `password_env`. A stored task password can't be read back, so idempotency can't detect a drifted password - only `state: latest` is guaranteed to re-apply one.

`settings` (only declared keys are managed/compared):

| Field | Description |
| --- | --- |
| `disallow_start_if_on_batteries` | |
| `start_when_available` | Run as soon as possible after a scheduled start is missed |
| `hidden` | |
| `wake_to_run` | Wake the machine from sleep to run this task |
| `allow_hard_terminate` | |
| `run_only_if_network_available` | |
| `run_only_if_idle` | |
| `multiple_instances` | `IgnoreNew`, `Parallel`, `Queue`, or `StopExisting` |
| `restart_count` | Retries on failure - Task Scheduler silently ignores this unless `restart_interval` is also set; declare both together |
| `execution_time_limit` | Kill the task if it runs longer than this. `PT0S` means no limit |
| `restart_interval` | Delay between retries when `restart_count` is set |
| `delete_expired_task_after` | Auto-delete the task this long after its last trigger expires |

Test compares the task like `registry` compares values: existence plus every declared field (actions, triggers, declared `principal`/`settings` fields, description, `enabled`) must match for `present`/`latest` to resolve to "already applied" - any drift (including an extra/foreign action or trigger this handler didn't put there) triggers a full re-registration via `Register-ScheduledTask -Force`, replacing the task's action/trigger set wholesale rather than patching around the mismatch.

```yaml
tasks:
  - name: kanata autostart
    scheduled_task:
      name: kanata
      description: Starts kanata at logon
      actions:
        - execute: ~/.local/bin/kanata-run.bat
      triggers:
        - type: logon
      principal:
        run_level: Highest   # kanata needs elevation for keyboard remapping
      state: present         # registering this needs an elevated session - see above
```

## Tags filtering

`-Tags` is a flat set of tags; a leaf runs if `-Tags` is omitted, or if any of its effective tags (its own plus every ancestor task's) match:

```powershell
# Anything tagged "cli", anywhere in the tree
.\ironstate.ps1 -Tags cli

# Anything tagged "cli" OR "security"
.\ironstate.ps1 -Tags cli,security
```

There's no more `<group>.<tag>` scheme - `-Tags` no longer knows about module names at all, only tags.

A leaf with **no effective tags at all** (neither its own nor any ancestor task's) always runs, regardless of `-Tags` - it can't be deliberately targeted or excluded by a tag it doesn't have, so `-Tags` simply doesn't apply to it; `when` remains the only gate. This matters for untagged prerequisite tasks a `when` elsewhere depends on - e.g. a `fact` that gathers `facts.wsl_installed` with no `tags:` of its own must still run under `-Tags editors` for a later, differently-tagged task's `when: facts.wsl_installed` to see it set at all. Tag an always-relevant task explicitly if you want that documented in the YAML, but it isn't required for it to keep running.

## Includes

A related set of items (an eget binary, a symlink to it, a config file to copy, a setup script...) can be defined once as a package under `packages/<name>/main.yml`, using the same `tasks:` shape as `site.yml` (the explicit form, so it has a `tasks:` key to merge on). Pull it into a run with an `include:` action, wherever you'd write any other action:

```yaml
# site.yml
tasks:
  - name: install lolcat
    tags: [lolcat, cli]
    include:
      name: lolcat
      state: present    # optional, default 'present' - see Template expressions below
      tags: [keyboard]  # optional - see Template expressions below (distinct from the envelope 'tags:' above, which cascades to every included task regardless)
      with:              # optional - arbitrary inputs, see Template expressions below
        layout: colemak
```

`include` is handled like `actions:` during tree flattening (see [Task/action model](#taskaction-model)): it loads `packages/lolcat/main.yml` and splices its `tasks:` list in at this position, so the included tasks inherit this task's `tags`/`when` exactly like a nested `actions:` list would. Any `copy.src` or `shell.script` path inside a package's `main.yml` is resolved relative to that package's own directory (e.g. `files/kanata.kbd` → `packages/kanata/files/kanata.kbd`), not the `install/windows` root.

```yaml
# packages/kanata/main.yml
tasks:
  - tags: [keyboard, kanata]
    eget:
      package: jtroo/kanata
      state: present
      args: [--to=~/.local/bin/kanata, --upgrade-only, --asset=".zip"]

  - tags: [keyboard, kanata]
    copy:
      src: files/kanata.kbd          # -> packages/kanata/files/kanata.kbd
      dest: ~/.kanata/kanata.kbd
      state: present

  - tags: [keyboard, kanata]
    symlinks:
      src: ~/.local/bin/kanata/kanata_windows_gui_winIOv2_cmd_allowed_x64
      dest: ~/.local/bin/kanata.exe
      state: present
```

### Template expressions

`state`, `tags`, and `with` on an `include:` action are **not** applied to the included tasks automatically (that's what the envelope `tags:`/`when:` next to `include:` are for - see the `install lolcat` example above). They're only made available inside that package's `main.yml` as GitHub-Actions-style `${{ ... }}` expressions, which a package opts into by writing the expression on whichever of its own fields should receive it:

| Expression | Resolves to |
| --- | --- |
| `${{ package.name }}` | The package's directory name |
| `${{ package.state }}` | The `state` on the `include:` action (default `present` if omitted) |
| `${{ package.tags }}` | The `tags` array on the `include:` action (default `[]`) |
| `${{ inputs.<key> }}` | `with.<key>` on the `include:` action (dotted paths like `${{ inputs.<key>.<nested> }}` work for nested values) |
| `${{ facts.<key> }}` | A gathered host fact, or an earlier task's `fact` (see [Facts](#facts) and [`fact`](#fact)) |
| `${{ vars.<key> }}` | A user-defined var (see [Vars](#vars)) |
| `${{ item }}` / `${{ item.<key> }}` | The current loop value, inside a task with `with`/`items` (see [Looping](#looping-withitems)) |
| `${{ parent.item }}` / `${{ parent.item.<key> }}` | The *enclosing* loop's value, inside a loop nested within another loop's `actions:` (chain `parent.parent.item` etc. for deeper nesting - see [Nested loops](#nested-loops-parent)) |
| `${{ <id> }}` / `${{ <id>.<field> }}` | An earlier task's `id`-registered result (see [Registering results](#registering-results-id)) |

Any of these paths can index into a list with `[N]` (0-indexed), interspersed with dotted access - e.g. `${{ example_task.results[0].rc }}` for a looped task's first iteration. `when` supports the same `[N]` indexing in its bare (non-`${{ }}`) grammar.

The text inside `${{ ... }}` is a full expression - not just a bare path - built on the same grammar as `when` (see [`when` conditions](#when-conditions)), including its `value | filter(args)` pipeline and `is`/`is not` type tests. The most common use is a value-or-default fallback:

```yaml
# packages/languages/java/main.yml
vars:
  defaults:
    languages:
      java:
        jdk: Microsoft.OpenJDK.25
        jre: false

tasks:
  - name: Install Java JDK
    winget:
      package: "${{ languages.java.jdk | toggle(defaults.languages.java.jdk) }}"
      state: present
    when: languages.java.jdk != false
```

A `vars:` block at the top of a package's own `main.yml` declares **package-local defaults**: bare top-level names (here `defaults`, but a package author can name it anything - it's not a reserved word) distinct from site-level `vars.*`, so a package can express "the user's override, else my own built-in default" as two separate paths rather than one merged value. Package-local vars are available inside that package's own expressions only (not to its caller, nor to a package it in turn includes); if a package-local name ever collides with a site-level var/fact/id-registered name, the site-level one wins.

Here, a user's `site.yml` sets `languages.java.jdk` to a plain **boolean** (`true`/`false`, matching every other `languages.*` toggle) to enable/disable Java without specifying a package - `toggle(...)` (unlike `default`, which only replaces `null`) treats *any* boolean the same as unset, and falls back to the package's own built-in default; a **string** value instead names a specific override package ID (e.g. `jdk: Eclipse.Temurin.21`). The separate `when: languages.java.jdk != false` is what actually skips the task when the user explicitly disabled it - `toggle(...)`'s job is only to pick the right package ID once the task is known to be enabled. A filter's argument can just as well be a literal (`default('Oracle.JDK.25')`) or a site-level path (`default(vars.editor)`) instead - whichever suits the package. Filters also chain left-to-right (`${{ inputs.name | trim | upper }}`).

If an expression is the **entire** value of a field (e.g. `state: ${{ package.state }}` or `tags: ${{ package.tags }}`), that field is replaced with the referenced value's native type — a string stays a string, an array stays an array. If that whole-value expression can't be resolved, the field is **omitted** instead (a warning is logged either way) - the consuming code's own default applies, rather than injecting a wrongly-typed empty string (see [Looping](#looping-withitems) for the common case: an optional per-item field like `args: ${{ item.args }}`). If the expression is embedded inside a larger string instead (e.g. a `shell.command` or a `copy.dest` path), it's substituted as text, and an unresolved one just blanks that portion of the string (there's no "omit part of a string" equivalent).

```yaml
# site.yml
tasks:
  - name: kanata
    include:
      name: kanata
      state: absent      # toggle the whole package off
      with:
        layout: colemak

# packages/kanata/main.yml
tasks:
  - eget:
      package: jtroo/kanata
      state: present     # this dependency stays installed even if the package is 'absent'
      args: [--to=~/.local/bin/kanata, --upgrade-only, --asset=".zip"]

  - copy:
      src: files/${{ inputs.layout }}.kbd
      dest: ~/.kanata/kanata.kbd
      state: ${{ package.state }}   # follows the package's toggle
```

Note that `${{ package.tags }}` replaces a field wholesale rather than appending — to combine caller-supplied and package-defined tags, reference individual `with` values instead (e.g. `tags: [keyboard, ${{ inputs.extra_tag }}]`).
