---
render_with_liquid: false
layout: default
title: lineinfile
parent: 👋 Handlers
nav_order: 25
---

# lineinfile

The `lineinfile` handler manages a single line in a text file, modeled on Ansible's `lineinfile` module.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `path` / `dest` / `destfile` / `name` | yes* | - | File to manage. `~` expansion is supported. Any of these aliases accepted. |
| `line` | no* | - | Line to ensure exists (or replacement text when `regexp`/`search_string` matches). Required for `state` present/latest. Supports regex backrefs and templating (`${{ ... }}`, Go templates, Jinja). |
| `regexp` | no | - | Regex to match target line(s). For present/latest, the last matching line is replaced. For absent, all matching lines are removed. Mutually exclusive with `search_string`. |
| `search_string` | no | - | Literal substring match alternative to `regexp`. Mutually exclusive with `regexp`. |
| `backrefs` | no | `false` | When true (requires `regexp`), `line` may contain regex backrefs. If `regexp` has no match, the file is left unchanged. |
| `with` | no | - | Optional context values for templated `line` rendering. |
| `insertafter` | no | `EOF` | Insertion location when no match found: `EOF`, `BOF`, or a regex. |
| `insertbefore` | no | - | Insertion location when no match found: `BOF` or regex. Mutually exclusive with `insertafter`. |
| `firstmatch` | no | `false` | With regex insertbefore/insertafter, use the first matching line instead of the last. |
| `create` | no | `false` | Create target file (and parent directory) when missing for state present/latest. |
| `backup` | no | `false` | Write a timestamped backup copy before changing the file. |
| `owner` | no | - | Ownership applied to target file after write. |
| `group` | no | - | Group applied to target file after write. |
| `mode` | no | - | Mode bits applied to target file after write. |
| `state` | no | `present` | `present` / `absent` / `latest` |

\* At least one path alias required. `line` required for present/latest.
