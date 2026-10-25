---
layout: default
title: 🧩 Plugin
nav_order: 6
parent: 🖥️ Commands
grand_parent: 🖥️ CLI
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🧩 Plugin
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
Manages external handler [plugins](../../plugins/).

## Usage

```shell
ironstate plugin <subcommand> [args] [flags]
```

A plugin reference is `<organization>.<plugin>`, optionally followed by `@<version>`. The reference maps to the Go module `github.com/<organization>/ironstate-handler-<plugin>`. For example, `acme.hosts@v1.2.3` maps to `github.com/acme/ironstate-handler-hosts@v1.2.3`.

Installed plugins live in the per-user cache: `<user cache dir>/ironstate/plugins/<org>/<plugin>/<version>/`. That's `~/.cache` on Linux, `~/Library/Caches` on macOS, and `%LocalAppData%` on Windows.

| Subcommand | Purpose |
| --- | --- |
| [`install`](#install) | Install a plugin. |
| [`list`](#list) | List installed plugins. |
| [`info`](#info) | Show an installed plugin's manifest. |
| [`update`](#update) | Install a newer version of one or all plugins. |
| [`uninstall`](#uninstall) | Remove one or all versions of a plugin. |
| [`test`](#test) | Run a handler's operations without a playbook. |
| [`bench`](#bench) | Benchmark a handler operation. |

## `install`

```shell
ironstate plugin install <organization.plugin>[@version]
```

Resolves the version (`latest` when omitted) and builds the plugin with `go install`. It then launches the binary to validate the handshake and list its handlers, and copies the binary and a `manifest.json` into the plugin store.

> [!NOTE]
> Installing requires the Go toolchain on `PATH`, plus network access to the module proxy or the repository.

```text
$ ironstate plugin install acme.hosts@v1.2.3
installed acme.hosts@v1.2.3
```

## `list`

```shell
ironstate plugin list
```

```text
PLUGIN      VERSION  HANDLERS  SUMMARY
acme.hosts  v1.2.3   entry     Manage hosts-file entries
```

## `info`

```shell
ironstate plugin info <organization.plugin>
```

Prints every installed version's manifest: source module, checksum, license, handlers, and summary.

```text
acme.hosts@v1.2.3
  source: github.com/acme/ironstate-handler-hosts
  checksum: sha256:...
  license: MIT
  handlers: entry
  summary: Manage hosts-file entries
```

## `update`

```shell
ironstate plugin update <organization.plugin>[@version] [--playbook <path>]
ironstate plugin update --all [--playbook <path>]
ironstate plugin update --lock-file-only [<organization.plugin>[@version]] [--playbook <path>]
```

Installs the given version, or `latest`, side by side with existing versions. Pass exactly one plugin **or** `--all`.

After installing, `update` writes `ironstate.lock.yaml` beside the playbook, pinning the version and checksum of every plugin the playbook declares under `plugins:`:

- A plugin updated by this command is pinned to its new version.
- Other declared plugins keep their existing lock entry, or are pinned at their declared version if they have none.
- Plugins the playbook doesn't declare aren't written.

If there's no playbook in the current directory and you didn't pass `--playbook`, `update` only installs and says the lockfile wasn't written.

With `--lock-file-only`, nothing is installed. `update` pins the named plugin (at `@version`, or its declared version), or every declared plugin when none is named, using what is already installed. Use it to create a lockfile for a working setup, or to re-pin after `plugin install`.

| Flag | Default | Description |
| --- | --- | --- |
| `--all` | `false` | Update every installed plugin to its latest version. |
| `--lock-file-only` | `false` | Only write `ironstate.lock.yaml` from installed plugins; install nothing. |
| `--playbook <path>` | `.` | Playbook whose `ironstate.lock.yaml` is written. Required to exist when given, or with `--lock-file-only`. |

```shell
$ ironstate plugin update --lock-file-only --playbook ~/playbooks/workstation
wrote /home/ryan/playbooks/workstation/ironstate.lock.yaml (2 plugin(s))
```

## `uninstall`

```shell
ironstate plugin uninstall <organization.plugin>[@version]
```

With `@version`, removes only that version. Without it, removes **every** installed version of the plugin.

## `test`

```shell
ironstate plugin test <organization.plugin> --handler <name> --item '<yaml-or-json>' [--apply]
```

Launches the latest installed version and calls the handler's `Test` operation, then `Describe`, with the given item. No playbook is needed. With `--apply`, it then runs `Install` or `Uninstall`, depending on the item's state.

| Flag | Required | Description |
| --- | --- | --- |
| `--handler <name>` | yes | Handler name inside the plugin, such as `entry`. |
| `--item <mapping>` | yes | The handler item as a YAML or JSON mapping. |
| `--apply` | | Run the install or uninstall operation after testing. |

```shell
ironstate plugin test acme.hosts --handler entry --item '{ip: 10.0.0.12, hostname: build.local}'
```

## `bench`

```shell
ironstate plugin bench <organization.plugin> --handler <name> --item '<yaml-or-json>' [--operation test] [--iterations 10] [--apply]
```

Calls one handler operation repeatedly and prints timing statistics as JSON: total, average, minimum, and maximum, in nanoseconds and milliseconds.

| Flag | Default | Description |
| --- | --- | --- |
| `--handler <name>` | | Handler name inside the plugin. |
| `--item <mapping>` | | The handler item as a YAML or JSON mapping. |
| `--operation <op>` | `test` | `test`, `describe`, `install`, or `uninstall`. |
| `--iterations <n>` | `10` | Number of calls. |
| `--apply` | `false` | Required for `install` or `uninstall`, which make real changes. |

```shell
ironstate plugin bench acme.hosts --handler entry --item '{"hostname":"build.local"}' --operation test --iterations 100
```

## See also

- [Using a plugin](../../plugins/using): declaring plugins in a playbook and using the lockfile.
- [`doctor --playbook`](./doctor#plugins---playbook): validating a playbook's plugins.
