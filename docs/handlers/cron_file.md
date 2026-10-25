---
layout: default
title: cron_file
parent: 👋 Handlers
nav_order: 10
---

# cron_file

The `cron_file` handler manages a system cron file entry in cron.d style.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `cron_file` | yes | - | Absolute file path, or filename relative to `/etc/cron.d/`. |
| `name` | no | - | Optional identifier/label. |
| `minute` | no | `*` | Minute field |
| `hour` | no | `*` | Hour field |
| `day` | no | `*` | Day of month |
| `month` | no | `*` | Month |
| `weekday` | no | `*` | Day of week |
| `job` / `command` | no | - | Command to execute. |
| `special_time` | no | - | Special alias such as `reboot`, `yearly`, `annually`, `monthly`, `weekly`, `daily`, `hourly`. |
| `user` | no | `root` | Target user for command entries in the cron file. |
| `env` | no | `false` | Manage environment variable line instead of a command. |
| `value` | no | - | Value for env var when `env: true`. |
| `disabled` | no | `false` | Disable entry. |
| `owner` | no | - | Optional file owner (username or numeric uid). |
| `group` | no | - | Optional file group (group name or numeric gid). |
| `mode` | no | - | Optional file mode (e.g. `0644` or `420`). |
| `state` | no | `present` | `present` / `absent` / `latest` |
