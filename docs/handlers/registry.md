---
layout: default
title: registry
parent: 👋 Handlers
nav_order: 33
---

# registry

The `registry` handler manages Windows Registry entries.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `key` | yes | - | Registry key path. |
| `name` / `value_name` | no | - | Value name. |
| `value` / `data` | no | - | Value data. |
| `type` | no | - | Registry value type (e.g., `REG_SZ`, `REG_DWORD`, `REG_BINARY`). |
| `state` | no | `present` | `present` / `absent` / `latest` |

> [!NOTE]
> This handler is forWindows only.
