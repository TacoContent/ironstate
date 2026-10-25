---
layout: default
title: fail
parent: 👋 Handlers
nav_order: 15
---

# fail

The `fail` handler unconditionally fails this leaf (non-zero `rc`) with a message - guard with `when` to fail only under specific conditions, like Ansible's `fail` module. It reuses the present/absent state machine: `state: present` (default) or `latest` fails with the `install` message; `state: absent` fails with the `uninstall` message.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `message` | no* | - | Shorthand for `install.message` - mutually exclusive with `install`/`uninstall`. |
| `exit_code` | no | `1` | The `rc` this leaf reports - applies to whichever phase actually runs. |
| `install` | no* | - | Phase-specific config for install/present/latest. See `failPhase`. |
| `uninstall` | no* | - | Phase-specific config for uninstall/absent. See `failPhase`. |
| `state` | no | `present` | `present` / `absent` / `latest` |

\* Either provide `message` directly or use `install`/`uninstall` blocks.
