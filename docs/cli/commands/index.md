---
layout: default
title: 🖥️ Commands
nav_order: 1
has_children: true
parent: 🖥️ CLI
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🖥️ Commands

`ironstate` with no subcommand runs a playbook. See [CLI](../index.md) and [Flags](../flags.md). These subcommands cover everything else:

| Command | Synopsis | Purpose |
| --- | --- | --- |
| [`completion`](./completion.md) | `ironstate completion <bash\|zsh\|fish\|powershell>` | Generate shell completion scripts. |
| [`doctor`](./doctor.md) | `ironstate doctor [--filters-dir dir] [--playbook path]` | Check your environment and a playbook's plugins. |
| [`filters`](./filters.md) | `ironstate filters list [--dir dir]` | List available filters. |
| [`help`](./help.md) | `ironstate help [command]` | Show help. |
| [`init`](./init.md) | `ironstate init [playbook-name] [--scan]` | Scaffold a playbook. |
| [`plugin`](./plugin.md) | `ironstate plugin <install\|list\|info\|update\|uninstall\|test\|bench>` | Manage external handler plugins. |
| [`remote`](./remote.md) | `ironstate remote <ping\|logs\|clean>` | Remote-apply helpers. |
| [`validate`](./validate.md) | `ironstate validate --playbook path` | Schema-validate a playbook. |
| [`version`](./version.md) | `ironstate version` | Print version information. |

Every command accepts `-h`/`--help`.
