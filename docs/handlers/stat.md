---
layout: default
title: stat
parent: 👋 Handlers
nav_order: 40
---

# stat

The `stat` handler gathers file/directory statistics (read-only).

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `path` | yes | - | Path to stat. `~` expansion is supported. |
| `follow` | no | `false` | Follow symlinks. |
| `get_checksum` | no | `false` | Compute checksum. |
| `checksum_algorithm` | no | - | Checksum algorithm to use. |
| `state` | no | `present` | `present` / `absent` / `latest` |

This is a read-only handler (implements `ReadOnlyHandler`).
