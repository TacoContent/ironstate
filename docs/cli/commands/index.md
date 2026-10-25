---
layout: default
title: 🖥️ Commands
nav_order: 1
has_children: true
parent: 🖥️ CLI
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🖥️ Commands

`ironstate` with no subcommand runs a playbook. See [CLI](../) and [Flags](../flags). These subcommands cover everything else:

| Command | Synopsis | Purpose |
| --- | --- | --- |
| [`completion`](./completion) | `ironstate completion <bash\|zsh\|fish\|powershell>` | Generate shell completion scripts. |
| [`doctor`](./doctor) | `ironstate doctor [--filters-dir dir] [--playbook path]` | Check your environment and a playbook's plugins. |
| [`filters`](./filters) | `ironstate filters list [--dir dir]` | List available filters. |
| [`help`](./help) | `ironstate help [command]` | Show help. |
| [`init`](./init) | `ironstate init [playbook-name] [--scan]` | Scaffold a playbook. |
| [`plugin`](./plugin) | `ironstate plugin <install\|list\|info\|update\|uninstall\|test\|bench>` | Manage external handler plugins. |
| [`remote`](./remote) | `ironstate remote <ping\|logs\|clean>` | Remote-apply helpers. |
| [`validate`](./validate) | `ironstate validate --playbook path` | Schema-validate a playbook. |
| [`version`](./version) | `ironstate version` | Print version information. |

Every command accepts `-h`/`--help`.
