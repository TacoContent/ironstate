---
layout: default
title: service
parent: 👋 Handlers
nav_order: 36
---

# service

The `service` handler manages system services.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `name` | yes | - | Service name. |
| `enabled` | no | - | Whether service should be enabled. |
| `state` | no | `present` | `present` / `absent` / `latest` / `started` / `stopped` / `restarted` |
