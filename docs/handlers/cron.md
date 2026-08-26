# cron

The `cron` handler is a wrapper for scheduled jobs. It chooses a backend based on platform:

- Linux/macOS: a Unix cron backend (`cron` / `crontab`-style tasks)
- Windows: the existing `scheduled_task` backend

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `name` | no | unset | Optional job label/identifier |
| `backend` | no | `auto` | Override: `auto`, `cron`, `scheduled_task` |
| `command` | yes for Unix rule | unset | Command to execute |
| `job` | no | unset | Alias for `command` |
| `schedule` | no | unset | Full cron expression such as `*/15 * * * *` |
| `special_time` | no | unset | Special alias such as `hourly`, `daily`, `weekly`, `monthly`, `yearly` |
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
| `environment` | no | unset | Unix backend map of env vars emitted before command |
| `state` | no | `present` | `present` / `absent` / `latest` |

## Platform behavior

### Linux/macOS

The Unix backend writes an entry into the current user crontab (or the target `user`'s crontab when specified). It uses the conventional 5-field cron schedule format:

```text
minute hour day month weekday command
```

When `name` is set, the Unix backend anchors the job with an Ansible-style marker (`#Ansible: <name>`) to make updates/removals idempotent by logical job identity.

When `env: true`, the Unix backend manages `NAME=value` lines in the crontab (matching Ansible-style env mode) instead of managing a scheduled command line.

### Windows

On Windows the wrapper defers to `scheduled_task`, preserving the same task semantics for `present`/`absent`/`latest`.

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

  - name: windows task wrapper
    cron:
      backend: scheduled_task
      name: nightly-cleanup
      command: "C:\\tools\\cleanup.ps1"
      state: present
```
