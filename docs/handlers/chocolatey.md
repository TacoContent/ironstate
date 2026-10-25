---
layout: default
title: chocolatey
parent: 👋 Handlers
nav_order: 8
---

# chocolatey

The `chocolatey` handler manages packages via Chocolatey.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `package` / `name` | no | - | Package name as known to Chocolatey. Can be string or array. |
| `state` | no | `present` | `present` / `absent` / `latest` |

Windows only (typically).
