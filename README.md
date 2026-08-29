# ironstate

`ironstate` is a declarative, Ansible-style task runner driven by YAML: it reconciles each leaf action's desired `state` against what's currently installed, printing what it would do by default (dry-run) and only making changes when `--apply` is passed. It ships as a single cross-platform binary (Windows/Linux/macOS - see `.goreleaser.yaml`); `cmd/ironstate` is the entrypoint, with the engine/handlers/filters/template code living under `internal/` (see [Architecture](#architecture)).

## Installation

### Install script (recommended)

Downloads the release archive matching your OS/architecture from [GitHub Releases](https://github.com/TacoContent/ironstate/releases), verifies its checksum, and installs the `ironstate` binary to `~/.local/bin` by default. Every OS/architecture combination this project ships is built (Windows/Linux/macOS, each on `amd64`/`x86_64` and `arm64`) - macOS covers both Intel (`amd64`) and Apple Silicon (`arm64`).

**Linux/macOS:**

```shell
curl -fsSL https://raw.githubusercontent.com/TacoContent/ironstate/develop/install/install.sh | bash
```

**Windows (PowerShell):**

```powershell
irm https://raw.githubusercontent.com/TacoContent/ironstate/develop/install/install.ps1 | iex
```

Both scripts accept an install directory and a specific version to pin instead of latest:

```shell
./install.sh --dir /usr/local/bin --version v0.1.0
```

```powershell
./install.ps1 -InstallDir C:\tools\bin -Version v0.1.0
```

If no release exists for your OS/architecture, the script exits with an error, a link to open an issue on this repo, and a block of diagnostic text (OS, architecture, script name, the release tag/asset it tried) to paste directly into the issue - see `install/install.sh`/`install/install.ps1`.

### Manual download

1. Download the archive for your OS/architecture from the [latest release](https://github.com/TacoContent/ironstate/releases/latest) - `ironstate_<version>_<os>_<arch>.zip` (Windows) or `ironstate_<version>_<os>_<arch>.tar.gz` (Linux/macOS). `<arch>` is Go's naming: `amd64` (x86_64/Intel) or `arm64` (Apple Silicon/ARM64) - e.g. an Intel Mac (`uname -m` prints `x86_64`) wants `ironstate_<version>_darwin_amd64.tar.gz`.
2. (optional) Verify it against that release's `checksums.txt` (e.g. `sha256sum -c checksums.txt` or `Get-FileHash -Algorithm SHA256`). `checksums.txt` is itself keyless-signed with [cosign](https://github.com/sigstore/cosign): `cosign verify-blob --bundle checksums.txt.sigstore.json checksums.txt --certificate-identity-regexp "https://github.com/TacoContent/ironstate/.github/workflows/release.yml@.*" --certificate-oidc-issuer https://token.actions.githubusercontent.com`.
3. Extract the archive and put the `ironstate`/`ironstate.exe` binary somewhere on your `PATH`.

### `go install`

`ironstate` is a public Go module, so the Go toolchain (1.26+, see `go.mod`) can build and install it directly from source, no release archive needed:

```shell
go install github.com/TacoContent/ironstate/cmd/ironstate@latest
```

This installs to `$(go env GOPATH)/bin` (make sure it's on `PATH`). Since this skips the release build's `-ldflags`, `ironstate version` reports `dev`/`none`/`unknown` instead of a real version/commit/date - functionally identical otherwise.

### Using [zyedidia/eget](https://github.com/zyedidia/eget)

`eget` is a small, cross-platform Go binary that downloads and installs a release from GitHub Releases. It can be used to install `ironstate` without a shell script:

#### linux/macOS

```shell
eget TacoContent/ironstate --to=~/.local/bin
```

#### Windows (PowerShell)

```powershell
eget TacoContent/ironstate --to=$env:USERPROFILE\.local\bin
```

### Build from source

```shell
git clone https://github.com/TacoContent/ironstate.git
cd ironstate
go build -o bin/ironstate ./cmd/ironstate     # bin/ironstate.exe on Windows
```

## Getting started

```shell
# Scaffold a new playbook (see Playbooks below)
ironstate init my-playbook

# Dry-run / apply / tags - --playbook takes a specific file, a playbook
# directory (searches for site.yml/main.yml inside it), or a bare name
# with a sibling '<name>.yml'; omit it to use the current directory.
ironstate --playbook path/to/your/playbook
ironstate --playbook path/to/your/main.yml --apply
ironstate --playbook path/to/your/main.yml --tags cli,security --apply

# Verbose (also prints a line for every already-satisfied/skipped leaf)
ironstate --playbook path/to/your/main.yml --verbose

# Merge in an extra vars document, and/or override individual vars by
# dotted key path (repeatable, highest precedence of all)
ironstate --playbook path/to/your/main.yml --vars-file ./ci-vars.yml
ironstate --playbook path/to/your/main.yml --var editor=nvim --var ssh.port=2222

# Introspection
ironstate filters list
ironstate doctor
ironstate version
```

**Exit codes**: `0` on a clean run; `1` when the run stopped on a task's `failed_when`/an unhandled dispatch error; `2` when the site file failed to load or parse (see `internal/cli/errors.go`).

**Cross-platform**: `ironstate` itself builds and runs on Windows/Linux/macOS. Windows-only modules (`winget`, `chocolatey`, `registry`, `scheduled_task`, `path`) error clearly when dispatched on a non-Windows host; a `shell.host: pwsh` task still needs `pwsh` on `PATH` wherever it's actually dispatched, same as any other `shell` host preset needing its own interpreter present. See [Facts](#facts) for the `platform`/`arch`/`os_family` facts a `main.yml` can branch on.

**Output**: on a real terminal, results print as a colored table with a per-module emoji, a host-facts panel up front, and a final summary block with elapsed time - a "changed" leaf (installed/removed) reads brighter than an already-satisfied skip, and a failure reads in a danger red. `--output json` switches to a plain JSON array on stdout instead (informational/progress lines go to stderr, so this stream stays clean and pipeable, e.g. `... --output json | jq`); colors auto-disable when not attached to a terminal, honor `NO_COLOR`/`IRONSTATE_NO_COLOR`, or can be forced off with `--no-color`.

## Playbooks

`ironstate` doesn't hard-code a single site file location - like an Ansible playbook, you create your own directory with a `main.yml` plus whatever `hosts/`, `variables/`, `packages/`/`roles/` overlays it needs, then point `--playbook` at that directory (or its `main.yml` directly). [`playbooks/camalot/`](playbooks/camalot) in this repo is one such playbook, kept here as a real-world worked example (the repo owner's own machine setup) - copy its shape for your own playbook, or start from an empty `main.yml`. Nothing about the `playbooks/` directory name, or `camalot` itself, is special or required by `ironstate` - it's just a sample.

`--playbook` doesn't require the exact file path: point it at a directory and it searches for `site.yml`, `site.yaml`, `main.yml`, then `main.yaml` inside it; point it at a bare name (e.g. `playbooks/camalot`) and it also tries a sibling `playbooks/camalot.yml`/`.yaml` file; omit `--playbook` entirely to search the current directory. An error names every path tried if none exist.

### `ironstate init`

`ironstate init [playbook-name]` scaffolds that starter shape for you - creates `<playbook-name>/` (or initializes the current directory if no name is given) with:

```
<playbook-name>/
├── main.yml
├── roles/
├── tasks/
├── packages/
├── hosts/<machine-name>.yml
└── variables/<user-name>.yml
```

Every generated YAML file is intentionally minimal (`vars: {}` / `tasks: []`) - just enough to be a valid document to start editing; `roles/`, `tasks/`, `packages/` are created empty (with a `.gitkeep` placeholder so they survive a fresh `git clone`). `<machine-name>`/`<user-name>` come from this machine's own `computer_name`/`user_name` facts (see [Facts](#facts)), lowercased. Re-running `init` is safe - an already-existing file or directory is left untouched, never overwritten.

```shell
ironstate init my-playbook
cd my-playbook
ironstate --playbook main.yml
```

## File hierarchy

Files are loaded and merged in this order. Each subsequent file's `tasks` list is **appended** to the base file's list; `vars` **deep-merges** key-by-key (an overlay can add or override individual vars without wiping out the base set).

``` tree
<playbook>/
├── main.yml                              ← base (always loaded)
├── hosts/
│   ├── main.yml                          ← default, merged if it exists
│   └── <chained-name>.yml                ← chained overlays, merged least-to-most specific
└── variables/
    ├── main.yml                          ← default, merged if it exists
    ├── <chained-name>.yml                ← chained overlays, merged least-to-most specific
    └── <USERNAME>.yml                    ← legacy per-user overlay, merged last
```

The **chained overlay** filenames are built from this machine's own facts (see [Facts](#facts)): `hostname`, `os_family`, `platform`, `arch`. Use **any N of them**, joined by `.`, **in any order you like** - so a bare single-characteristic name like `windows.yml`/`ubuntu.yml`/`archlinux.yml` (matching `os_family`) or `amd64.yml` (matching `arch`) works, and so does `amd64.krayt.yml` written arch-before-hostname instead of `krayt.amd64.yml`:

```
windows.yml                     # any host where os_family == "windows"
ubuntu.yml                      # any Linux host whose distro ID is "ubuntu"
amd64.yml                       # any host where arch == "amd64"
amd64.windows.yml                # arch == "amd64" AND os_family == "windows"
krayt.yml                       # this specific host, any arch/os
krayt.amd64.yml                 # same as amd64.krayt.yml - order doesn't matter
krayt.amd64.windows.yml
krayt.amd64.windows.linux.yml    # the fully-qualified chain (os_family and platform both listed)
```

`ironstate` merges every one of these that exists, broadest first, most specific last. Precedence is a weighted priority - hostname (8) > os_family (4) > platform (2) > arch (1) - so a name's "specificity" is the sum of the characteristics it names: any hostname-containing name always outranks any name without one (8 alone beats 4+2+1=7 combined), and within that, adding more/higher-priority characteristics always ranks higher still. So `hosts/windows.yml`/`hosts/ubuntu.yml` applies to any matching machine, `hosts/krayt.yml` layers a `krayt`-only override on top of it, and `hosts/krayt.amd64.yml` layers on top of that again. This same `main.yml` + chained-overlay mechanism also applies to **every** `include:` (`roles/`, `packages/`, `hosts/<name>/`, etc. - see [Architecture](#architecture)): a package can ship e.g. `roles/foo/windows.yml` or `roles/foo/krayt.yml` right next to its `roles/foo/main.yml` to override/extend it for a class of hosts or one specific host.

`--vars-file <path>` (repeatable) merges one or more additional documents on top of everything above, in the order given (a later `--vars-file` wins over an earlier one on overlapping keys) - highest precedence short of `--var` - handy for a CI-only or one-off vars file that isn't part of the playbook's own hierarchy:

```shell
ironstate --playbook main.yml --vars-file ./ci-vars.yml
ironstate --playbook main.yml --vars-file ./ci-vars.yml --vars-file ./local-overrides.yml
```

`--var key=value` (repeatable) overrides a single var by dotted key path, after everything else has merged - the final, most explicit word on a var's value:

```shell
ironstate --playbook main.yml --var editor=nvim --var ssh.port=2222
```

Overlay files use the explicit `tasks:`/`vars:` mapping form (not the bare-list form) so they have somewhere to merge into.

### `.env` / `.secrets`

Before anything else runs, `ironstate` loads `KEY=VALUE` lines from a `.env` file, then a `.secrets` file, out of the **current working directory** (not the playbook's own directory, and not relative to the binary) into the process environment - so `lookup('env', 'KEY')`, a `scheduled_task`'s `password_env`, or anything else reading `$env:KEY`/`os.Getenv` sees them. Either file is optional; a missing one is silently skipped. Each non-blank, non-`#`-comment line is `KEY=VALUE`; a value wrapped in matching `'...'`/`"..."` has those quotes stripped. Loaded in that order (`.env` then `.secrets`), so run `ironstate` from wherever these files live - typically your playbook's own root, alongside `main.yml`, if you keep it there.

### Secrets and sensitive values

`ironstate` treats sensitive values as secret once they are known, then masks them in any later output it prints while leaving the value usable for actual evaluation.

This is done in two ways:

- `.secrets` is automatically sensitive. Anything loaded from `.secrets` is registered as a secret value immediately, before tasks start running. This makes `.secrets` the authenticated/stored-secret channel, while `.env` remains ordinary config and is not treated as automatically secret.
- A fact/var/task result can be marked as secret by prefixing the authored name with `$`. The `$` is stripped for runtime access, but the value is still tracked as secret and redacted from logs, tables, and JSON output.

```yaml
vars:
  $github_token: "ghp_..."

tasks:
  - fact:
      name: $api_token
      value: ${{ lookup('env', 'MY_TOKEN') }}

  - id: $task_secret
    shell:
      command: Write-Output "secret message"
```

Accessible names remain normal after the prefix is stripped:

- `vars.github_token`
- `facts.api_token`
- `task_secret.stdout`

The secret marker is display-only. Conditions and template expressions still resolve the real value; redaction happens when `ironstate` prints it back out, replacing the sensitive value with `***` instead of emitting the raw string. This applies to per-task logging, task result tables, and `--output json` output alike.

A few safeguards are deliberate:

- very short values are ignored by the registry to avoid over-redacting common values like `true` or `1`
- transformed outputs such as `${{ vars.token | upper }}` are only redacted when the underlying secret value is the exact registered string, matching the usual CI-style secret masking model
- a secret task still preserves the real value internally for evaluation, but its verbose log lines and result labels are suppressed/redacted to avoid leaking the script text, task name, or stdout/stderr content

### Machine-specific overrides — `hosts/`

Create a file named after the machine's hostname to add or change tasks for that machine only (the same `computer_name` fact - see [Facts](#facts) - falls back to the OS hostname on Linux/macOS). You can also chain on `.arch`/`.os_family`/`.platform` for a narrower override (e.g. `KRAYT.amd64.windows.yml`, or `KRAYT.amd64.ubuntu.yml` on Linux), use a bare characteristic name with no hostname at all for a broader one (e.g. `windows.yml` for every Windows machine, or `ubuntu.yml` for every Ubuntu machine), or drop in a `hosts/main.yml` as a default applied to every machine before any of these - see [File hierarchy](#file-hierarchy).

```powershell
# Find your hostname
$env:COMPUTERNAME   # e.g. KRAYT - or `hostname` on Linux/macOS
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

Create a file named after the username to add tasks or vars for a specific user account on any machine. This same directory also accepts the chained hostname/arch/os_family/platform overlays (including bare characteristic names like `windows.yml` or `ubuntu.yml`) and a `variables/main.yml` default described in [File hierarchy](#file-hierarchy) - the username file always merges last (highest precedence of the auto-detected files, before `--vars-file`/`--var`).

```powershell
# Find your username
$env:USERNAME   # e.g. ryan - or `$USER`/`whoami` on Linux/macOS
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

`ironstate` loads YAML, merges overlays, flattens the task/action tree (accumulating `tags` and `when` - see below - and loading any `include`d packages inline as it goes), applies `--tags` filtering, then dispatches the surviving leaves to their handlers *sequentially*, in the order the tasks are written (Ansible-style; not grouped by module). "Sequentially" matters: each leaf's `when` and any remaining `${{ }}` references are resolved *immediately before that leaf runs*, against facts/vars plus a registry that grows as earlier leaves' `id`/`fact` results come in - not all evaluated up front - so a task can act on an earlier task's result (see [Registering results](#registering-results-id)). The actual per-module logic (how to test/install/uninstall/describe a leaf) lives in its own handler under `internal/handlers`.

**Fact gathering runs first**: before any of that, every `fact` leaf (and every other fact-producing module, like [`mount_facts`](#mount_facts) - anything whose handler implements `engine.FactProducer`) in the whole tag-filtered tree runs as its own pass, in document order, ahead of every other leaf - matching Ansible's "gather facts" phase. This means such a leaf's own `value`/`when` can only see gathered facts, vars, and facts registered earlier in this same pass; it can **never** reference another task's `id`-registered result, since no non-fact task has run yet at that point - see [`fact`](#fact) for how to compute a fact's value from a live command instead.

``` tree
cmd/ironstate/            ← main package (thin - flag parsing lives in internal/cli)
internal/
├── cli/                  ← cobra command tree (root/apply, 'filters', 'doctor', 'version')
├── config/                ← flags/env/config-file loading (viper)
├── packages/              ← YAML loading, hierarchy merge, 'include' package loading
├── model/                 ← generic YAML shape + helpers shared across packages
├── tasks/                 ← task/action tree flattening (loops, tags/when cascade, 'include')
├── facts/                 ← gathered host facts (Windows-real / other-OS build split)
├── expr/                  ← shared expression tokenizer/parser/evaluator (variable paths,
│                            comparisons, 'is' tests, '|' filter pipeline) - used by both
│                            conditions/ and template/
├── conditions/            ← 'when'/'failed_when' expression evaluation
├── template/               ← '${{ }}' resolution passes (soft/strict/self-referential)
├── templateengines/        ← 'jinja' (native) and 'gotemplate' (Go stdlib) render engines
├── filters/                ← built-in '|' filters, plus the external script-filter adapter
│   └── embed/               ← the generic PowerShell shim script filters run through
├── engine/                 ← dispatch loop (fact-first two-phase run, registry/facts/
│                            command-availability threading, table/JSON/summary output)
├── handlers/               ← one file per module: winget, chocolatey, homebrew, apt, pacman, yum, apk,
│                            snap, flatpak, scoop, macports, pipx, npm, cargo, go,
│                            gem, eget, git, iptables, ufw, advfirewall, firewall, zip, symlinks, file,
│                            copy, shell, blockinfile, lineinfile,
│                            ssh_host_block, log, fail, path, fact, mount_facts, assert, async, wait_for,
│                            registry, scheduled_task, template
├── ui/                     ← terminal color/emoji output styling
└── exec/                   ← external-process Runner abstraction handlers shell out through
```

Each `internal/handlers/*.go` file implements the shared `Handler` interface (`Test`/`Describe`/`Install`/`Uninstall` - see `internal/engine/engine.go`). To add a new module: register a handler in `internal/handlers/handlers.go`'s `All()`; add its name to `handlers.AllModuleNames` (task-tree flattening won't recognize the module key in a YAML task at all otherwise - see `internal/tasks.Options.ModuleNames`/`firstModuleKey`); add it to `engine.DefaultNoCommandCheckModules` if it isn't backed by an external CLI; and, for parity, add it to `internal/tasks/realfixture_test.go`'s `realModuleNames`. Optionally add a matching `$defs` entry to `ironstate.schema.json` for editor validation/autocomplete on the new module's fields.

### Custom filters

Two ways to add a `|` filter (see [`when` conditions](#when-conditions) for the pipeline syntax):

- **Built-in** (compiled in): add a function to `internal/filters/builtins.go` and register it there - requires rebuilding the binary, but has no external process to spawn per call.
- **Script filter** (no rebuild): drop a script implementing the shim's contract into the directory `ironstate` scans for filters - `filters/` by default, resolved relative to the site file's own directory, configurable via a `filters.dir` config value (`ironstate doctor --filters-dir <dir>` / `ironstate filters list --dir <dir>` to inspect what's discovered). It's registered automatically at startup under its file's base name (`upper.ps1` → the `upper` filter, `leet.sh` → the `leet` filter) - **only if no built-in already claims that name** (a built-in always wins). If a directory has more than one script implementing the *same* filter name (e.g. both `leet.ps1` and `leet.sh`), only one wins, by this fixed priority order: **PowerShell → bash → zsh → fish → nushell**. Five extensions ship a runner today, each via a generic, embedded shim speaking a small JSON-over-stdio protocol:
  - `.ps1` - PowerShell, `param($Value, [object[]] $ArgValues)` (`internal/filters/embed/shim.ps1`), returning any JSON-serializable value.
  - `.sh` / `.zsh` / `.fish` - bash/zsh/fish, plain positional args (`$1`/`$argv[1]` is the value, the rest are the call's args, always coerced to strings) and stdout as the string result (`internal/filters/embed/shim.sh`/`shim.zsh`/`shim.fish`) - each requires `jq` on `PATH` for the shim itself to decode/encode requests (these three shells have no native JSON support). On Windows, a bare `bash` on `PATH` very commonly resolves to Windows' own WSL launcher stub (`System32\bash.exe`), which is incompatible (it mangles Windows-style paths) - ironstate skips that launcher and prefers a real Windows-path-aware bash (Git for Windows, MSYS2, Cygwin) instead.
  - `.nu` - nushell, `def main [value, ...args] { ... }` printing its result (`internal/filters/embed/shim.nu`) - like the other shells, always a plain string result. Unlike every other extension, a `.nu` filter is **not** kept warm as a persistent worker process across calls - nushell has no reliable way to read one request, respond, then read a later request on the same still-open stdin pipe, so each call spawns a fresh `nu` process instead (slower per call, but correct).

  A `filters.interpreters` config value maps other extensions to their own interpreter argv for whenever a new script language gets a shim. Each discovered script's interpreter process is started once, lazily, and kept warm for the run's lifetime rather than spawned per call (except `.nu`, see above).

Run `ironstate filters list` (or `ironstate doctor`) to see every filter currently available, built-in or script, by name.

### `ironstate filters list`

Lists every filter available to `${{ }}`/`when` right now - every built-in from the table above, plus whatever script filters (see [Custom filters](#custom-filters)) are discovered under a directory - one `<name> <built-in|script>` line per filter, sorted by name:

| Flag | Default | Description |
| --- | --- | --- |
| `--dir` | `filters/` | Directory to scan for external script filters |

```shell
$ ironstate filters list --dir path/to/your/playbook/filters
basename             built-in
concat               built-in
default              built-in
...
upper                built-in
my_custom_filter     script
```

A name shown as `built-in` is always resolved from `internal/filters/builtins.go`, even if a same-named script also exists under `--dir` - a built-in always wins (see [Custom filters](#custom-filters)).

### `ironstate doctor`

A single at-a-glance health check: confirms every package-manager CLI a `main.yml` might dispatch to is actually on `PATH`, plus reports discovered script filters (the same listing `filters list` gives, folded into one command since both are "is my environment set up correctly" checks).

| Flag | Default | Description |
| --- | --- | --- |
| `--filters-dir` | `filters` | Directory to scan for external script filters |

```shell
$ ironstate doctor --filters-dir path/to/your/playbook/filters
[ok]      winget   C:\Users\you\AppData\Local\Microsoft\WindowsApps\winget.exe
[missing] choco    not found on PATH
[missing] brew     not found on PATH
[missing] apt-get  not found on PATH
[ok]      pipx     C:\Users\you\.local\bin\pipx.exe
[missing] npm      not found on PATH
[ok]      cargo    C:\Users\you\.cargo\bin\cargo.exe
[ok]      go       C:\Program Files\Go\bin\go.exe
[missing] gem      not found on PATH
[ok]      eget     C:\Users\you\.local\bin\eget.exe
[ok]      pwsh     C:\Program Files\PowerShell\7\pwsh.exe

script filters discovered under path/to/your/playbook/filters:
  my_custom_filter
```

A `[missing]` line isn't necessarily a problem - it only matters for the specific package-manager modules (`winget`/`chocolatey`/`homebrew`/`apt`/`pacman`/`yum`/`apk`/`snap`/`flatpak`/`scoop`/`macports`/`pipx`/`npm`/`cargo`/`go`/`gem`/`eget`) or `shell.host: pwsh` tasks your own `main.yml` actually uses; `doctor` checks a fixed list of every module this build knows about; `bin` availability is otherwise re-checked per-module at dispatch time regardless (see [Architecture](#architecture)).

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

This grammar lives in `internal/expr` and is shared with `${{ }}` template expressions (see [Template expressions](#template-expressions)) - anything documented here, including filters, also works inside `${{ }}`, and vice versa.

- String comparisons (`==`, `!=`, ordering) are **case-sensitive**, matching Ansible/Jinja.
- `in`/`not in`: right-hand side a list → membership check; a string → substring containment.
- A bare variable with no operator is truthy-checked directly (Ansible-style `when: some_var`).
- `is`/`is not` type-tests: `mapping` (alias `map`), `boolean` (alias `bool`), `string`, `number`, `list`, `defined`, `none` (alias `null`). Use these instead of `==`/truthy checks when you need to know a value's actual type rather than whether it casts to `true` - notably, `some_map == true` is **also true for any non-empty mapping**, since both go through the same truthy cast, so `is mapping`/`is boolean` are the only reliable way to tell "a boolean `true`" apart from "a map" (see `packages/languages/main.yml`, where a var can be either a blanket `true` or a per-key map).
- `value | filter(args)` pipes a value through a named filter, Jinja-style, and chains left-to-right (`value | trim | upper`). Filters bind tighter than comparisons, so `java_version | default("25") == "25"` filters first, then compares. Every built-in filter below lives in `internal/filters/builtins.go`; a playbook can also add its own without recompiling via a script filter (see [Custom filters](#custom-filters)). Unless noted otherwise, every filter is **null-in/null-out**: a `null` piped value passes straight through rather than erroring.

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

Gathered fresh every run; a deliberately small, easy-to-extend starter set (see `internal/facts/facts.go`). Anything pricier to gather that most runs don't need - like the host's mounted filesystems - is a task-callable module instead, gathered only when a `main.yml` actually asks for it: see [`mount_facts`](#mount_facts).

| Fact | Description |
| --- | --- |
| `computer_name` | The machine's hostname (`$env:COMPUTERNAME` on Windows, OS hostname elsewhere) |
| `user_name` | The current user's name (`$env:USERNAME` on Windows, the OS user record/`$USER` elsewhere) |
| `home` | The current user's home directory |
| `os_version` | OS version, reported as-is (not reformatted/reparsed): `major.minor.build` on Windows (its real OS version), macOS's product version verbatim (`sw_vers -productVersion`, e.g. `14.5`), or on Linux the distro's `/etc/os-release` `VERSION_ID` verbatim (e.g. Ubuntu `22.04`, Debian `13`), falling back to the kernel release verbatim (`uname -r`, e.g. `6.8.0-31-generic`) for rolling releases with no `VERSION_ID` (e.g. Arch Linux) |
| `os_build` | OS build number (Windows only - always `0` on Linux/macOS, where there's no equivalent single number) |
| `is_admin` | Whether the current process is running elevated |
| `shell_pwsh` | Whether `pwsh` is on `PATH` |
| `pwsh_version` | Output of `pwsh --version` if `pwsh` is on `PATH`, else `null` |
| `shell_bash` | Whether `bash` is on `PATH` |
| `bash_version` | Output of `bash --version` (first line only) if `bash` is on `PATH`, else `null` |
| `shell_zsh` | Whether `zsh` is on `PATH` |
| `zsh_version` | Output of `zsh --version` if `zsh` is on `PATH`, else `null` |
| `shell_fish` | Whether `fish` is on `PATH` |
| `fish_version` | Output of `fish --version` if `fish` is on `PATH`, else `null` |
| `shell_nu` | Whether `nu` is on `PATH` |
| `nu_version` | Output of `nu --version` if `nu` is on `PATH`, else `null` |
| `platform` | Go's `GOOS` - `windows`, `linux`, or `darwin` |
| `arch` | Go's `GOARCH` - `amd64`, `arm64`, ... |
| `os_family` | `windows`/`darwin` as-is; on Linux, the distribution ID from `/etc/os-release` (e.g. `ubuntu`, `debian`, `archlinux`, `alpine`, `redhat`, `fedora`) if detectable, else `linux` |

### Vars

User-defined, under a top-level `vars:` mapping (merges across `main.yml`/`hosts/`/`variables/` - see [File hierarchy](#file-hierarchy)):

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
| `rc` | Exit code. Real for every CLI-backed module (`winget`/`chocolatey`/`homebrew`/`apt`/`pacman`/`yum`/`apk`/`snap`/`flatpak`/`scoop`/`macports`/`pipx`/`npm`/`cargo`/`go`/`eget`) and `shell`; every pure-PowerShell module defaults to `0` unless it throws (then `1`) |
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

**Ordering matters**: this only works because tasks are dispatched sequentially and each one's `when`/`${{ }}` are resolved *immediately before* it runs (not all up front) - see [Architecture](#architecture). Referencing an `id` that hasn't executed yet (written later in the file, skipped by an earlier `when`/`--tags`, or never run because it's an `--apply`-gated real command evaluated during a dry-run) resolves to `$null`/the zero-value defaults, not an error - write comparisons defensively (e.g. `foo_stat != null and foo_stat.rc != 0`) if that distinction matters. In dry-run, `changed` is still accurately predicted for every module (it doesn't require actually running anything), but `rc`/`stdout`/`stderr` stay at their zero-value defaults for every module since nothing actually runs without `--apply`.

`fact` (see below) is the write-your-own-value counterpart to `id`: it always actually applies (dry-run included), since it's pure bookkeeping with no real system side effect - useful when you want a value available for later `when`/`${{ }}` previews without gating it behind an `id`'d real action. Unlike `id`, a fact registers under the `facts.<name>` namespace (see [Facts](#facts)/[Vars](#vars)), not bare.

### Native `pwsh` results

An `id`'d `shell` task under the default `host: pwsh` doesn't just capture text - if the command's only real output (ignoring `Write-Host`/`Write-Information`) is a single object, that object's own properties are merged directly into the registered result, right alongside `rc`/`stdout`/etc. (a real property is never overwritten by one of those reserved names). So a task that runs a plain PowerShell expression returning a rich object can have its fields referenced directly, no extra indirection:

```yaml
tasks:
  - name: get program files directory
    id: pf
    shell:
      command: (Get-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion")

  - name: log it
    log:
      message: ${{ pf.ProgramFilesDir }}
```

This only applies to `host: pwsh` (the only host with a real "object" concept - an external process like `cmd`/`node`/`npx tsx` only ever produces text) and only when exactly one such object comes out; a command producing plain text, zero objects, or more than one object just gets the usual `rc`/`stdout`/`stdout_lines`/`stderr`/`stderr_lines` shape.

**A `fact` can't read this off another task's `id`** (see [`fact`](#fact) - facts run in their own gather-facts pass, before any `id`'d task). To turn a native result like this into a fact, give the fact its own embedded `shell` instead - `value` then self-references this same merged result, the same convention `failed_when` uses:

```yaml
tasks:
  - name: program files directory fact
    fact:
      name: program_files_dir
      shell:
        command: (Get-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion")
      value: ${{ ProgramFilesDir }}   # -> referenced later as ${{ facts.program_files_dir }}
```

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

- **Unset (default)**: a failed task stops the whole run immediately - remaining tasks never dispatch, and `ironstate` exits with a non-zero code. Matches Ansible's default (no `ignore_errors`).
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
| `homebrew` (alias: `brew`) | Homebrew formulae/casks on macOS and Linux (`brew`) |
| `apt` | Debian/Ubuntu packages (`apt-get`) - the one package-manager module here that can install/remove several packages in a single leaf (`package:` takes a list); modeled on `ansible.builtin.apt`'s core surface (`state`, `update_cache`/`cache_valid_time`, `upgrade`, `purge`, `autoremove`, `autoclean`, `install_recommends`, `only_upgrade`, `allow_unauthenticated`, `force`). Needs `become: true` (or a specific user) to actually run, since apt-get requires root - see [`become`](#become) |
| `pacman` | Arch Linux packages (`pacman`) - like `apt`, can install/remove several packages in a single leaf (`package:` takes a list); modeled on `ansible.builtin.pacman`'s core surface (`state`, `update_cache`/`cache_valid_time`, `upgrade`, `force`, `recurse`, `nosave`). Needs `become: true` (or a specific user) to actually run, since pacman requires root - see [`become`](#become) |
| `yum` | RHEL/CentOS/Fedora packages (`yum`) - like `apt`, can install/remove several packages in a single leaf (`package:` takes a list); modeled on `ansible.builtin.yum`'s core surface (`state`, `update_cache`/`cache_valid_time`, `enablerepo`, `disablerepo`, `exclude`, `disable_gpg_check`). Needs `become: true` (or a specific user) to actually run, since yum requires root - see [`become`](#become) |
| `apk` | Alpine Linux packages (`apk`) - like `apt`, can install/remove several packages in a single leaf (`package:` takes a list); modeled on `community.general.apk`'s core surface (`state`, `update_cache`/`cache_valid_time`, `upgrade`, `repository`, `no_cache`). Needs `become: true` (or a specific user) to actually run, since apk requires root - see [`become`](#become) |
| `snap` | Snap packages (`snap`) - can install/remove several snaps in a single leaf (`package:` takes a list); modeled on `ansible.builtin.snap`'s core surface (`state`, `classic`, `channel`). `state: latest` refreshes an installed snap, falling back to install if it isn't present yet. Needs `become: true` (or a specific user) to actually run, since snap requires root - see [`become`](#become) |
| `flatpak` | Flatpak applications (`flatpak`) - can install/remove several apps in a single leaf (`package:` takes a list); modeled on `community.general.flatpak`'s core surface (`state`, `remote`, `method`). `state: latest` updates an installed app, falling back to install if it isn't present yet. System-scope installs need `become: true`; `method: user` installs don't - see [`become`](#become) |
| `scoop` | Scoop apps on Windows (`scoop`) - can install/remove several apps in a single leaf (`package:` takes a list); modeled on `community.windows.win_scoop`'s core surface (`state`, `global`, `architecture`). `state: latest` updates an installed app, falling back to install if it isn't present yet. Per-user installs (the default) need no elevation; `global: true` does |
| `macports` | MacPorts packages on macOS (`port`) - can install/remove several ports in a single leaf (`package:` takes a list); modeled after `homebrew`'s surface (`state`, `update_cache`). `state: latest` upgrades an installed port, falling back to install if it isn't present yet. Needs `become: true` (or a specific user) to actually run, since port requires root - see [`become`](#become) |
| `pipx` | Python isolated tools (`pipx`) |
| `npm` | Node global packages (`npm -g`) |
| `cargo` | Rust crates (`cargo install`) |
| `go` | Go binaries (`go install`) |
| `eget` | GitHub release binaries (`eget`) |
| `git` | Manage git checkouts (`git`) - see [docs/handlers/git.md](docs/handlers/git.md) |
| `cron` | Cross-platform cron wrapper (Unix cron or Windows scheduled tasks) - see [docs/handlers/cron.md](docs/handlers/cron.md) |
| `cron_unix` | Manage Unix cron entries via `crontab` - see [docs/handlers/cron_unix.md](docs/handlers/cron_unix.md) |
| `cron_file` | Manage system cron-file entries (cron.d style) - see [docs/handlers/cron_file.md](docs/handlers/cron_file.md) |
| `iptables` | Manage iptables/ip6tables rules (`iptables`) - see [docs/handlers/iptables.md](docs/handlers/iptables.md) |
| `ufw` | Manage rules through UFW (`ufw`) - see [docs/handlers/ufw.md](docs/handlers/ufw.md) |
| `advfirewall` | Manage Windows Firewall rules (`netsh advfirewall`) - see [docs/handlers/advfirewall.md](docs/handlers/advfirewall.md) |
| `firewall` | Cross-platform firewall wrapper (auto backend translation) - see [docs/handlers/firewall.md](docs/handlers/firewall.md) |
| `zip` | Download + extract ZIP (no external tool) |
| `symlinks` | Symbolic links (no external tool) |
| `copy` | Copy a local file into place (no external tool) |
| `shell` | Run an inline PowerShell command or script file (no external tool) |
| `blockinfile` | Insert/update/remove a marker-delimited block of text in a file (no external tool, modeled on Ansible's `blockinfile`) |
| `lineinfile` | Ensure/replace/remove line(s) in a text file (no external tool, modeled on Ansible's `lineinfile`) |
| `log` | Print a message at a chosen level (no external tool) |
| `path` | Add/remove directories on the current user's `PATH` (no external tool) |
| `fact` | Set an arbitrary named value for later tasks to reference (no external tool) |
| `async` | Run a nested task list in the background, without waiting for it (no external tool) |
| `wait_for` | Wait for an async job (or a condition) to complete/become true, with a timeout (no external tool) |
| `registry` | Write one or more named values under a registry key (no external tool) |
| `scheduled_task` | Register/update/remove a Windows Task Scheduler task (`ScheduledTasks` module) |
| `group` | Manage local groups on Windows/Linux/macOS |
| `user` | Manage local users on Windows/Linux/macOS |
| `include` | Pull in another document's tasks from `packages/<name>/main.yml` (no external tool) |

Every leaf shares these envelope fields, which sit *beside* its module key, not inside it:

| Field | Values | Description |
| --- | --- | --- |
| `name` | string | Label used in output |
| `tags` | list of strings | Used with `--tags`; cascades down from ancestor tasks |
| `when` | string or list of strings | Gate(s); AND'd with ancestor tasks' `when` |
| `id` | string | Registers this leaf's result for later tasks to reference - leaf actions only. See [Registering results](#registering-results-id) |
| `items` / `with` | list / any value | Materializes this task once per entry (`items`) or exactly once (`with`), templating `${{ item.* }}` first. See [Looping](#looping-withitems) |
| `failed_when` | string or list of strings | Overrides whether this leaf counts as failed. See [Failing a task](#failing-a-task-failed_when-continue_on_error) |
| `continue_on_error` | boolean, default `false` | Keep running past a failed leaf instead of stopping the run. See [Failing a task](#failing-a-task-failed_when-continue_on_error) |
| `become` | boolean or string, default `false` | Run this leaf's command elevated. See [`become`](#become) |

Each module's own fields (documented inline in `main.yml`) still include `state` (`present`/`absent`/`latest`, default `present`).

### `become`

`become: true` runs this leaf's command through `sudo` (or Windows 11's built-in `sudo.exe`, if enabled under Settings > For developers); `become: '<user>'` (e.g. `become: root`, `become: deploy`) elevates to that specific user via `sudo -u <user>` (Windows' `sudo.exe` has no equivalent to an arbitrary `-u <user>` switch - it only elevates to Administrator, so a non-`root` user string there just gets a warning and elevates anyway). `become: false`, or omitting the field, runs unelevated - the default, since ironstate itself is not meant to run elevated and not every task needs to be. Unlike `tags`/`when`, `become` does **not** cascade from a grouping task (`actions:`) down to its children - set it on each leaf action that needs it, matching `continue_on_error`'s own per-leaf (not inherited) scope.

Both Unix `sudo` and Windows' `sudo.exe` may prompt interactively (a password, or a UAC dialog) - that's expected, `become` is meant for interactive use, not unattended automation. If `sudo` isn't on `PATH` at all, the leaf fails outright (no silent fallback to running unelevated) and flows through the normal `failed_when`/`continue_on_error` chain like any other failure.

`become` takes effect for any module whose Install/Uninstall shells out to an external command (every package manager here, plus `git`, `shell`, `iptables`, `ufw`, `cron_unix`, `cron_file`, ...) - which covers the common case of "this needs root" (installing/removing system packages, managing services, editing firewall rules). A handful of modules that mutate the system directly in Go (`file`, `copy`, `registry`, `user`, `group`, ...) don't currently honor `become` - if one of those needs elevated permissions, run ironstate itself elevated instead.

```yaml
- name: install packages via apt (requires root)
  become: true
  apt:
    package: [git, curl, ripgrep]

- name: run a command as a specific user
  become: deploy
  shell:
    command: whoami
```

### `git`

Detailed handler docs and examples: [docs/handlers/git.md](docs/handlers/git.md).

### `cron`

Detailed handler docs and examples: [docs/handlers/cron.md](docs/handlers/cron.md).

### `cron_unix`

Detailed handler docs and examples: [docs/handlers/cron_unix.md](docs/handlers/cron_unix.md).

### `cron_file`

Detailed handler docs and examples: [docs/handlers/cron_file.md](docs/handlers/cron_file.md).

### `iptables`

Detailed handler docs and examples: [docs/handlers/iptables.md](docs/handlers/iptables.md).

### `ufw`

Detailed handler docs and examples: [docs/handlers/ufw.md](docs/handlers/ufw.md).

### `advfirewall`

Detailed handler docs and examples: [docs/handlers/advfirewall.md](docs/handlers/advfirewall.md).

### `firewall`

Detailed handler docs and examples: [docs/handlers/firewall.md](docs/handlers/firewall.md).

### `zip`

| Field | Required | Description |
| --- | --- | --- |
| `src` | yes | URL of the ZIP archive |
| `dest` | yes | Directory to extract into (`~` expansion supported) |
| `creates` | yes | Glob patterns whose presence signals "already installed" |
| `include` | no | Filename patterns to extract (whitelist) |
| `exclude` | no | Filename patterns to skip (blacklist) |
| `sha256.cache` | no | Path to cache the downloaded archive's SHA256 hash; used by `state: latest` to skip unchanged archives |
| `owner` / `group` | no | Ownership metadata applied to extracted files |
| `mode` | no | Mode bits applied to extracted files |

### `symlinks`

A thin wrapper over `file` (`type: link`) - see below. Kept as its own module for the simpler `src`/`dest` shape.

| Field | Required | Description |
| --- | --- | --- |
| `src` | yes | Link target (`~` expansion supported) |
| `dest` | yes | Link path (`~` expansion supported) |
| `force` | no | Replace whatever already exists at `dest` if it isn't already the right symlink. Default `true` - unlike `file`'s own `force` (default `false`), this preserves the original always-replace behavior. Set `false` to warn and skip instead |
| `owner` / `group` | no | Ownership metadata applied after link creation |
| `mode` | no | Mode bits applied after link creation |

### `file`

Modeled on Ansible's [`file`](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/file_module.html): manages a path as a plain file, directory, symlink, or hard link.

| Field | Required | Description |
| --- | --- | --- |
| `path` | yes | Path to manage (`~` expansion supported) |
| `type` | no | `file` (default, creates an empty file if missing, no-op otherwise), `directory` (creates it and any missing parents), `link` (symlink to `src`), `hard` (hard link to `src`), `touch` (always updates the timestamp, creating an empty file first if missing - like Unix `touch`) |
| `src` | one of `type: link`/`type: hard` | Existing path the link points to |
| `force` | no | When `path` already exists as something other than `type`, replace it. Default `false` - warns and skips instead |
| `owner` / `group` | no | Ownership metadata applied after create/touch/link operations |
| `mode` | no | Mode bits applied after create/touch/link operations |

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
| `src` | yes | Source file or directory. Resolved relative to the playbook's own root directory (the directory containing its `main.yml`), or the owning package's own directory — see [Includes](#includes) — unless it's absolute or `~`-prefixed |
| `dest` | yes | Destination path (`~` expansion supported) - a directory when `src` is a directory |
| `owner` / `group` | no | Ownership metadata applied to copied file(s) |
| `mode` | no | Mode bits applied to copied file(s) |

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
| `engine` | yes | Which template engine renders `src`: `jinja` or `gotemplate` (see below) |
| `vars` | no | Extra key/value pairs layered on top of facts/vars/registry for this render only (last-write-wins, shallow merge) |
| `owner` / `group` | no | Ownership metadata applied to `dest` after write |
| `mode` | no | Mode bits applied to `dest` after write |

`present`/`latest` render `src` and write it to `dest` whenever the freshly-rendered content differs from what's already there (a plain string compare - the template equivalent of `copy`'s SHA256 hash compare); `absent` removes `dest`. The render context is the same facts/vars/id-registry context `when`/`${{ }}` already resolve against, with this task's own `vars` layered on top.

Two engines:

- **`jinja`** - a sandboxed, block-capable engine built on the same expression grammar as `when`/`${{ }}` (see [Template expressions](#template-expressions)): `{{ expr }}` output, `{% if %}`/`{% elif %}`/`{% else %}`/`{% endif %}`, `{% for x in iterable %}`/`{% endfor %}` (nesting supported), and `{% set x = expr %}`. No cmdlet/command invocation exists in the grammar at all, so a template can only read/compare/filter context values - never cause a side effect.
- **`gotemplate`** - Go's stdlib [`text/template`](https://pkg.go.dev/text/template) directly, unmodified: real Go template syntax (`{{ .facts.computer_name }}`-style leading-dot field/map-key access, `{{ if }}`/`{{ range }}`/`{{ with }}`, pipelines) against the same render context, rather than `${{ }}`'s own expression grammar. A missing key renders as a zero value (`missingkey=zero`) instead of erroring.

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
| `owner` / `group` | no | Ownership metadata applied to `dest` after write |
| `mode` | no | Mode bits applied to `dest` after write |

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

### `lineinfile`

Modeled on Ansible's [`lineinfile`](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/lineinfile_module.html): ensures a single line is present (or absent), optionally replacing by regex or literal search.

| Field | Required | Description |
| --- | --- | --- |
| `path` | one of `path`/`dest`/`destfile`/`name` | File to manage (`~` expansion supported). `dest`, `destfile`, and `name` are aliases |
| `line` | yes for `state: present`/`latest` | Desired line content |
| `regexp` | no | Regex to match lines. For `present`/`latest`, the last match is replaced. For `absent`, all matches are removed |
| `search_string` | no | Literal substring match alternative to `regexp` (mutually exclusive with `regexp`) |
| `backrefs` | no | `true` lets `line` use backrefs (`\1`, `\g<name>`) from `regexp` matches. If no regex match is found, no insertion occurs |
| `with` | no | Optional replacement-template context. Keys are available as top-level names and under `with`/`input` for `${{ }}` (for example `${{ input.Version }}`) |
| `insertafter` | no | Insertion point (when no match is found): `EOF` (default), `BOF`, or regex. Regex mode inserts after the last match unless `firstmatch: true` |
| `insertbefore` | no | Insertion point alternative to `insertafter`: `BOF` or regex. Regex mode inserts before the last match unless `firstmatch: true` |
| `firstmatch` | no | With regex `insertbefore`/`insertafter`, use the first match instead of the last |
| `create` | no | Create the file (and parent directory) when missing for `present`/`latest`. Default `false` |
| `backup` | no | Write a timestamped backup (`<path>.<yyyyMMddHHmmss>.bak`) before changing. Default `false` |
| `owner` / `group` | no | Ownership metadata applied to `path` after write |
| `mode` | no | Mode bits applied to `path` after write |

`present`/`latest` behavior:

- If `regexp` or `search_string` matches, the last matching line is replaced.
- If no match is found, `line` is inserted at `insertafter`/`insertbefore` (default end-of-file).
- With `backrefs: true`, `regexp` is required and no insertion is done when there is no match.

Replacement string syntax in `line`:

- Regex capture groups (with `backrefs: true`): supports `$1`, `\1`, and `\g<name>`.
- `${{ ... }}` syntax (ironstate expression engine), including `with`/`input` context aliases.
- Go template syntax (`{{ .Version }}` style).
- Jinja-style syntax (`{{ version }}` style).

When both regex captures and templates are used, templates render first, then regex capture replacement is applied.

`absent` behavior:

- Removes all lines matching `regexp` (or containing `search_string`, or exactly equal to `line` when neither matcher is provided).

```yaml
tasks:
  - name: ensure bell is disabled in bash exports
    lineinfile:
      path: ~/.bash/custom/exports/default.bash
      regexp: '^bind .set bell-style none'
      line: "bind 'set bell-style none' 2>/dev/null"
      insertafter: EOF

  - name: rewrite listen directive with regex backrefs
    lineinfile:
      path: ~/.config/myapp.conf
      regexp: '^(listen=).*$'
      line: '\1localhost:9090'
      backrefs: true

  - name: rewrite version using $1 capture syntax
    lineinfile:
      path: ~/.config/myapp.conf
      regexp: '^([Vv]ersion):\s+.*$'
      line: '$1: 1.2.5'
      backrefs: true

  - name: replacement using go template + with context
    lineinfile:
      path: ~/.config/myapp.conf
      regexp: '^Version:'
      line: 'Version: {{ .Version }}'
      with:
        Version: '1.2.5'

  - name: replacement using ${{ }} + input alias
    lineinfile:
      path: ~/.config/myapp.conf
      regexp: '^Version:'
      line: 'Version: ${{ input.Version }}'
      with:
        Version: '1.2.5'

  - name: remove legacy directive lines
    lineinfile:
      path: ~/.config/myapp.conf
      state: absent
      regexp: '^legacy_'
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

### `fail`

Unconditionally fails this leaf (non-zero `rc`) with a message - guard it with `when` to fail only under a specific condition, like Ansible's `fail` module. Reuses the present/absent state machine like `log`: `state: present` (default) or `latest` fails with the `install` message; `state: absent` fails with the `uninstall` message.

| Field | Description |
| --- | --- |
| `message` | Shorthand for `install.message` |
| `exit_code` | The `rc` this leaf reports (default `1`) - applies to whichever phase actually runs |
| `install.message` / `uninstall.message` | Nested form: message used when this resolves to `Install`/`Uninstall` (`state: absent`) |

```yaml
tasks:
  - name: stop the run if a required var is missing
    when: required_setting is not defined
    fail:
      message: "required_setting must be set"
      exit_code: 2
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

**Every `fact` leaf runs before every other task** (see [Architecture](#architecture)) - so a `fact`'s `value`/`when` can only reference gathered facts, vars, and facts registered earlier in this same gather-facts pass. It **cannot** reference another task's `id`-registered result (no non-fact task has run yet); a fact needing a live command's output must compute it itself via its own embedded `shell` instead (see below).

| Field | Required | Description |
| --- | --- | --- |
| `name` | yes | Name this fact is registered under, as `facts.<name>` (not to be confused with the `registry` module below - this is the in-memory fact namespace, not the Windows registry) |
| `shell` | no | Runs this command (same shape as [`shell`](#shell)) to compute the fact's value, instead of a literal `value`. See below |
| `value` | yes, unless `state: absent` or `shell` is given | Any YAML value - scalar, list, or nested mapping. May reference `${{ facts.* }}`/`${{ vars.* }}` (or, with an embedded `shell`, this same task's own `rc`/`stdout`/... - see below), resolved before being stored |

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
```

#### `fact` with an embedded `shell`

A fact can compute its own value by running a command directly - the only way to derive a fact from a live command's output, since a fact can't reference a separately-`id`'d task's result (see above). This command **always actually runs, even without `--apply`** - the same dry-run exception the fact registration itself already gets, since a fact has no real system side effect and a dry-run preview of a later `when`/`${{ }}` reference needs a real value to check, not a zero-result stand-in:

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

### `mount_facts`

Gathers the host's currently-mounted filesystems and registers them as a fact - a task-callable, on-demand counterpart to the fixed set of always-gathered host facts (see [Facts](#facts)), so the (mildly expensive, and platform-specific) work of enumerating mounts only happens when a `main.yml` actually asks for it. Mirrors Ansible's `mount_facts` module, scoped down to "report what's currently mounted" (no `/etc/fstab`-vs-`/proc/mounts` aggregation) plus a `filter` option to drop entries before they're registered.

**Runs in the same facts-first pass as [`fact`](#fact)** (see [Architecture](#architecture)) - a `mount_facts` task's own `when` can only see gathered facts, vars, and facts registered earlier in that same pass. Like `fact`, it reuses the present/absent state machine: `state: present` (default) or `latest` (re)gathers and (re)sets the fact every time it's reached; `state: absent` unsets it. Always actually runs, even without `--apply` - gathering has no real system side effect, so a dry-run preview of a later `when`/`${{ }}` reference needs a real value to check.

Each registered entry is an object with:

| Field | Description |
| --- | --- |
| `source` | Where this entry was read from - a file path (`/proc/mounts`, `/etc/mtab`) on Linux, `"getfsstat"` on macOS, or a fixed Win32-API label (`"GetVolumePathNamesForVolumeName"` for a local volume, `"WNetGetConnection"` for a mapped network drive) on Windows, which has no fstab/mtab equivalent |
| `device` | The underlying volume - a `/dev/...` node on Linux/macOS, a `\\?\Volume{guid}\` volume path for a local Windows volume, or a `\\server\share` UNC path for a mapped network drive |
| `fstype` | Filesystem type (`ext4`, `apfs`, `ntfs`, ...) - blank for a mapped network drive, since resolving it would mean another possibly-slow call to the same remote server |
| `options` | Comma-joined mount options. Linux/macOS report real mount options (`rw,relatime`, ...); Windows synthesizes `rw`/`ro` plus any detected volume flags (e.g. `compressed`), since it has no native options string - blank for a mapped network drive |
| `path` | Where it's mounted - a POSIX path on Linux/macOS, or a drive letter root (`C:\`) or NTFS folder mount point on Windows |

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `name` | no | `mounts` | Name this fact is registered under, as `facts.<name>` |
| `timeout` | no | `10` | Maximum seconds to spend gathering before failing this task (`rc: 1`). `0` means no bound. Mainly matters on Windows, where an unreachable mapped network drive can otherwise stall indefinitely - Linux/macOS gathering is effectively instant (a file read / local syscall), so it practically never trips there |
| `filter` | no | *(none)* | Condition(s) a mount must satisfy to be kept. A single expression string, or a list of expressions (implicit AND) - same bare-expression grammar as `when`/`that`, evaluated per-mount against just that mount's own `device`/`fstype`/`options`/`path`/`source` fields |
| `state` | no | `present` | `present` / `absent` / `latest` |

```yaml
tasks:
  - name: gather mount facts
    mount_facts: {}

  - name: fail if no mounts were found
    assert:
      that:
        - facts.mounts is defined
        - facts.mounts | length > 0

  - name: gather mounts under a different fact name, with a tighter timeout
    mount_facts:
      name: disks
      timeout: 3

  - name: gather only real, non-network NTFS mounts
    mount_facts:
      name: local_ntfs_mounts
      filter:
        - device not in ["none", "drivers"]
        - fstype == "NTFS"
        - source not in ["WNetGetConnection"]
```

### `assert`

Fails this task unless every `that` condition holds. `that` uses the same bare-expression grammar as [`when` conditions](#when-conditions) - dotted/indexed identifiers, `== != < <= > >=`, `and`/`or`/`not`, `in`/`not in`, `is`/`is not`, and `|` filters - evaluated against facts/vars/id-registered results. There's no real idempotent "already applied" state (like `log`/`fact`): the check always fires when reached, and - unlike `log`/`fact` - it's always actually evaluated even without `--apply`, since the check *is* the point and has no system side effect to skip in a dry run.

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

### `async`

Runs a nested `tasks` list in the background and returns immediately - the rest of the playbook continues right away, without waiting for it. A later [`wait_for`](#wait_for) task references the job by its `id` to observe completion.

`id` here is the async job's own handle for `wait_for` to find later - distinct from this task's own top-level `id` (which, like any task, would register this leaf's immediate "started" result, not the background job's eventual outcome). Always actually runs, even without `--apply`, since starting the background job has no meaningful dry-run preview.

The nested tasks run against a snapshot of facts/vars/id-registered results taken when the `async` task runs, against a private, isolated run state - a fact or `id` registered *inside* the background tasks is **not** visible to any other task; only the aggregate result (`rc`/`stdout`/etc. per nested task, under `wait_for`'s own registered `results`) is, once a `wait_for` observes completion.

| Field | Required | Description |
| --- | --- | --- |
| `id` | yes | Job handle name. A `wait_for` task's `for` list references this to wait for this job to finish |
| `tasks` | yes | Task/action list to run in the background, in order - same grammar as the top-level `tasks:` |

```yaml
tasks:
  - name: kick off a slow provisioning script in the background
    async:
      id: provision_widgets
      tasks:
        - shell:
            command: C:\tools\provision-widgets.ps1

  - name: do other work while it runs
    log:
      message: "provisioning started; continuing..."

  - name: wait for provisioning to finish
    wait_for:
      for: provision_widgets
      timeout: 300
```

### `wait_for`

Blocks until every async job named in `for` has finished, and/or `condition` becomes true (both, if both are given - implicit AND), polling every `interval` seconds up to `timeout` seconds. Fails (`rc: 1`) if the timeout elapses first, or if any awaited async job itself failed.

`condition` uses the same bare-expression grammar as [`when` conditions](#when-conditions)/`assert`'s `that`, evaluated against facts/vars/id-registered results captured when this task runs - it is not re-gathered on every poll, so a plain fact/id reference only ever evaluates once; a `condition` is only useful for polling when driven by something that itself checks live state on each call (e.g. a script filter), or combined with `for`. Always actually runs, even without `--apply`.

| Field | Required | Description |
| --- | --- | --- |
| `for` | one of `for`/`condition` | Async job id(s) (see `async`'s `id`) to wait for completion of. A single string, or a list (waits for all of them) |
| `condition` | one of `for`/`condition` | Condition(s) that must all be true before continuing - same grammar as `when`/`that`. A single expression string, or a list of expressions (implicit AND) |
| `timeout` | no | Maximum seconds to wait before failing this task. Default `30` |
| `interval` | no | Seconds to sleep between checks. Default `0.5` |

```yaml
tasks:
  - name: wait up to 5 minutes for provisioning
    wait_for:
      for: [provision_widgets]
      timeout: 300
      interval: 2
```

### `registry`

Writes one or more named values under a single registry key.

| Field | Required | Description |
| --- | --- | --- |
| `path` | yes | Registry key path. Supports hive shortcuts `HKLM`/`HKCU`/`HKCR`/`HKU`/`HKCC` and their `HKEY_*` full names (e.g. `HKEY_LOCAL_MACHINE\Software\...`), with or without a trailing `:` or forward slashes |
| `values` | yes | One or more `{ name, type, value }` entries to write under `path` |
| `owner` / `group` | no | Windows ACL owner/group to apply to the target registry key |

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

Detailed handler docs and examples: [docs/handlers/scheduled_task.md](docs/handlers/scheduled_task.md).

`scheduled_task` also accepts top-level `owner` / `group` aliases that map to `principal.user_id` / `principal.group_id` when those principal fields are omitted.

### `group`

Manage local groups on Windows/Linux/macOS.

| Field | Required | Description |
| --- | --- | --- |
| `name` | yes | Group name |
| `gid` | no | Group id on Unix/macOS |
| `system` | no | Create as a system group where supported |

### `user`

Manage local users on Windows/Linux/macOS.

| Field | Required | Description |
| --- | --- | --- |
| `name` | yes | User name |
| `password` / `password_env` | no | Password value or env-var containing it (platform behavior varies) |
| `uid` | no | User id on Unix/macOS |
| `group` / `gid` | no | Primary group (name or gid) |
| `groups` | no | Supplementary groups |
| `shell` | no | Login shell |
| `home` | no | Home directory |
| `comment` | no | Display/full name/comment |
| `system` | no | Create as a system account where supported |
| `create_home` | no | Linux: create home directory (default `true`) |
| `remove_home` | no | Linux: remove home directory when deleting user |

## Tags filtering

`--tags` is a flat set of tags; a leaf runs if `--tags` is omitted, or if any of its effective tags (its own plus every ancestor task's) match:

```shell
# Anything tagged "cli", anywhere in the tree
./bin/ironstate --tags cli

# Anything tagged "cli" OR "security"
./bin/ironstate --tags cli,security
```

There's no `<group>.<tag>` scheme - `--tags` doesn't know about module names at all, only tags.

A leaf with **no effective tags at all** (neither its own nor any ancestor task's) always runs, regardless of `--tags` - it can't be deliberately targeted or excluded by a tag it doesn't have, so `--tags` simply doesn't apply to it; `when` remains the only gate. This matters for untagged prerequisite tasks a `when` elsewhere depends on - e.g. a `fact` that gathers `facts.wsl_installed` with no `tags:` of its own must still run under `--tags editors` for a later, differently-tagged task's `when: facts.wsl_installed` to see it set at all. Tag an always-relevant task explicitly if you want that documented in the YAML, but it isn't required for it to keep running.

## Includes

A related set of items (an eget binary, a symlink to it, a config file to copy, a setup script...) can be defined once as a package under `packages/<name>/main.yml`, using the same `tasks:` shape as `main.yml` (the explicit form, so it has a `tasks:` key to merge on). Pull it into a run with an `include:` action, wherever you'd write any other action:

```yaml
# main.yml
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

`include` is handled like `actions:` during tree flattening (see [Task/action model](#taskaction-model)): it loads `packages/lolcat/main.yml` and splices its `tasks:` list in at this position, so the included tasks inherit this task's `tags`/`when` exactly like a nested `actions:` list would. Any `copy.src` or `shell.script` path inside a package's `main.yml` is resolved relative to that package's own directory (e.g. `files/kanata.kbd` → `packages/kanata/files/kanata.kbd`), not the playbook's own root directory.

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

Here, a user's `main.yml` sets `languages.java.jdk` to a plain **boolean** (`true`/`false`, matching every other `languages.*` toggle) to enable/disable Java without specifying a package - `toggle(...)` (unlike `default`, which only replaces `null`) treats *any* boolean the same as unset, and falls back to the package's own built-in default; a **string** value instead names a specific override package ID (e.g. `jdk: Eclipse.Temurin.21`). The separate `when: languages.java.jdk != false` is what actually skips the task when the user explicitly disabled it - `toggle(...)`'s job is only to pick the right package ID once the task is known to be enabled. A filter's argument can just as well be a literal (`default('Oracle.JDK.25')`) or a site-level path (`default(vars.editor)`) instead - whichever suits the package. Filters also chain left-to-right (`${{ inputs.name | trim | upper }}`).

If an expression is the **entire** value of a field (e.g. `state: ${{ package.state }}` or `tags: ${{ package.tags }}`), that field is replaced with the referenced value's native type — a string stays a string, an array stays an array. If that whole-value expression can't be resolved, the field is **omitted** instead (a warning is logged either way) - the consuming code's own default applies, rather than injecting a wrongly-typed empty string (see [Looping](#looping-withitems) for the common case: an optional per-item field like `args: ${{ item.args }}`). If the expression is embedded inside a larger string instead (e.g. a `shell.command` or a `copy.dest` path), it's substituted as text, and an unresolved one just blanks that portion of the string (there's no "omit part of a string" equivalent).

```yaml
# main.yml
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
