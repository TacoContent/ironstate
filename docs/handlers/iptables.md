---
layout: default
title: iptables
parent: 👋 Handlers
nav_order: 24
---

# iptables

The `iptables` handler manages iptables rules on Linux.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `table` | no | - | iptables table. |
| `chain` | no | - | iptables chain. |
| `rule` / `rulespec` | no | - | Rule specification. |
| `state` | no | `present` | `present` / `absent` / `latest` |
