---
layout: default
title: 🖥️ CLI
nav_order: 7
has_children: true
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🖥️ CLI
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

## Synopsis

```text
ironstate [flags]                 # run a playbook (dry-run unless --apply)
ironstate <command> [args] [flags]
```

Running `ironstate` without a subcommand **runs a playbook**. That is the main thing ironstate does. Everything else is a helper command:

| Command | Purpose |
| --- | --- |
| [`completion`](./commands/completion.md) | Generate shell completion scripts. |
| [`doctor`](./commands/doctor.md) | Check `PATH` for package managers, shells, script filters, and plugin health. |
| [`filters`](./commands/filters.md) | List built-in and discovered script filters. |
| [`help`](./commands/help.md) | Show help for any command. |
| [`init`](./commands/init.md) | Scaffold a new playbook, optionally from the current system state. |
| [`plugin`](./commands/plugin.md) | Install, update, inspect, test, and benchmark external handler plugins. |
| [`remote`](./commands/remote.md) | Ping, read logs from, and clean up remote-apply targets. |
| [`validate`](./commands/validate.md) | Validate a playbook against the JSON schema. |
| [`version`](./commands/version.md) | Print the version, commit, and build date. |

Every flag for running a playbook is described in [Flags](./flags.md).

## Running a playbook

```shell
# Preview (dry-run) the playbook in the current directory
ironstate

# Preview a specific playbook directory or file
ironstate --playbook path/to/playbook
ironstate --playbook path/to/playbook/main.yml

# Apply the changes
ironstate --playbook path/to/playbook --apply

# Only tasks tagged cli or security
ironstate --playbook path/to/playbook --tags cli,security --apply

# Override variables
ironstate --playbook path/to/playbook --vars-file ./ci-vars.yml --var editor=nvim

# Run on other machines over SSH
ironstate --playbook path/to/playbook --inventory inventory.yml --limit linux --apply
```

### Dry-run vs. apply

Without `--apply`, ironstate gathers facts, evaluates every `when`, and asks each handler whether its task is already satisfied. Then it prints what it *would* do. Nothing changes on the system, with a few deliberate exceptions:

- `fact` tasks always run, because they only record a value.
- Read-only probes run so the preview is accurate: `failed_when` shell checks and environment `valueFrom` shell sources.

In a dry-run, `id`-registered results report `changed` correctly, but `rc`, `stdout`, and `stderr` keep their zero values because no command actually ran.

### Playbook resolution

`--playbook` accepts:

1. **A file**, such as `path/to/main.yml`.
2. **A directory.** ironstate searches it for `site.yml`, `site.yaml`, `main.yml`, then `main.yaml`.
3. **A bare name**, such as `playbooks/camalot`. ironstate also tries a sibling `playbooks/camalot.yml` or `.yaml`.

Without `--playbook`, ironstate searches the current directory. If nothing is found, the error lists every path it tried.

## Output

`--output` selects the result format:

| Format | Description |
| --- | --- |
| `table` (default) | A colored table with a per-handler emoji, a host-facts panel, and a final summary with elapsed time. Changed tasks are highlighted, satisfied tasks are dimmed, and failures are shown in red. |
| `json` | A single JSON array of results on stdout. Progress messages go to stderr, so stdout stays pipeable (`ironstate --output json \| jq`). Each result includes `duration_ms`. |
| `ndjson` | A stream of one JSON event per line: `leaf_start`, `log`, `leaf_result`, `facts`, `summary`, then a final `done` with the exit code. Failures add an `error` event. Every line starts with `{"v":1,`, messages are secret-redacted, and output is never colored. |

Colors are disabled automatically when output isn't a terminal. They're also disabled when `NO_COLOR` or `IRONSTATE_NO_COLOR` is set, or with `--no-color`.

With remote apply, `table` prefixes live lines with `[host]` and prints a result table per host. `json` prints one document with a `hosts` array. `ndjson` tags every event with `"host"`.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success. With remote apply, every host succeeded. |
| `1` | A task failed (`failed_when`, a non-zero `rc`, or a handler error) and stopped the run. With remote apply, at least one host failed. |
| `2` | The playbook, its configuration, or a vars file couldn't be loaded or parsed. |
| `3` | Remote apply only: a host was unreachable or couldn't be bootstrapped. Nothing was applied on that host, so it's safe to retry. |

## Configuration sources

Options are resolved in this order. Earlier sources win.

1. Command-line flags.
2. `IRONSTATE_*` environment variables, such as `IRONSTATE_PLAYBOOK` or `IRONSTATE_TAGS`.
3. An `ironstate.yaml` or `ironstate.yml` config file in the current directory.
4. Built-in defaults.

ironstate also loads `.env` and `.secrets` files from the current directory into the process environment before a run. See [Flags](./flags.md#environment-variables-and-config-file) for details.
