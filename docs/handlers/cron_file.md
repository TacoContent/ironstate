# cron_file

The `cron_file` handler manages system cron entries in cron-file style locations (for example `/etc/cron.d/*`), similar to Ansible's `cron_file` usage.

It is intended for host-level jobs that should be managed as files rather than per-user `crontab` entries.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `cron_file` | yes | unset | Absolute path, or filename relative to `/etc/cron.d` |
| `name` | no | unset | Logical marker name (`#Ansible: <name>`) for idempotence |
| `command` | yes (unless `env: true`) | unset | Command to execute |
| `job` | no | unset | Alias for `command` |
| `schedule` | no | composed from fields | Full cron expression, e.g. `*/15 * * * *` |
| `special_time` | no | unset | Alias like `hourly`, `daily`, `weekly`, `monthly`, `yearly`, `reboot` |
| `minute` | no | `*` | Cron minute field |
| `hour` | no | `*` | Cron hour field |
| `day` | no | `*` | Cron day-of-month field |
| `month` | no | `*` | Cron month field |
| `weekday` | no | `*` | Cron day-of-week field |
| `user` | no | `root` | User field in command entries (`<schedule> <user> <command>`) |
| `owner` | no | unset | File owner for the cron file (username or uid) |
| `group` | no | unset | File group for the cron file (group name or gid) |
| `mode` | no | unset | File mode for the cron file, e.g. `"0644"` |
| `env` | no | `false` | Manage an env variable line (`NAME=value`) instead of a command |
| `value` | no | `""` | Value for env mode |
| `insertbefore` | no | append | Insert env variable before this variable name |
| `insertafter` | no | append | Insert env variable after this variable name |
| `disabled` | no | `false` | Prefix command line with `#` |
| `environment` | no | unset | Environment map emitted before command line |
| `state` | no | `present` | `present` / `absent` / `latest` |

## Entry format

For command entries, the handler writes system cron shape:

```text
<minute> <hour> <day> <month> <weekday> <user> <command>
```

For `special_time` aliases, it writes:

```text
@reboot <user> <command>
```

## Idempotence behavior

- With `name`, entries are anchored using `#Ansible: <name>` and updated in place.
- With `env: true`, entries are matched by env var key (`name`) and updated by key.
- Without `name`, command entries are matched by exact rendered line.

## File metadata hardening

For stricter `/etc/cron.d` operations you can optionally enforce metadata on the cron file:

- `owner`: username or numeric uid
- `group`: group name or numeric gid
- `mode`: file mode (`"0644"`, `"0600"`, or numeric)

When these are set, the handler writes content and then applies the requested metadata.

## Examples

```yaml
tasks:
  - name: nightly system backup
    cron_file:
      cron_file: backups
      name: nightly-backup
      minute: "0"
      hour: "2"
      user: root
      command: "/usr/local/sbin/backup-nightly.sh"
      state: present

  - name: set MAILTO for backup cron file
    cron_file:
      cron_file: backups
      env: true
      name: MAILTO
      value: ops@example.com
      insertbefore: PATH
      state: present

  - name: remove legacy cron file entry
    cron_file:
      cron_file: /etc/cron.d/legacy-jobs
      name: old-cleanup
      state: absent

  - name: enforce strict cron file metadata
    cron_file:
      cron_file: backups
      name: nightly-backup
      minute: "0"
      hour: "2"
      user: root
      command: "/usr/local/sbin/backup-nightly.sh"
      owner: root
      group: root
      mode: "0644"
      state: present
```
