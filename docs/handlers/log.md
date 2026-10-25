---
layout: default
title: log
parent: 👋 Handlers
nav_order: 26
---

# log

The `log` handler prints a message at a chosen level. It reuses the present/absent state machine: `state: present` (default) or `latest` print the `install` message; `state: absent` prints the `uninstall` message.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `message` | no* | - | Shorthand for `install.message` - mutually exclusive with `install`/`uninstall`. |
| `level` | no | - | Log level (see `logLevel` in schema). |
| `install` | no* | - | Phase-specific config for install/present/latest. See `logPhase`. |
| `uninstall` | no* | - | Phase-specific config for uninstall/absent. See `logPhase`. |
| `state` | no | `present` | `present` / `absent` / `latest` |

\* Either provide `message`/`level` directly or use `install`/`uninstall` blocks.
