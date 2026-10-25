---
layout: default
title: firewall
parent: 👋 Handlers
nav_order: 16
---

# firewall

The `firewall` handler manages firewall rules (cross-platform wrapper).

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `name` | no | - | Rule name/identifier. |
| `state` | no | `present` | `present` / `absent` / `latest` |

Platform-specific backends: uses `ufw` on Linux, `iptables` on some Linux systems, `advfirewall`/Windows Firewall on Windows. See related handler docs.
