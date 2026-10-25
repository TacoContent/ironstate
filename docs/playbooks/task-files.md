---
render_with_liquid: false
layout: default
title: 📄 Task Files
nav_order: 2
parent: 📖 Playbooks
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 📄 Task Files
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
Every YAML file in a playbook, whether it's `main.yml`, an overlay, or an included package, has the same shape.

## Two forms

**Mapping form**, with top-level keys:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/TacoContent/ironstate/develop/ironstate.schema.json
---
version: "1"
tags: [base]
vars:
  editor: nvim
envs:
  EDITOR: ${{ vars.editor }}
tasks:
  - name: Say hello
    log: { message: "Hello from ${{ facts.computer_name }}" }
```

**List form**, where the document is just the task list:

```yaml
- name: Say hello
  log: { message: "Hello" }
```

Use the list form for short files that need no `vars`, `envs`, or `tags`. Overlay files should use the mapping form, so their `vars` and `tasks` have somewhere to merge.

## Top-level keys

| Key | Type | Description |
| --- | --- | --- |
| `tasks` | list | The tasks to run, in order. See [Tasks](./tasks.md). |
| `vars` | map | Variables for conditions and templates. Overlays **deep-merge** into it. See [Facts and variables](./facts-vars.md). |
| `envs` / `environment` | map | Environment variables for the run, or, in an included file, for that file's tasks. See [Environment variables](./environment.md). |
| `tags` | list | File-level tags added to every task in the file. See [Tags](./tags.md). |
| `plugins` | list | External handler plugins this playbook uses (root playbook). See [Using a plugin](../plugins/using.md). |
| `version` | string | Schema version. Currently `"1"`; optional. |

## How files merge

When ironstate loads a playbook, or an included folder, it reads `main.yml` and then each matching overlay from least to most specific:

- **`tasks` append**: an overlay's tasks run **after** the base file's tasks.
- **`vars` deep-merge**: an overlay can change one nested key without restating the rest.
- **`envs` merge by variable name**: a later declaration replaces that variable's value.
- **`tags`**: a `main.yml`'s tags apply to its own tasks **and** to tasks from its overlays. An overlay's tags apply only to its own tasks.

```yaml
# main.yml
vars:
  shells: { zsh: true, fish: false }

# hosts/windows.yml
vars:
  shells: { pwsh: true }      # result: { zsh: true, fish: false, pwsh: true }
tasks:
  - winget: { package: Microsoft.PowerShell }   # runs after main.yml's tasks
```

## Editor support

Add the schema comment to the first line of any task file. Editors with the YAML language server, such as VS Code with the Red Hat YAML extension, then offer completion, hover docs, and inline validation:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/TacoContent/ironstate/develop/ironstate.schema.json
```

Run [`ironstate validate`](../cli/commands/validate.md) to check the whole playbook from the command line.
