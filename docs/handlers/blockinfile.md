---
layout: default
title: blockinfile
parent: 👋 Handlers
nav_order: 6
---

# blockinfile

The `blockinfile` handler manages a marker-delimited block of text inside `dest`, modeled on Ansible's `blockinfile` module. Only the text between the marker lines is ever touched.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `dest` | yes | - | File to manage. `~` expansion is supported. |
| `block` | no* | - | Content to place between the markers. Use YAML block scalar (`|`) for multiline content. One of `block`/`template` required unless `state` is `absent`; `template` wins if both given. |
| `template` | no* | - | Renders a template and uses its output as block content instead of literal `block`. |
| `marker` | no | `# {mark} IRONSTATE MANAGED - {name}` | Template for both marker lines; `{mark}` replaced with `marker_begin`/`marker_end`, `{name}` with `marker_name`. |
| `marker_name` | no | task name or dest filename | Identifier substituted for `{name}` in `marker`. |
| `marker_begin` | no | `BEGIN` | Substituted for `{mark}` in opening marker line. |
| `marker_end` | no | `END` | Substituted for `{mark}` in closing marker line. |
| `insertafter` | no | `EOF` | Where to insert block if not found: `EOF`, `BOF`, or a regex. |
| `insertbefore` | no | - | Where to insert block if not found: `BOF` or regex. Mutually exclusive with `insertafter`. |
| `firstmatch` | no | `false` | With regex insertbefore/insertafter, use the first matching line instead of the last. |
| `create` | no | `false` | Create target file (and parent directory) if missing when `state` present/latest. |
| `backup` | no | `false` | Write a timestamped backup copy before changing the file. |
| `owner` | no | - | Ownership applied to target file after write. |
| `group` | no | - | Group applied to target file after write. |
| `mode` | no | - | Mode bits applied to target file after write. |
| `state` | no | `present` | `present` / `absent` / `latest` |

\* One of `block` or `template` required unless `state` is `absent`.
