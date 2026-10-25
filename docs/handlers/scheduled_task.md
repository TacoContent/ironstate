---
layout: default
title: scheduled_task
parent: 👋 Handlers
nav_order: 34
---

# scheduled_task

The `scheduled_task` handler manages Windows Scheduled Tasks.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `name` | yes | - | Task name. |
| `action` / `actions` | no | - | Task action(s). |
| `trigger` / `triggers` | no | - | Task trigger(s). |
| `principal` | no | - | Task principal/run-as configuration. |
| `settings` | no | - | Task settings. |
| `state` | no | `present` | `present` / `absent` / `latest` |

> [!NOTE]
> This handler is for Windows only.
