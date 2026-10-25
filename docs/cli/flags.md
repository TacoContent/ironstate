---
layout: default
title: 🏷️ Flags
nav_order: 2
parent: 🖥️ CLI
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🏷️ Flags
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

These flags apply when **running a playbook** (`ironstate [flags]`). Subcommands have their own flags, documented on each [command page](./commands/index.md).

## Playbook selection and variables

| Flag | Default | Description |
| --- | --- | --- |
| `--playbook <path>` | current directory | Playbook file, directory, or bare name. A directory is searched for `site.yml`, `site.yaml`, `main.yml`, then `main.yaml`. A bare name also tries a sibling `<name>.yml` or `<name>.yaml`. |
| `--vars-file <path>` | | Extra vars document to merge on top of the playbook's own `hosts/` and `variables/` hierarchy. Repeatable; later files win on overlapping keys. |
| `--var <key=value>` | | Override one variable by dotted key path, such as `--var ssh.port=2222`. Repeatable. This has the highest precedence of all. |

```shell
ironstate --playbook main.yml --vars-file ./ci-vars.yml --vars-file ./local.yml
ironstate --playbook main.yml --var editor=nvim --var ssh.port=2222
```

The full variable precedence order is described in [Facts & Variables](../playbooks/facts-vars.md#variable-precedence).

## Execution

| Flag | Default | Description |
| --- | --- | --- |
| `--apply` | `false` | Actually make changes. Without it, every run is a dry-run. |
| `--tags <a,b,...>` | | Only run tasks carrying any of these tags (comma-separated). |
| `--tag <name>` | | Same as `--tags`, one tag per flag. Repeatable, and combines with `--tags`. |
| `--allow-plugin-install` | `false` | Let ironstate install playbook-declared plugins that aren't installed yet. Requires Go on `PATH`. Use only in trusted automation. |
| `--allow-remote-uses` | `false` | Pre-approve every non-isolated remote `uses:` source instead of prompting. Non-interactive runs decline untrusted sources unless this flag is set. |

```shell
ironstate --tags cli,security --tag editors --apply
```

Untagged tasks always run, even when tags are selected. See [Tags](../playbooks/tags.md).

## Output

| Flag | Default | Description |
| --- | --- | --- |
| `--output <format>` | `table` | `table`, `json`, or `ndjson`. See [Output](./index.md#output). |
| `-v`, `--verbose` | `false` | Also print a line for every already-satisfied or skipped task. |
| `--no-color` | `false` | Disable colored output. `NO_COLOR` and `IRONSTATE_NO_COLOR` do the same. |

## Remote apply

These flags run the playbook on other machines over SSH. They are shared with [`ironstate remote`](./commands/remote.md). See [Inventory files](../playbooks/inventory.md) for the inventory format and how remote apply works.

| Flag | Default | Description |
| --- | --- | --- |
| `--target <host>` | | Apply to a host over SSH: `[user@]host[:port]` or an `~/.ssh/config` alias. `local` runs the agent on this machine without SSH. Repeatable. |
| `--inventory <path>` | | Inventory file, or a directory of inventory fragments. |
| `--limit <hosts>` | every host | With `--inventory`: only these hosts or groups. Comma-separated or repeatable; `all` selects every host. |
| `--forks <n>` | `5` | How many hosts to run at once. |
| `--host-timeout <duration>` | `0` (disabled) | Maximum time for one host operation, such as `5m`. |
| `--agent-binary <os/arch=path>` | | ironstate binary to ship for a target platform, such as `linux/arm64=./dist/ironstate`. Repeatable. Required for development builds that target a different OS or architecture. |
| `--no-agent-download` | `false` | Never download a release agent binary. Use `--agent-binary` or the local agent cache instead. |
| `--remote-agent-dir <dir>` | `~/.cache/ironstate/agent` (POSIX), `%LOCALAPPDATA%\ironstate\agent` (Windows) | Directory on targets for the cached agent binary. The inventory `agent_dir` overrides this per host. |
| `--ssh-config <file>` | | SSH config file, passed to `ssh -F`. |
| `--ssh-accept-new-host-keys` | `false` | Accept and remember host keys of never-seen targets (`StrictHostKeyChecking=accept-new`). Host-key checking is never disabled. |
| `--skip-agent-verification` | `false` | Skip target-side SHA-256 verification for trusted targets that have no hash utility. The agent is re-uploaded on every run. |
| `--forward-env <NAME>` | | Forward this controller environment variable to the targets. Repeatable. Values travel in memory and are never written to the target's disk. |
| `--ask-become-pass` | `false` | Prompt once (without echo) for the remote `sudo` password. The password is sent only over the agent's stdin channel. |
| `--remote-detach` | `false` | With `--apply`: start remote agents in the background. Collect results later with `ironstate remote logs`. Can't be combined with `.env`/`.secrets` values, `--forward-env`, or `--ask-become-pass`. |

```shell
ironstate --playbook . --apply --target rconr@snoke --target kresh.lan
ironstate --playbook . --apply --inventory inventory.yml --limit linux --forks 4 --host-timeout 10m
ironstate --playbook . --apply --target pi --agent-binary linux/armv7=./ironstate-linux-armv7
```

## Global

| Flag | Description |
| --- | --- |
| `-h`, `--help` | Show help for `ironstate` or any subcommand. |

## Environment variables and config file

Every playbook-run option can also come from an environment variable or a config file. Command-line flags always win.

### `IRONSTATE_*` environment variables

Uppercase the flag name, replace `-` with `_`, and prefix it with `IRONSTATE_`:

| Variable | Equivalent flag |
| --- | --- |
| `IRONSTATE_PLAYBOOK` | `--playbook` |
| `IRONSTATE_APPLY` | `--apply` |
| `IRONSTATE_TAGS` | `--tags` (comma-separated) |
| `IRONSTATE_OUTPUT` | `--output` |
| `IRONSTATE_VERBOSE` | `--verbose` |
| `IRONSTATE_ALLOW_REMOTE_USES` | `--allow-remote-uses` |
| `IRONSTATE_NO_COLOR` | `--no-color` |

`--var` and `--vars-file` are read only from the command line.

### `ironstate.yaml`

An optional `ironstate.yaml` (or `ironstate.yml`) in the **current working directory** provides defaults:

```yaml
playbook: playbooks/camalot
output: table
tags: [cli, editors]
verbose: false

filters:
  dir: filters            # where script filters are discovered, relative to the playbook (default: filters)
  interpreters:           # extension -> interpreter argv used to run that extension's shim
    .ps1: [powershell.exe, -NoProfile, -File]
    .sh: [/usr/local/bin/bash]
```

`filters.dir` and `filters.interpreters` exist only in the config file. Setting `filters.interpreters` **replaces** the default map, so list every extension you want discovered. See [Custom filters](../filters/custom.md#choosing-interpreters).

### `.env` and `.secrets`

Before a run, ironstate reads `KEY=VALUE` lines from `.env`, then from `.secrets`, in the **current working directory**, and adds them to the process environment. Either file is optional.

- Blank lines and lines starting with `#` are ignored.
- A value wrapped in matching `'...'` or `"..."` has the quotes stripped.
- Every value from `.secrets` is registered as a secret and is redacted as `***` in all output. See [Secrets](../playbooks/secrets.md).

These values are visible to `lookup('env', 'KEY')`, the `env` filter, `envs.KEY`, and any process ironstate starts. With remote apply, they're forwarded in memory to each target.
