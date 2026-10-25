---
layout: default
title: 🖥️ Hosts and Variables
nav_order: 5
parent: 📖 Playbooks
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🖥️ Hosts and Variables
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
The `hosts/` and `variables/` folders hold **overlays**: files that are merged on top of `main.yml` when their name matches the machine. Both folders follow the [file naming](./naming.md) rules.

## `hosts/`: per machine

Use `hosts/` for anything that depends on **which machine** it is:

```text
hosts/
├── main.yml          # every machine (merged first)
├── windows.yml       # every Windows machine
├── archlinux.yml     # every Arch-family machine
├── krayt.yml         # only the machine named krayt
└── base/main.yml     # shared tasks that host files include
```

```yaml
# hosts/krayt.yml
vars:
  development:
    editors: { vscode: true }

tasks:
  - name: Gaming
    tags: [gaming]
    winget: { package: Valve.Steam }

  - name: Shared base setup
    include: { name: hosts/base }
```

Find the hostname with `hostname` (Linux and macOS) or `$env:COMPUTERNAME` (Windows). Run `ironstate init` to create `hosts/<hostname>.yml` and `hosts/<os_family>.yml` for the current machine.

Relative paths inside `hosts/` and `variables/` files, such as `copy.src`, resolve from the **playbook root**, not from the overlay's folder.

## `variables/`: per user

`variables/` accepts the same machine overlays, plus one more: **`<USERNAME>.yml`**, named after the `USERNAME` environment variable, which merges after all the others. Use it for settings that follow a person across machines:

```yaml
# variables/ryan.yml
vars:
  editor: nvim
  git:
    user: { name: Ryan, email: ryan@example.com }

tasks:
  - tags: [cli]
    pipx: { package: posting }
```

Windows always sets `USERNAME`. Many Linux and macOS shells set only `USER`, so the `<USERNAME>.yml` file is skipped there unless you export `USERNAME` (for example, `export USERNAME=$USER` in your shell profile). Machine overlays such as `variables/linux.yml` and `variables/<hostname>.yml` work everywhere.

## Merge order

```text
main.yml
hosts/main.yml
hosts/<overlays>           least to most specific
variables/main.yml
variables/<overlays>       least to most specific
variables/<USERNAME>.yml
```

After that, inventory variables, `--vars-file`, and `--var` are applied. See [Variable precedence](./facts-vars.md#variable-precedence).

## Patterns

**Shared host baseline**: put common tasks in `hosts/base/main.yml` and include it from each machine file. Machines that don't include it skip it.

**Toggles in vars, logic in roles**: keep roles generic and controlled by variables, and turn features on per machine:

```yaml
# roles/development/main.yml
tasks:
  - name: VS Code
    when: development | enabled('editors', 'vscode')
    winget: { package: Microsoft.VisualStudioCode }

# hosts/krayt.yml
vars:
  development:
    editors: { vscode: true }
```

**No file needed for remote hosts**: hosts managed through an [inventory](./inventory.md) can carry their `vars` in the inventory instead of in a `hosts/` file.
