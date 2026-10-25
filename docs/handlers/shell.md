---
layout: default
title: shell
parent: 👋 Handlers
nav_order: 37
---

# shell

The `shell` handler runs inline PowerShell commands or script files. It can be specified as a shorthand string or as a structured object.

## Fields (when used as object)

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `command` | yes* | - | Inline command(s) to execute. Use a YAML block scalar (`\|`) for multiline scripts. Mutually exclusive with `script`. |
| `script` | yes* | - | Path to a script to execute. Resolved relative to the install system directory or owning package directory unless absolute or `~`-prefixed. Mutually exclusive with `command`. |
| `type` | no | - | Interpreter type (e.g. `bash`, `python`, `python3.11`). Takes precedence over `host`. If omitted, uses `SHELL` env var, falling back to `pwsh` on Windows and `bash` elsewhere. |
| `host` | no | - | Legacy interpreter/command selector. `type` takes precedence. Recognized presets expand executables; other values split as exe + args. |
| `extension` | no | - | Overrides temp file's extension when using `command` with non-`pwsh` host. |
| `args` | no | - | Arguments passed to the command/script. |
| `creates` | no | - | Glob patterns whose presence signals "already run". If omitted, command always runs for `state` `present`/`latest`. For `state` `absent`, paths are removed instead. |
| `removes` | no | - | Glob patterns the command is expected to remove. For `state` `present`/`latest`, considered already run when every pattern absent; for `state` `absent`, runs while any pattern remains. |
| `silent` | no | `false` | Suppress logging of captured stdout/stderr and shell exit-code warnings unless the task fails. Output still captured for IDs/facts. |
| `state` | no | `present` | `present` / `absent` / `latest` |

\* Exactly one of `command` or `script` must be provided.

## Usage

Shorthand form:
```yaml
tasks:
  - shell: echo hello
```

Object form:
```yaml
tasks:
  - name: Run a script
    shell:
      script: scripts/setup.sh
      args: ["--force"]
      creates:
        - /opt/myapp/installed.flag
      state: present
```
