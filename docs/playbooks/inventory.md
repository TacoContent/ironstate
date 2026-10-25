---
render_with_liquid: false
layout: default
title: 🛰️ Inventory
nav_order: 15
parent: 📖 Playbooks
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🛰️ Inventory
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
ironstate can run the same playbook on other machines over SSH. It copies itself to each target, runs there as an agent, and streams the results back. Facts, overlays, `when`, and `become` all behave as if you'd run ironstate on that machine.

```shell
# Ad hoc targets
ironstate --playbook . --apply --target ryan@snoke --target kresh.lan

# From an inventory
ironstate --playbook . --apply --inventory inventory.yml --limit linux --forks 4

# Check connectivity first (applies nothing)
ironstate remote ping --inventory inventory.yml
```

## Inventory files

An inventory names your hosts, how to reach them, and optional per-host variables:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/TacoContent/ironstate/develop/inventory.schema.json
defaults:
  user: ryan
  port: 22
  vars:
    local: { bin_path: ~/.local/bin }

hosts:
  snoke:
    address: 192.168.2.103
    platform: linux
  kresh:
    address: kresh.lan
    port: 2222
    platform: windows
  laptop: {}                   # address defaults to the name (e.g. an ~/.ssh/config alias)
  router:
    address: router.lan
    user: admin
    become: false
    verify_agent: false
    vars:
      local: { bin_path: /home/admin }
  self:
    address: local             # this machine, without SSH

groups:
  linux: [snoke, router]
  windows: [kresh]
```

### Top-level keys

| Key | Description |
| --- | --- |
| `defaults` | Settings applied to every host, unless the host sets them itself. |
| `hosts` | Hosts, keyed by name. Names use letters, digits, `.`, `_`, and `-`, and can't be `all`. |
| `groups` | Named lists of hosts, for use with `--limit`. |

### Host settings

| Key | Description |
| --- | --- |
| `address` | Host name, IP address, SSH config alias, or `local`. Defaults to the host's name. |
| `user` | SSH user. |
| `port` | SSH port. |
| `platform` | Hint for the target OS. `windows` selects the PowerShell bootstrap. |
| `agent_dir` | Where the agent is cached on the target. |
| `become` | `false` turns off every `become` for this host. Tasks then run as the SSH user, for example when there's no sudo. |
| `verify_agent` | `false` skips SHA-256 verification of the agent on the target. |
| `vars` | Variables for this host. These merge on top of the playbook's variables. See [Variable precedence](./facts-vars.md#variable-precedence). |

Inventory `vars` can contain `${{ }}` expressions. Each target evaluates them with its own facts.

### Inventory folders

`--inventory` also accepts a folder. `main.yml` (or `main.yaml`) loads first, then the remaining `.yml` and `.yaml` files alphabetically. Maps merge, later values win, and group lists are appended:

```text
inventory/
├── main.yml          # defaults
├── homelab.yml       # hosts + groups
└── work.yml
```

> [!NOTE]
> An inventory **inside** the playbook folder fails [`ironstate validate`](../cli/commands/validate.md), which checks every YAML file there against the playbook schema. Keep it beside the playbook if you use `validate`.

## Selecting hosts

| Flag | Effect |
| --- | --- |
| `--limit <names>` | Host and group names, comma-separated or repeated. `all` means every host. Without it, every inventory host runs. |
| `--target <host>` | Adds an ad hoc host: `[user@]host[:port]`, an SSH alias, or `local`. Repeatable. |
| `--forks <n>` | How many hosts run at once (default `5`). |

## Requirements

**Controller** (where you run ironstate): an OpenSSH `ssh` client. `ironstate doctor` checks for it.

**Targets**: Linux, macOS, or Windows with OpenSSH Server, on amd64 or arm64 (and ARMv7 Linux), with key-based login.

- Linux and macOS targets need `sh` and a SHA-256 tool (`sha256sum`, `shasum`, `openssl`, or BusyBox).
- Windows targets need PowerShell 7 (`pwsh`) on the SSH session's `PATH`. Keep your PowerShell profile from printing output or prompting.

Authentication is entirely your SSH setup: `~/.ssh/config`, ssh-agent, `ProxyJump`, and `known_hosts`. ironstate always uses `BatchMode=yes` and never disables host key checking. Pass `--ssh-accept-new-host-keys` to accept first-time keys, or `--ssh-config` to use another config file.

## The agent

- When a target has the **same OS and architecture** as the controller, ironstate uploads its own binary.
- Otherwise, release builds **download** the matching release once, verify it against the release checksums (and the cosign signature if `cosign` is installed), and cache it.
- Development builds never download. Pass `--agent-binary os/arch=path` instead (for example, `--agent-binary linux/arm64=./ironstate-linux-arm64`).
- On the target, the agent is cached in `~/.cache/ironstate/agent/` or `%LOCALAPPDATA%\ironstate\agent\`. It's verified before every run and re-uploaded only when it changes.

## What gets sent

The **playbook folder** is bundled and sent to each target, except for:

- `.git`, `.env`, `.secrets`
- anything matched by glob patterns in a `.ironstateignore` file

`--vars-file` files, an outside `filters.dir`, your `filters.*` config, and approved [`uses`](./uses.md) checkouts are sent too. `.env`, `.secrets`, and `--forward-env` values travel **in memory only**.

Expressions run **on the target**, so `facts.*`, `lookup('env', …)`, and `lookup('url', …)` reflect the target.

## Elevation

- **Linux and macOS**: `become` needs passwordless sudo, or `--ask-become-pass`. That flag prompts once, without echo, and sends the password only through the agent's input stream.
- **Windows**: connect with an account whose SSH session is already elevated. `become: true` is a no-op there.
- An inventory host with `become: false` ignores `become` in the playbook.

## Plugins

Playbooks with `plugins:` work remotely. When a target matches the controller's OS and architecture, the controller copies its installed plugins over. Other targets use their own installed plugins, and `--allow-plugin-install` lets them install missing ones (which requires Go on the target). Lockfile checksums are per platform. See [Using a plugin](../plugins/using.md).

## Output and logs

| `--output` | Remote behavior |
| --- | --- |
| `table` | Live `[host]`-prefixed lines, a result table per host, then a summary. |
| `json` | One document with a `hosts` array: status, facts, results, and stats for each host. |
| `ndjson` | Every agent event, tagged with `"host"`, then a final `done` event. |

Each target keeps the event log of its last 20 runs. Use [`ironstate remote logs`](../cli/commands/remote.md#remote-logs) to read one.

## Reliability

- Only **one** run at a time per target. A second concurrent run fails immediately.
- Temporary SSH and bootstrap failures are retried, for up to 3 attempts in total. Use `--host-timeout` to cap each host.
- **Ctrl-C** asks each agent to stop after its current task. A second Ctrl-C aborts.
- `--remote-detach --apply` starts the run in the background. Collect the results later with `remote logs`.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Every host succeeded. |
| `1` | At least one host failed. |
| `2` | A controller-side load or config error. |
| `3` | At least one host was unreachable or couldn't be bootstrapped. Nothing was applied there, so it's safe to retry. |
