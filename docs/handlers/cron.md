---
layout: default
title: cron
parent: 👋 Handlers
nav_order: 12
---

# cron

The `cron` handler is a cross-platform wrapper for scheduled jobs. It chooses a backend based on platform or explicit `backend` setting.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `name` | no | - | Optional job label/identifier |
| `backend` | no | `auto` | Override: `auto`, `cron`, `cron_unix`, `scheduled_task` |
| `command` | yes* | - | Command to execute (Unix rule). Alias `job` also accepted. |
| `job` | no* | - | Alias for `command`. |
| `schedule` | no | - | Full cron expression such as `*/15 * * * *` |
| `special_time` | no | - | Special alias such as `hourly`, `daily`, `weekly`, `monthly`, `yearly` |
| `minute` | no | `*` | Minute field |
| `hour` | no | `*` | Hour field |
| `day` | no | `*` | Day of month |
| `month` | no | `*` | Month |
| `weekday` | no | `*` | Day of week |
| `user` | no | current user | User to manage under on Unix |
| `env` | no | `false` | Unix backend only: manage environment variable line instead of a command |
| `value` | no | `""` | Unix backend env value (used with `env: true`) |
| `insertbefore` | no | append | Unix backend env placement hint |
| `insertafter` | no | append | Unix backend env placement hint |
| `disabled` | no | `false` | Disable Unix cron entry (comment line) or Windows task |
| `environment` | no | - | Unix backend map of env vars emitted before command |
| `state` | no | `present` | `present` / `absent` / `latest` |

\* `command`/`job` required for Unix rule (unless `env: true` with appropriate settings).

## Platform behavior

- Linux/macOS: Uses Unix cron backend (writes to crontab). When `name` is set, entries are anchored with Ansible-style markers (`#Ansible: <name>`) for idempotent updates/removals. When `env: true`, manages `NAME=value` lines instead of command lines.
- Windows: Delegates to `scheduled_task` backend.

## Examples

```yaml
tasks:
  - name: run backup every 15 minutes
    cron:
      minute: "*/15"
      hour: "*"
      day: "*"
      month: "*"
      weekday: "*"
      command: "/usr/local/bin/backup.sh"
      state: present

  - name: daily cleanup at 2am
    cron:
      special_time: daily
      command: "rm -rf /tmp/ironstate-cache/*"
      state: present
```
