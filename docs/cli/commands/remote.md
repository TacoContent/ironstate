---
layout: default
title: ☎️ Remote
nav_order: 7
parent: 🖥️ Commands
grand_parent: 🖥️ CLI
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# ☎️ Remote
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
Helper commands for [remote apply](../../playbooks/inventory.md). Running the playbook remotely uses the main command with `--target` or `--inventory`. These subcommands check targets and maintain them.

## Usage

```shell
ironstate remote ping  [remote flags]
ironstate remote logs  <host> [run_id] [remote flags]
ironstate remote clean [remote flags]
```

## `remote ping`

Connects to each selected host, probes it (platform, `sudo` availability), and makes sure the ironstate agent is present and runs. The agent is uploaded or downloaded if needed. **Nothing is applied.**

```text
$ ironstate remote ping --inventory inventory.yml
[ok]   snoke (192.168.2.103)  linux/amd64  sudo=passwordless  agent: v1.4.0
[ok]   kresh (192.168.2.10)  windows/amd64  agent: v1.4.0 (uploaded)
[unreachable] laptop: ssh: connect to host laptop port 22: Connection timed out
```

Use it to check SSH access, host keys, and agent bootstrap before your first `--apply`.

## `remote logs`

```shell
ironstate remote logs <host> [run_id] --inventory inventory.yml
```

Prints the raw NDJSON event log of a host's most recent run. If you pass `run_id`, it prints that run's log instead. Every remote run writes its events to the target's cache (`runs/<run_id>/events.ndjson`), and the last 20 runs are kept. This is how you collect results from a [detached](../flags.md#remote-apply) (`--remote-detach`) run.

`<host>` is an inventory host name when you pass `--inventory`. Otherwise it's a target string, such as `user@host`. It must select exactly one host.

## `remote clean`

```shell
ironstate remote clean --inventory inventory.yml [--limit linux]
```

Removes the cached agent binaries and retained run logs from each selected host. It refuses to clean a host that's in the middle of a run.

## Flags

All three subcommands accept the shared remote flags:

| Flag | Default | Description |
| --- | --- | --- |
| `--target <host>` | | `[user@]host[:port]`, an ssh config alias, or `local`. Repeatable. |
| `--inventory <path>` | | Inventory file or directory. |
| `--limit <hosts>` | every host | Hosts or groups to select (`all` = every host). |
| `--forks <n>` | `5` | Hosts to process at once. |
| `--host-timeout <duration>` | `0` | Maximum time for one host operation. |
| `--agent-binary <os/arch=path>` | | Agent binary for a target platform. Repeatable. |
| `--no-agent-download` | `false` | Never download a release agent. |
| `--remote-agent-dir <dir>` | | Agent cache directory on targets. |
| `--ssh-config <file>` | | Passed to `ssh -F`. |
| `--ssh-accept-new-host-keys` | `false` | Use `StrictHostKeyChecking=accept-new`. |
| `--skip-agent-verification` | `false` | Skip target-side SHA-256 verification. |

See [Flags › Remote apply](../flags.md#remote-apply) for full descriptions.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Every host succeeded. |
| `1` | At least one host returned an error. |
| `2` | The inventory or flags couldn't be loaded. |
| `3` | At least one host was unreachable or couldn't be bootstrapped. |
| `130` | `remote ping` was interrupted with Ctrl-C. |
