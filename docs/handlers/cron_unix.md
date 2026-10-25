---
layout: default
title: cron_unix
parent: 👋 Handlers
nav_order: 11
---

# cron_unix

The `cron_unix` handler manages Unix crontab entries directly.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `name` | no | - | Job identifier for marker-based management. |
| `command` / `job` | no | - | Command to execute. |
| `minute` | no | `*` | Minute field |
| `hour` | no | `*` | Hour field |
| `day` | no | `*` | Day of month |
| `month` | no | `*` | Month |
| `weekday` | no | `*` | Day of week |
| `special_time` | no | - | Special alias such as `reboot`, `yearly`, `annually`, `monthly`, `weekly`, `daily`, `hourly`. |
| `user` | no | current user | User whose crontab to manage. |
| `env` | no | `false` | Manage environment variable line instead of a command. |
| `value` | no | - | Value for env var when `env: true`. |
| `disabled` | no | `false` | Disable entry (comment line). |
| `environment` | no | - | Map of env vars emitted before command. |
| `state` | no | `present` | `present` / `absent` / `latest` |
