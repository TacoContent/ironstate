---
render_with_liquid: false
layout: default
title: 🧩 Tasks
nav_order: 3
parent: 📖 Playbooks
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🧩 Tasks
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
A task list is made of three kinds of item. ironstate works out each item's kind from its keys.

| Has… | Kind | What it does |
| --- | --- | --- |
| a module key (`winget`, `shell`, `copy`, …) | **leaf action** | Does one thing. |
| `actions:` | **grouping task** | Holds a nested task list. Its `tags` and `when` apply to everything inside it. |
| `include:` or `uses:` | **grouping task** | Like `actions:`, but the nested list comes from another file. See [Includes](./includes.md) and [Remote sources](./uses.md). |

## Leaf actions

A leaf has exactly **one** module key. The module's settings go inside that key, and the shared task fields sit beside it:

```yaml
- name: Install ripgrep
  tags: [cli, search]
  when: facts.platform == 'windows'
  winget:
    package: BurntSushi.ripgrep.MSVC
    state: present
```

Most modules accept `state`: `present` (the default), `absent`, and, for package managers, `latest`. Every run checks the current state first, so running the same playbook twice changes nothing the second time.

Some modules have a short form. A `shell` task, for example, can be just the command string:

```yaml
- name: Show the kernel
  shell: uname -a
```

See [Handlers](../handlers/index.md) for every module and its fields.

## Grouping tasks

Use `actions:` to group related tasks, so you can tag, gate, or loop them together. Groups can be nested to any depth.

```yaml
- name: Linux desktop
  tags: [desktop]
  when: facts.platform == 'linux'
  actions:
    - name: Terminal
      pacman: { package: [kitty] }
      become: true
    - name: GNOME tweaks
      when: facts.distribution == 'manjaro'
      actions:
        - shell: gsettings set org.gnome.desktop.interface color-scheme prefer-dark
```

- **`tags` accumulate**: a leaf's tags are its own tags plus the tags of every enclosing group.
- **`when` combines with AND**: a leaf runs only if its own `when` **and** every enclosing group's `when` are true.
- **`become`**, **`id`**, and **`continue_on_error`** do **not** cascade. Set them on each leaf that needs them.
- **`envs`** set on a group apply to every child.

## Shared task fields

These fields sit **beside** the module key on any task:

| Field | Type | Applies to | Description |
| --- | --- | --- | --- |
| `name` | string | any | Label shown in output. |
| `tags` | list | any (cascades) | Used by `--tags`. See [Tags](./tags.md). |
| `when` | string or list | any (cascades, AND) | Run only when true. See [Conditions](./conditions.md). |
| `items` / `with` | list / any | any | Repeat this task for each item, or bind one value. See [Loops](./loops.md). |
| `envs` / `environment` | map | any (cascades) | Environment overrides. See [Environment variables](./environment.md). |
| `id` | string | leaf | Save this leaf's result for later tasks. See [Results](./results.md#registering-results-with-id). |
| `failed_when` | expression(s) or shell checks | leaf | Decide whether this leaf failed. See [Results](./results.md#failed_when). |
| `continue_on_error` | bool | leaf | Keep going when this leaf fails. |
| `become` | bool or user name | leaf | Run elevated with `sudo`. See [Results](./results.md#become). |
| `creates` / `removes` | path or list | `shell` leaf | Skip the shell command when these paths already exist, or no longer exist. See [shell](../handlers/shell.md). |

## Order of execution

1. All `fact`-producing tasks (`fact`, `mount_facts`, and plugin fact producers) run first, in document order, even during a dry-run.
2. Every other leaf then runs in the order written, top to bottom, across files and includes.
3. Each leaf's `when` and `${{ }}` expressions are resolved **just before it runs**, so results from earlier leaves are available.

## Dry-run vs. apply

Without `--apply`, ironstate checks each leaf and reports what it **would** change, without changing anything. A few things still run in a dry-run because they only read the system:

- `fact` tasks, including any embedded `shell` that computes a value.
- `failed_when` shell checks.
- `valueFrom.task` environment sources.

Keep those commands read-only.
