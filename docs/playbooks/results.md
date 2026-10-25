---
render_with_liquid: false
layout: default
title: 📋 Results and Failures
nav_order: 9
parent: 📖 Playbooks
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 📋 Results and Failures
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
## Registering results with `id`

Give a leaf an `id` to save its result. Later tasks can then read it by that name, either bare in `when` or inside `${{ }}`:

```yaml
- name: Is the tool installed?
  id: tool_check
  shell:
    command: command -v my-tool
    type: bash
  continue_on_error: true

- name: Install the tool
  when: tool_check.rc != 0
  shell: curl -fsSL https://example.com/install.sh | bash
```

| Field | Description |
| --- | --- |
| `changed` | Whether the leaf needed a change (install or uninstall). |
| `failed` | Whether the leaf counted as failed. |
| `rc` | Exit code of the command (`0` for modules that don't run a command, unless they error). |
| `stdout`, `stdout_lines` | Standard output, as one string and as a list of lines. |
| `stderr`, `stderr_lines` | Standard error, the same way. |
| `results` | Looped tasks only: every iteration's result, in order. |

> [!NOTE]
> In a **dry-run**, nothing is executed, so `rc`, `stdout`, and `stderr` keep empty defaults. `changed` is still predicted accurately.

### Rich PowerShell results

When a `shell` task with `host: pwsh` outputs exactly **one object**, that object's properties are added to the result:

```yaml
- id: pf
  shell:
    command: Get-ItemProperty HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion

- log: { message: "${{ pf.ProgramFilesDir }}" }
```

### `id` vs. `fact`

`fact` tasks run before everything else, so **a fact can't read an `id` result**. To turn a command's output into a fact, give the fact its own `shell`. See [Custom facts](./facts-vars#custom-facts).

## `failed_when`

By default, a leaf fails when its handler reports a non-zero `rc` or an error. `failed_when` replaces that rule. It takes one expression, or a list of entries that must **all** be true. Each entry can reference the leaf's own `rc`, `stdout`, `stdout_lines`, `stderr`, `stderr_lines`, and `changed`, without an `id`:

```yaml
- name: Health check
  shell: C:\tools\healthcheck.exe
  failed_when: rc != 0 or 'ERROR' in stdout

- name: grep that may find nothing
  shell: grep -q pattern file.txt
  failed_when: rc > 1           # rc 1 just means "no match"

- name: Never fail
  shell: ./best-effort.sh
  failed_when: false
```

Expressions may be bare or wrapped in `${{ }}`. A list entry can also be a **shell check**: a read-only command that runs after the task, with `value` evaluated against **its** result:

```yaml
- name: Install my_command
  shell: ./install-my-command.sh
  failed_when:
    - ${{ rc != 0 }}
    - shell:
        command: '! command -v my_command >/dev/null'
        type: bash
      value: ${{ rc == 0 }}
```

Shell checks also run in a dry-run, so keep them read-only.

## `continue_on_error`

A failed leaf normally **stops the run**. With `continue_on_error: true`, the failure is reported and the run continues. The result, including any `id`, still shows `failed: true`:

```yaml
- name: Best-effort cleanup
  continue_on_error: true
  shell: Remove-Item C:\maybe\missing.tmp -ErrorAction Stop
```

## `become`

`become` runs a leaf's command elevated:

| Value | Effect |
| --- | --- |
| `true` | Run with `sudo` (or Windows 11's built-in `sudo.exe`). |
| `'<user>'` | Run as that user with `sudo -u <user>`. On Windows this elevates to Administrator. |
| `false` / omitted | Run as the current user (default). |

```yaml
- name: System packages
  become: true
  apt: { package: [git, curl, ripgrep] }

- name: As the deploy user
  become: deploy
  shell: whoami
```

- `become` is **per leaf**; it doesn't cascade from groups.
- It applies to modules that run external commands: package managers, `shell`, `git`, firewall and cron modules, and plugins. Modules that change the system directly (`file`, `copy`, `registry`, `user`, `group`, …) ignore it; run ironstate itself elevated if they need privileges.
- If `sudo` isn't available, the leaf fails. It never silently runs unelevated.
- `sudo` may prompt for a password. For remote runs, use passwordless sudo or `--ask-become-pass`. See [Inventory](./inventory#elevation).
- Tasks from an isolated [`uses`](./uses#isolation) source can't use `become`.
