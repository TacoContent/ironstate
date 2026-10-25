---
layout: default
title: zip
parent: 👋 Handlers
nav_order: 50
---

# zip

The `zip` handler creates or extracts ZIP archives.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `src` | no* | - | Source path(s) to compress (when creating archive). Can be string or array. |
| `dest` | no* | - | Destination archive path (when creating) or extraction directory (when extracting). |
| `path` | no* | - | Alternative to `src`/`dest` in some forms. |
| `archive` | no* | - | Archive path for extraction operations. |
| `files` | no* | - | Files to include. |
| `extract` | no | `false` | If true, extract instead of create. |
| `create` | no | `false` | If true, create archive. |
| `remove` | no | `false` | Remove source files after operation where supported. |
| `overwrite` | no | `false` | Overwrite existing files. |
| `owner` | no | - | Ownership metadata. |
| `group` | no | - | Group metadata. |
| `mode` | no | - | Mode bits. |
| `state` | no | `present` | `present` / `absent` / `latest` |

> [!NOTE]
> Usage depends on operation mode (create vs extract).
