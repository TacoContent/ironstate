---
layout: default
title: 🚀 Init
nav_order: 5
parent: 🖥️ Commands
grand_parent: 🖥️ CLI
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🚀 Init
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
Scaffolds a new playbook.

## Usage

```shell
ironstate init [playbook-name] [--scan]
```

| Argument | Description |
| --- | --- |
| `playbook-name` | Directory to create. If you leave it out, the current directory is scaffolded. |

| Flag | Default | Description |
| --- | --- | --- |
| `--scan` | `false` | Populate the playbook from the current system state instead of creating empty placeholders. |
| `-h`, `--help` | | Show help. |

## Default scaffold

```shell
ironstate init my-playbook
```

```text
my-playbook/
├── main.yml
├── filters/.gitkeep
├── hosts/
│   ├── <computer_name>.yml
│   └── <os_family>.yml
├── packages/.gitkeep
├── roles/.gitkeep
├── tasks/.gitkeep
└── variables/
    └── <user_name>.yml
```

- Every YAML file contains only `vars: {}` and `tasks: []`: a valid, empty document ready to edit.
- `<computer_name>`, `<os_family>`, and `<user_name>` come from this machine's [facts](../../playbooks/facts-vars#facts), lowercased. For example: `hosts/krayt.yml`, `hosts/windows.yml`, `variables/ryan.yml`.
- `.gitkeep` files keep the empty folders in git.
- **Nothing is overwritten.** Existing files are reported as `skip ... (already exists)`, so running `init` again is safe.

```text
$ ironstate init my-playbook
created my-playbook/main.yml
created my-playbook/hosts/krayt.yml
created my-playbook/hosts/windows.yml
created my-playbook/variables/ryan.yml
...

Playbook ready. Try:
  ironstate --playbook my-playbook/main.yml
```

See [Folder structure](../../playbooks/structure) for what each folder is for.

## `--scan`

```shell
ironstate init my-playbook --scan
```

With `--scan`, ironstate asks every scan-capable handler what is already configured on this machine. Examples include packages installed through supported package managers. ironstate then writes those findings into the playbook as tasks, grouped into roles.

- Built-in handlers and installed [plugins](../../plugins/) that implement scanning both participate.
- If any plugin contributed items, it is added to the playbook's `plugins:` list at its installed version.
- A progress indicator shows each handler as it is scanned.

```text
$ ironstate init my-playbook --scan
populated my-playbook from current system state
Use ironstate --playbook my-playbook/main.yml to review the generated playbook.
```

> [!TIP]
> Review a scanned playbook with a dry-run (`ironstate --playbook my-playbook`) before applying it anywhere. Then trim anything you don't want to manage.
