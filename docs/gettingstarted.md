---
render_with_liquid: false
layout: default
title: 🏁 Getting Started
nav_order: 3
has_children: false
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🏁 Getting Started
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

This page walks you from a freshly [installed](./installation.md) `ironstate` binary to your first applied playbook. It takes about five minutes.

## 1. Check your environment

```shell
ironstate version
ironstate doctor
```

`doctor` lists which package managers and shells are on your `PATH`. A `[missing]` line only matters if your playbook uses that tool. See [doctor](./cli/commands/doctor.md).

## 2. Scaffold a playbook

```shell
ironstate init my-playbook
cd my-playbook
```

This creates a starter layout:

```text
my-playbook/
├── main.yml                  ← the base task file, always loaded
├── filters/                  ← custom script filters
├── hosts/
│   ├── <your-hostname>.yml   ← tasks/vars for this machine only
│   └── <your-os-family>.yml  ← tasks/vars for every machine of this OS family
├── packages/                 ← reusable bundles you 'include'
├── roles/                    ← higher-level groupings you 'include'
├── tasks/                    ← shared task lists you 'include'
└── variables/
    └── <your-username>.yml   ← tasks/vars for your user account
```

Every generated YAML file contains just `vars: {}` and `tasks: []`. Running `init` again never overwrites existing files. See [Folder structure](./playbooks/structure.md) for what each folder is for.

> [!TIP]
> `ironstate init my-playbook --scan` fills the playbook from the software already installed on this machine. That's a quick way to capture an existing setup. See [init](./cli/commands/init.md).

## 3. Write a task

Open `main.yml` and replace its contents:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/TacoContent/ironstate/develop/ironstate.schema.json
vars:
  greeting: Hello

tasks:
  - name: Say hello
    log:
      message: "${{ vars.greeting }} from ${{ facts.computer_name }} (${{ facts.platform }}/${{ facts.arch }})"

  - name: Make sure a config directory exists
    tags: [config]
    shell:
      command: mkdir -p ~/.config/my-tool
      type: bash
    creates:
      - ~/.config/my-tool
    when: platform != "windows"
```

- `vars` holds your own values; `facts` are gathered from the machine on every run.
- Each task has one **handler** key (`log`, `shell`, `winget`, ...) plus envelope fields such as `name`, `tags`, and `when`.
- `creates` makes the shell task idempotent: once the path exists, the task is skipped.

The `yaml-language-server` comment gives you completion and validation in editors that support it, such as VS Code with the YAML extension.

## 4. Validate and preview

```shell
ironstate validate --playbook .
ironstate --playbook .
```

`validate` checks every YAML file against the schema. A plain run is a **dry-run**: ironstate prints a table of what it would do and changes nothing. Add `-v` to also see tasks that are already satisfied.

## 5. Apply

```shell
ironstate --playbook . --apply
```

Run it a second time and the shell task reports as already satisfied, because the directory now exists.

## 6. Run a subset with tags

```shell
ironstate --playbook . --tags config --apply
```

Only tasks tagged `config` run, along with any task that has no tags at all. See [Tags](./playbooks/tags.md).

## 7. Add a machine-specific override

Put tasks or vars that only apply to this machine into `hosts/<your-hostname>.yml`:

```yaml
vars:
  greeting: Howdy

tasks:
  - name: Only on this machine
    log:
      message: This task runs on one host only.
```

Overlay `vars` deep-merge over `main.yml`, and overlay `tasks` are appended. See [Task file naming](./playbooks/naming.md) for every name you can use (`windows.yml`, `ubuntu.yml`, `archlinux.yml`, `amd64.yml`, `myhost.amd64.yml`, ...).

## Next steps

- [Playbooks](./playbooks/index.md): the full playbook format.
- [Handlers](./handlers/index.md): every built-in handler and its fields.
- [Filters](./filters/index.md): transforming values in `when` and `${{ }}`.
- [CLI](./cli/index.md): every command and flag.
- [Inventory files](./playbooks/inventory.md): applying a playbook to other machines over SSH.
