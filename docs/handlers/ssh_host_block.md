---
layout: default
title: ssh_host_block
parent: 👋 Handlers
nav_order: 39
---

# ssh_host_block

The `ssh_host_block` handler renders one or more ssh_config `Host` blocks from structured data into a marker-delimited block in `dest`, reusing `blockinfile`'s marker/insert/backup model.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `dest` | yes | - | ssh_config file to manage. `~` expansion is supported. |
| `hosts` | yes | - | List of host entry objects. |
| `marker` | no | `# {mark} IRONSTATE MANAGED - {name}` | Marker template for both marker lines. |
| `marker_name` | no | task name or dest filename | Identifier substituted for `{name}` in `marker`. |
| `marker_begin` | no | `BEGIN` | Substituted for `{mark}` in opening marker line. |
| `marker_end` | no | `END` | Substituted for `{mark}` in closing marker line. |
| `insertafter` | no | `EOF` | Where to insert block if not found: `EOF`, `BOF`, or a regex. |
| `insertbefore` | no | - | Where to insert block if not found: `BOF` or regex (mutually exclusive with `insertafter`). |
| `firstmatch` | no | `false` | With regex insertbefore/insertafter, use the first matching line instead of the last. |
| `create` | no | `false` | Create target file (and parent directory) if missing. |
| `backup` | no | `false` | Write a timestamped backup copy before changing the file. |
| `owner` | no | - | Ownership applied to target file after write. |
| `group` | no | - | Group applied to target file after write. |
| `mode` | no | - | Mode bits applied to target file after write. |
| `state` | no | `present` | `present` / `absent` / `latest` |

### Host entry fields

Each entry in `hosts`:

- `host` (required): Becomes this entry's `Host <value>` header line.
- `comment` (optional): Rendered verbatim as `# <comment>` line above the `Host` line.
- Other fields: Become `<PascalCaseKey> <value>` (booleans render as `yes`/`no`). List-valued fields are singularized (trailing `s` dropped from PascalCased key) and repeated once per entry. Field names may be snake_case/camelCase/PascalCase.
