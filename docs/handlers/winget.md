---
layout: default
title: winget
parent: 👋 Handlers
nav_order: 47
---

# winget

The `winget` handler manages packages via Windows Package Manager (winget). Matched by exact package ID.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `package` | yes | - | winget package ID, matched with `--exact`. |
| `source` | no | - | winget source to install from, e.g. `winget` or `msstore`. |
| `state` | no | `present` | `present` / `absent` / `latest` |

> [!NOTE]
> This handler is for Windows only.
