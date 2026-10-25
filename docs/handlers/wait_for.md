---
layout: default
title: wait_for
parent: 👋 Handlers
nav_order: 46
---

# wait_for

The `wait_for` handler waits for conditions.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `host` | no | - | Host to wait for. |
| `port` | no | - | Port to wait for. |
| `path` | no | - | Path to wait for. |
| `delay` | no | - | Delay before checking. |
| `timeout` | no | - | Timeout for waiting. |
| `state` | no | `present` | `present` / `absent` / `latest` |
