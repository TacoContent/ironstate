---
layout: default
title: 🗂️ Folder Structure
nav_order: 1
parent: 📖 Playbooks
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🗂️ Folder Structure
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
A playbook is an ordinary folder. Only `main.yml` is required; every other folder is a convention that ironstate either loads automatically or that you reference yourself.

```text
my-playbook/
├── main.yml                 # the root task file (required)
├── ironstate.lock.yaml      # pinned plugin versions (optional, generated)
├── .ironstateignore         # files to leave out of remote bundles (optional)
├── hosts/                   # machine-specific overlays   (loaded automatically)
│   ├── main.yml
│   ├── windows.yml
│   └── krayt.yml
├── variables/               # user-specific overlays      (loaded automatically)
│   ├── main.yml
│   └── ryan.yml
├── filters/                 # custom script filters       (loaded automatically)
│   └── slugify.ps1
├── roles/                   # groups of related tasks     (pulled in with include)
│   └── shells/
│       ├── main.yml
│       └── windows.yml
├── packages/                # one reusable unit per tool  (pulled in with include)
│   └── kanata/
│       ├── main.yml
│       └── files/kanata.kbd
├── tasks/                   # shared task lists           (pulled in with include)
│   └── debian-based/main.yml
└── inventory/               # remote hosts (optional; see the note below)
    └── main.yml
```

`ironstate init` creates this layout for you. See [`ironstate init`](../cli/commands/init.md).

## Automatically loaded

| Path | Purpose |
| --- | --- |
| `main.yml` | The root task file. ironstate also accepts `site.yml`, `site.yaml`, or `main.yaml`. |
| `hosts/` | Overlays chosen by **machine** characteristics: hostname, OS family, distribution, platform, architecture. See [Hosts and variables](./hosts.md). |
| `variables/` | The same overlays, plus a `<username>.yml` that merges last. Use it for per-user settings. |
| `filters/` | Script filters, registered by file name. See [Custom filters](../filters/custom.md). |
| `ironstate.lock.yaml` | Plugin versions and checksums. See [Using a plugin](../plugins/using.md). |

## Referenced by you

`roles/`, `packages/`, and `tasks/` aren't loaded on their own. You pull them in with an [`include`](./includes.md), which names a folder **relative to the playbook root**:

```yaml
- include: { name: roles/shells }
- include: { name: packages/kanata }
- include: { name: tasks/debian-based }
```

The folder names are only conventions. ironstate treats them the same, so organize them however you like:

- **`roles/`**: larger areas of a machine, such as shells, development, or gaming.
- **`packages/`**: one tool and everything it needs, such as its binary, config file, and symlink.
- **`tasks/`**: small shared task lists reused by several roles.

Each included folder has its own `main.yml` and can carry its own [overlays](./naming.md) (`windows.yml`, `krayt.yml`, …) and supporting files. Relative paths in `copy.src`, `template.src`, and `shell.script` resolve from the included folder.

## Where to keep the inventory

An [inventory](./inventory.md) can live anywhere. [`ironstate validate`](../cli/commands/validate.md) checks every YAML file under the playbook folder against the playbook schema, so an inventory kept **inside** the playbook folder fails validation. Keep it beside the playbook instead if you rely on `validate`.

## Running from another directory

`--playbook` accepts a file, a directory, or a bare name:

```shell
ironstate --playbook ~/dotfiles/my-playbook           # directory: finds site.yml / main.yml
ironstate --playbook ~/dotfiles/my-playbook/main.yml  # explicit file
ironstate --playbook playbooks/camalot                # bare name: also tries playbooks/camalot.yml
ironstate                                             # current directory
```

`.env` and `.secrets` are read from the **current directory**, not the playbook folder. See [Secrets](./secrets.md).
