---
layout: default
title: ❓ Help
nav_order: 4
parent: 🖥️ Commands
grand_parent: 🖥️ CLI
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# ❓ Help
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
Shows help for ironstate or any of its commands.

## Usage

```shell
ironstate help [command [subcommand]]
ironstate [command] --help
ironstate [command] -h
```

## Examples

```shell
ironstate help                 # top-level usage and every playbook-run flag
ironstate help plugin          # plugin subcommands
ironstate help plugin bench    # flags for 'plugin bench'
ironstate remote ping --help   # same thing, as a flag
```

Help text includes each command's synopsis, description, flags with their defaults, and available subcommands. The [CLI reference](../index.md) and the [Flags](../flags.md) page cover the same information in more detail.
