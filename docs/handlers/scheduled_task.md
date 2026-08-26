# scheduled_task

The `scheduled_task` handler registers, updates, and removes Windows Task Scheduler tasks by generating Task Scheduler XML and invoking the built-in `schtasks.exe` command.

This is the Windows backend used by the cross-platform `cron` wrapper, and it supports a rich set of Task Scheduler shapes so it can model real scheduled jobs rather than a minimal subset.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `name` | yes | - | Task name |
| `path` | no | `\` | Task folder, e.g. `\MyApps\`. Always normalized to start/end with `\` |
| `description` | no | unset | Human-readable task description |
| `enabled` | no | `true` | Whether the task is enabled after registration |
| `actions` | yes unless absent | unset | One or more action blocks to run, in order |
| `triggers` | no | unset | Trigger list; omit for a manual/on-demand-only task |
| `principal` | no | current user | Account and privilege settings for the task |
| `settings` | no | task defaults | Task-level behavior settings |
| `owner` | no | unset | Alias for `principal.user_id` when `principal.user_id` is omitted |
| `group` | no | unset | Alias for `principal.group_id` when `principal.group_id` is omitted |
| `state` | no | `present` | `present` / `absent` / `latest` |

## Actions

Each entry in `actions` supports:

| Field | Required | Description |
| --- | --- | --- |
| `execute` | yes | Path to the executable or script to run |
| `arguments` | no | Arguments passed to the executable |
| `working_directory` | no | Working directory for the action |

## Triggers

Each trigger entry has a `type` and then type-specific fields.

| `type` | Supported fields |
| --- | --- |
| `logon` | `user_id?`, `delay?`, `random_delay?` |
| `startup` | `delay?`, `random_delay?` |
| `once` | `at`, `repetition_interval?`, `repetition_duration?`, `random_delay?` |
| `daily` | `at`, `days_interval?` (default `1`), `repetition_interval?`, `repetition_duration?`, `random_delay?` |
| `weekly` | `at`, `days_of_week` (required list), `weeks_interval?` (default `1`), `repetition_interval?`, `repetition_duration?`, `random_delay?` |

Duration fields accept either:

- ISO 8601 durations such as `PT30S`, `PT1H`, `P1D`
- .NET TimeSpan strings such as `00:00:30` or `1.00:00:00`

## Principal configuration

`principal` may include:

| Field | Description |
| --- | --- |
| `user_id` | Username to run as, such as `SYSTEM` or `NT AUTHORITY\SYSTEM`. Mutually exclusive with `group_id` |
| `group_id` | Group to run as instead of a single user |
| `logon_type` | `None`, `Password`, `S4U`, `Interactive`, `Group`, `ServiceAccount`, or `InteractiveOrPassword` |
| `run_level` | `Limited` (default) or `Highest` |
| `password_env` | Environment variable name containing the password when `logon_type: Password` |

A `Password` login type requires both `user_id` and `password_env`. The password is never written into YAML.

## Settings

These are partial declarations: only the keys you list are managed or compared. Any omitted keys remain at Task Scheduler defaults.

| Field | Description |
| --- | --- |
| `disallow_start_if_on_batteries` | Do not start if the machine is on battery |
| `start_when_available` | Start as soon as possible after a missed schedule |
| `hidden` | Hide the task in the scheduler UI |
| `wake_to_run` | Wake the machine from sleep to run it |
| `allow_hard_terminate` | Allow hard termination |
| `run_only_if_network_available` | Only run when a network is available |
| `run_only_if_idle` | Only run when the machine is idle |
| `multiple_instances` | `IgnoreNew`, `Parallel`, `Queue`, or `StopExisting` |
| `restart_count` | Number of retries after failure |
| `execution_time_limit` | Maximum runtime before the task is killed; `PT0S` means no limit |
| `restart_interval` | Delay between retries when `restart_count` is set |
| `delete_expired_task_after` | Auto-delete the task after its last trigger expires |

## State behavior

- `present`: ensures the task exists.
- `latest`: same as `present`, but forces re-registration when the task is known to be stale or has drifted.
- `absent`: removes the task.

The handler intentionally does not do deep field-by-field diffs for the XML-backed Task Scheduler implementation: it checks task existence and enabled state, then re-registers if the task needs to be refreshed. This is the practical trade-off with `schtasks.exe` and the XML definition approach.

## Platform notes

- `logon` and `startup` triggers may require an elevated session.
- `once`, `daily`, `weekly`, and manual-only tasks can usually be created without admin rights.
- The task `path` is normalized to always have a leading and trailing backslash.

## Example

```yaml
tasks:
  - name: kanata autostart
    scheduled_task:
      name: kanata
      description: Starts kanata at logon
      actions:
        - execute: ~/.local/bin/kanata-run.bat
      triggers:
        - type: logon
      principal:
        run_level: Highest
      state: present
```
