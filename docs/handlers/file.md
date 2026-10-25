---
layout: default
title: file
parent: 👋 Handlers
nav_order: 15.5
---

# file

The `file` handler manages a path as a plain file, a directory, a symlink, or a hard link. It's modeled on Ansible's `file` module.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `path` | yes | - | Path to manage. `~` is expanded. |
| `type` | no | `file` | `file`, `directory`, `link`, `hard`, or `touch`. See below. |
| `src` | for `link` and `hard` | - | The existing path the link points to. |
| `force` | no | `false` | Replace `path` if it already exists as something other than `type`. Without it, the task warns and skips. |
| `owner` / `group` | no | - | Ownership applied after the path is created. |
| `mode` | no | - | Mode bits applied after the path is created. |
| `state` | no | `present` | `present` / `absent` / `latest` |

## Types

| `type` | Behavior |
| --- | --- |
| `file` | Creates an empty file if it's missing. An existing file is left alone. |
| `directory` | Creates the directory and any missing parents. |
| `link` | Creates a symlink to `src`. An existing symlink with a different target counts as not present. |
| `hard` | Creates a hard link to `src`. It counts as present when `path` and `src` have the same content. |
| `touch` | Always runs. Creates an empty file if it's missing, otherwise updates its timestamp. |

Notes:

- Unlike Ansible, `state` keeps its usual meaning (`present`/`absent`/`latest`). The kind of path goes in `type`.
- `state: absent` removes whatever is at `path`. A real directory is removed recursively, but a symlink or hard link is only unlinked, so removing a directory symlink never deletes the target's contents.
- For `link` and `hard`, a missing `src` produces a warning and the task is skipped.
- When `owner`, `group`, or `mode` is set, the task runs every time so the metadata is reapplied.
- `file` changes the filesystem directly, so it doesn't honor `become`. Run ironstate elevated if the path needs it.

## Examples

```yaml
tasks:
  - name: custom PowerShell scripts directory
    file:
      path: ~/.config/powershell/custom
      type: directory

  - name: pin a specific config as the active one
    file:
      path: ~/.config/app/active.json
      type: link
      src: ~/.config/app/profiles/work.json

  - name: remove an old cache directory
    file:
      path: ~/.cache/old-tool
      state: absent
```
