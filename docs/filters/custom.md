---
render_with_liquid: false
layout: default
title: 🛠️ Custom Filters
nav_order: 2
parent: 🧹 Filters
---
<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🛠️ Custom Filters
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
A playbook can add its own filters without recompiling ironstate. Drop a script into the playbook's `filters/` folder, and it becomes a filter named after the file:

```text
my-playbook/
├── main.yml
└── filters/
    ├── leet.sh        # -> the 'leet' filter
    └── slugify.ps1    # -> the 'slugify' filter
```

```yaml
message: "${{ 'hello world' | slugify | leet }}"
```

## Supported languages

| Extension | Language | Interpreter (default) | Result type | Kept warm |
| --- | --- | --- | --- | --- |
| `.ps1` | PowerShell | `pwsh -NoProfile -File` | any JSON value | yes |
| `.sh` | bash (4+) | `bash` | string | yes |
| `.zsh` | zsh | `zsh` | string | yes |
| `.fish` | fish | `fish --no-config` | string | yes |
| `.nu` | nushell | `nu --stdin --no-config-file` | string | no (one process per call) |

Each language has a small embedded **shim** that ironstate runs with the interpreter. The shim loads your script, reads requests as JSON on stdin, calls your code, and writes the result back as JSON. Your script only implements the filter logic.

> [!IMPORTANT]
> The `.sh`, `.zsh`, and `.fish` shims need [`jq`](https://jqlang.github.io/jq/) on `PATH` to decode and encode requests.

## Discovery rules

- **Location**: `filters/` next to the root playbook file, or the folder set by `filters.dir` in [`ironstate.yaml`](../cli/flags.md#ironstateyaml). A relative `filters.dir` is resolved from the playbook's directory.
- **Name**: the file name without its extension (`slugify.ps1` becomes `slugify`). Subfolders and files with unrecognized extensions are ignored, so a `filters/README.md` is fine.
- **Built-ins win**: a script named after a [built-in filter](./builtin.md) (for example, `upper.sh`) is never registered.
- **One script per name**: when several languages implement the same name, the winner is picked in this order: **PowerShell → bash → zsh → fish → nushell**.
- **Lifetime**: each script's interpreter starts the first time the filter is used and stays running for the rest of the run, so later calls are fast. Nushell is the exception: it starts a new `nu` process for every call.

Check what was discovered:

```shell
ironstate filters list --dir path/to/playbook/filters
```

## PowerShell (`.ps1`)

Declare a `param` block with the piped value and the argument list, then return the result. Any JSON-serializable value works: strings, numbers, booleans, arrays, hashtables, or `$null`.

```powershell
# filters/slugify.ps1
param($Value, [object[]] $ArgValues)

if ($null -eq $Value) { return $null }
$separator = if ($ArgValues.Count -gt 0) { [string]$ArgValues[0] } else { '-' }
($Value.ToString().ToLowerInvariant() -replace '[^a-z0-9]+', $separator).Trim($separator)
```

```yaml
"${{ 'Hello, World!' | slugify }}"        # hello-world
"${{ 'Hello, World!' | slugify('_') }}"   # hello_world
```

Throwing an exception (`throw "bad input"`) fails the expression with that message.

## bash (`.sh`), zsh (`.zsh`), fish (`.fish`)

Your script is run with positional arguments:

- `$1` is the piped value (an empty string when `null`); in fish, it's `$argv[1]`.
- `$2`, `$3`, … are the filter arguments. They're always strings: `null` becomes an empty string, and lists and maps arrive as compact JSON.
- **stdout** is the result, as a string, with trailing newlines removed.
- A **non-zero exit** fails the expression, using stderr as the message. Stderr from a successful run is ignored.

```bash
# filters/leet.sh
#!/usr/bin/env bash
value="$1"
printf '%s' "$value" | tr 'aeiot' '43107'
```

```fish
# filters/shout.fish
set value $argv[1]
set suffix (test (count $argv) -ge 2; and echo $argv[2]; or echo '!')
echo (string upper -- $value)$suffix
```

```yaml
"${{ 'elite' | leet }}"          # 3l173
"${{ 'hi' | shout('!!!') }}"     # HI!!!
```

> [!WARNING]
> On Windows, the `bash` that's often found first on `PATH` is the WSL launcher (`System32\bash.exe`), which mangles Windows paths. ironstate skips it and uses Git for Windows, MSYS2, or Cygwin bash instead.

## nushell (`.nu`)

Define a `main` command that takes the value and a rest parameter of arguments, and print the result:

```nu
# filters/reverse.nu
def main [value, ...args] {
  $value | str reverse
}
```

nushell can't serve several requests over one open stdin, so every call starts a new `nu` process. Prefer another language for filters that run inside large loops.

## Choosing interpreters

Override the interpreter command for each extension in `ironstate.yaml`. Keys are extensions **with** the leading dot, and values are the command and arguments; the shim path and your script path are appended to them.

```yaml
# ironstate.yaml
filters:
  dir: filters
  interpreters:
    .ps1: [powershell.exe, -NoProfile, -ExecutionPolicy, Bypass, -File]   # Windows PowerShell 5.1
    .sh: [/opt/homebrew/bin/bash]                                         # a newer bash on macOS
    .zsh: [zsh]
    .fish: [fish, --no-config]
    .nu: [nu, --stdin, --no-config-file]
```

> [!IMPORTANT]
> Setting `filters.interpreters` **replaces** the default table. Only the extensions you list are discovered, so include every language your playbook uses. Only extensions that ship with a shim (`.ps1`, `.sh`, `.zsh`, `.fish`, `.nu`) can be used.

## Remote apply

When you run a playbook with `--target` or `--inventory`, the playbook directory, including `filters/`, is sent to each target. A `filters.dir` outside the playbook and your `filters.*` config are sent too. Filters run **on the target**, so the target needs the interpreter, plus `jq` for the shell languages.

## Tips

- Keep filters **pure**: compute a value from the input and arguments. Use tasks for side effects.
- Return `null` (PowerShell) or empty output (shell languages) for "no value", so `| default(...)` works afterward.
- When a filter needs structured output (maps or lists), write it in PowerShell. The shell languages always return strings, so pipe them through [`from_json`](./builtin.md#from_json) if they print JSON.
- When a filter becomes widely useful, consider contributing it as a built-in. See [Contributing](../contributing.md).
