# cron_unix

The `cron_unix` handler manages Unix cron jobs using the local `crontab` command. It is the Linux/macOS backend used by the higher-level `cron` wrapper and is intentionally modelled after the common cron/task semantics users expect from Ansible-style job scheduling.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `name` | no | unset | Optional label for the job |
| `command` | yes (unless `env: true`) | unset | Command string to execute |
| `job` | no | unset | Alias for `command` |
| `schedule` | no | composed from fields | Full cron expression, e.g. `*/15 * * * *` |
| `special_time` | no | unset | Alias like `hourly`, `daily`, `weekly`, `monthly`, `yearly`, `reboot` |
| `minute` | no | `*` | Cron minute field |
| `hour` | no | `*` | Cron hour field |
| `day` | no | `*` | Cron day-of-month field |
| `month` | no | `*` | Cron month field |
| `weekday` | no | `*` | Cron day-of-week field |
| `user` | no | current user | Target user whose crontab is managed |
| `env` | no | `false` | When true, manages a `NAME=value` environment line instead of a command |
| `value` | no | `""` | Value for env mode (`name` becomes the env var key) |
| `insertbefore` | no | append | Insert env variable before this existing env var name |
| `insertafter` | no | append | Insert env variable after this existing env var name |
| `disabled` | no | `false` | Prefixes command entries with `#` (job mode only) |
| `environment` | no | unset | Map of env lines to place before the command entry |
| `state` | no | `present` | `present` / `absent` / `latest` |

## Schedule generation

A cron entry is produced as:

```text
<minute> <hour> <day> <month> <weekday> <command>
```

If you specify `schedule`, it is used verbatim. Otherwise the handler composes a schedule from the individual cron fields or from a `special_time` alias.

Supported `special_time` values:

- `hourly` → `0 * * * *`
- `daily` / `midnight` → `0 0 * * *`
- `weekly` → `0 0 * * 0`
- `monthly` → `0 0 1 * *`
- `yearly` / `annually` → `0 0 1 1 *`
- `reboot` → `@reboot`

## Named jobs and idempotence markers

When `name` is set in job mode, the handler writes an Ansible-compatible marker line:

```text
#Ansible: <name>
```

The marker is used as the primary idempotence anchor for updates/removals, which keeps behavior closer to Ansible's cron module and allows schedule/command changes without duplicate entries.

## Env mode

When `env: true`, the handler manages an environment line in the crontab instead of a scheduled command:

```text
<name>=<value>
```

In this mode:

- `name` is required and treated as the env variable key.
- `command`/`job` are ignored.
- `insertbefore` / `insertafter` control placement by env var name when possible.

## State behavior

- `present`: ensures the matching cron entry exists.
- `latest`: behaves like `present`, but is intentionally treated as a convergence point when the schedule is declared to be a desired state.
- `absent`: removes the matching cron entry.

## Notes

- The job is managed in the target user's crontab via `crontab -l` / `crontab -`.
- If `user` is omitted, the current user’s crontab is used.
- This handler is not intended to support every advanced cron feature of a full `/etc/crontab`; it is focused on the common per-user job model used by developers and automation.

## Examples

```yaml
tasks:
  - name: backup every 15 minutes
    cron_unix:
      name: nightly-backup
      minute: "*/15"
      hour: "*"
      day: "*"
      month: "*"
      weekday: "*"
      command: "/usr/local/bin/backup.sh"
      state: present

  - name: cleanup daily at 02:00
    cron_unix:
      special_time: daily
      command: "rm -rf /tmp/ironstate-cache/*"
      disabled: false
      state: present

  - name: weekly report for root
    cron_unix:
      user: root
      special_time: weekly
      command: "/opt/reports/run-weekly.sh"
      state: present

  - name: ensure MAILTO env for cron
    cron_unix:
      env: true
      name: MAILTO
      value: ops@example.com
      insertafter: PATH
      state: present
```
