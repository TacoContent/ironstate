---
render_with_liquid: false
layout: default
title: 🌱 Environment Variables
nav_order: 11
parent: 📖 Playbooks
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🌱 Environment Variables
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
`envs` (or its alias `environment`) sets environment variables for the processes that ironstate runs: shell commands, package managers, and script filters. Values can use `${{ }}` expressions.

## Scopes

| Where | Applies |
| --- | --- |
| Playbook root (`main.yml` and its overlays) | For the whole run. |
| Top of an included or used file | While that package's tasks run. |
| On a grouping task | For every child. |
| On a leaf | Only for that leaf. |

Inner scopes build on outer ones, and the previous values are restored when a scope ends.

```yaml
vars:
  greeting: hello
envs:
  GREETING: ${{ vars.greeting }}

tasks:
  - name: Temporary override
    envs:
      GREETING: hi
    shell: echo $GREETING          # hi

  - shell: echo $GREETING          # hello
```

Read the current value in expressions with `envs.NAME`, which includes inherited OS variables, or with the [`env`](../filters/builtin.md#env) filter.

## `PATH`

Extend `PATH` by referencing its current value. Use `:` on Linux and macOS, and `;` on Windows. A leading `~` in each entry is expanded:

```yaml
# roles/tools/main.yml
envs:
  PATH: ${{ facts.local_bin_path }}:${{ envs.PATH }}
tasks:
  - shell: my-tool --version
```

On Windows, ironstate also picks up new entries from the persistent machine and user `PATH` before each task. Tools installed by earlier tasks, such as Scoop apps, are then found without restarting the terminal.

## Value sources (`valueFrom`)

Instead of a plain value, an entry can read its value from a file or a command:

```yaml
environment:
  API_URL: https://api.example.com
  FROM_FILE:
    valueFrom:
      file: ./config/token.txt         # relative to this file's playbook or package
  FROM_COMMAND:
    valueFrom:
      task:
        shell:
          command: git rev-parse --short HEAD
        value: ${{ stdout | trim }}
```

- **`file`**: the file's entire contents, verbatim (including trailing newlines). `~` and expressions are allowed in the path.
- **`task`**: runs the optional `shell`, then evaluates `value` with its `rc`, `stdout`, and `stderr`. It doesn't register a fact or `id`.
- Sources run **during dry-runs too**. Keep commands read-only.
- Root sources run once at startup. Package and group sources run once, the first time one of their tasks passes `when`.
- A missing file, a failed command, or a non-scalar value stops the run with an error.

## Rules

- `envs` and `environment` can both appear in the same scope, but not with conflicting values for the same key.
- Values must be scalars. `null` becomes an empty string.
- Overlays replace a variable's whole definition; they don't merge `valueFrom` objects.
- A task's `when` is evaluated **before** that task's own `envs` are applied.
