---
render_with_liquid: false
layout: default
title: 📦 Includes
nav_order: 12
parent: 📖 Playbooks
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 📦 Includes
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
`include` pulls another folder's tasks into the current spot, as if they were written inline. Use it to split a playbook into reusable **roles**, **packages**, and **task lists**.

```yaml
# main.yml
tasks:
  - name: Kanata keyboard remapper
    tags: [keyboard]
    when: facts.platform == 'windows'
    include:
      name: packages/kanata
      state: present
      with:
        layout: colemak
```

```yaml
# packages/kanata/main.yml
tasks:
  - xget:
      package: jtroo/kanata
      args: [--to=~/.local/bin/kanata]

  - copy:
      src: files/${{ inputs.layout }}.kbd       # -> packages/kanata/files/colemak.kbd
      dest: ~/.kanata/kanata.kbd
      state: ${{ package.state }}
```

## `include` fields

| Field | Required | Description |
| --- | --- | --- |
| `name` | yes | Folder to load, relative to the **playbook root**, such as `roles/shells`, `packages/kanata`, or `hosts/base`. ironstate loads `<name>/main.yml` and that folder's [overlays](./naming). |
| `state` | no | Exposed as `${{ package.state }}` (default `present`). |
| `tags` | no | Exposed as `${{ package.tags }}` (default `[]`). |
| `with` | no | Inputs exposed as `${{ inputs.<key> }}`. |

If the folder or its `main.yml` doesn't exist, ironstate prints a warning and skips the include.

## What the included tasks inherit

The `include` task works like a grouping task:

- `tags`, `when`, and `envs` written **beside** `include:` apply to every included task.
- `state`, `tags`, and `with` written **inside** `include:` are only **inputs**. A package uses them by referencing `${{ package.state }}`, `${{ package.tags }}`, or `${{ inputs.<key> }}` in its own fields. Nothing is applied automatically.

This lets a package decide what an "absent" state means. A package can remove its config file but keep a shared dependency installed, for example.

## Inside a package

| Expression | Value |
| --- | --- |
| `package.name` | The `name` from the `include`. |
| `package.state` | `state`, default `present`. |
| `package.tags` | `tags`, default `[]`. |
| `inputs.<key>` | `with.<key>`; nested paths work. |
| `facts.*`, `vars.*`, results | The same as everywhere else. |

- **Relative paths** in `copy.src`, `template.src`, and `shell.script` resolve from the package folder.
- **Package `vars:`** are defaults visible only inside that package. A site variable with the same name replaces them. See [Package variables](./facts-vars#package-variables).
- **Package `envs:`** apply while the package's tasks run.
- **Overlays** such as `packages/kanata/windows.yml` or `roles/shells/krayt.yml` are merged on top of the package's `main.yml`.
- **Nested includes** also resolve `name` from the playbook root.

## Toggling a whole package

```yaml
# main.yml
- include:
    name: packages/kanata
    state: ${{ keyboard.kanata | ternary('present', 'absent') }}
```

```yaml
# packages/kanata/main.yml
tasks:
  - xget: { package: jtroo/kanata, state: "${{ package.state }}" }
  - copy: { src: files/kanata.kbd, dest: ~/.kanata/kanata.kbd, state: "${{ package.state }}" }
```

To pull tasks from **outside** the playbook, such as a git repository or a shared folder, use [`uses`](./uses).
