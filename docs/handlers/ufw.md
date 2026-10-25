---
layout: default
title: ufw
parent: 👋 Handlers
nav_order: 43
---

# ufw

The `ufw` handler manages UFW (Uncomplicated Firewall) on Linux.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `rule` | no | - | UFW rule. |
| `state` | no | `present` | `present` / `absent` / `latest` |
