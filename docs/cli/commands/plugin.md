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
ironstate plugin update <organization.plugin>[@version]
ironstate plugin update --all
```

Installs the given version, or `latest`, side by side with existing versions. Pass exactly one plugin **or** `--all`.

| Flag | Description |
| --- | --- |
| `--all` | Update every installed plugin to its latest version. |

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
