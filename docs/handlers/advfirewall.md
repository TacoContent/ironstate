---
layout: default
title: advfirewall
parent: 👋 Handlers
nav_order: 1
---

# advfirewall

The `advfirewall` handler manages Windows Advanced Firewall rules.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `name` | no | - | Rule name. |
| `state` | no | `present` | `present` / `absent` / `latest` |

Windows only.
