---
layout: default
title: 🩺 Doctor
nav_order: 2
parent: 🖥️ Commands
grand_parent: 🖥️ CLI
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🩺 Doctor
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
Checks that your environment can run the playbooks you care about.

## Usage

```shell
ironstate doctor [--filters-dir <dir>] [--playbook <path>]
```

## Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--filters-dir <dir>` | `filters` | Directory to scan for script filters. Relative to the current directory. |
| `--playbook <path>` | | Also validate the playbook's declared plugins and the plugin handlers it uses. |
| `-h`, `--help` | | Show help. |

## What it checks

### Tools on `PATH`

`doctor` looks up a fixed list of executables and prints `[ok]` with the resolved path, or `[missing]`:

`winget`, `choco`, `brew`, `apt-get`, `pipx`, `npm`, `cargo`, `go`, `gem`, `eget`, `xget`, `pwsh`, `ssh`

A `[missing]` line only matters if your playbook uses the matching handler. Examples are `winget`, `chocolatey`, and `homebrew`; `shell` tasks with `host: pwsh`; and remote apply, which needs `ssh`. Handlers also check their own tools when they run.

### Script filters

`doctor` lists every [script filter](../../filters/custom.md) discovered under `--filters-dir`. It's the same discovery that [`filters list`](./filters.md) uses.

### Plugins (`--playbook`)

With `--playbook`, `doctor` loads the playbook and its hierarchy for this machine and then, for each entry under `plugins:`:

1. Checks that the plugin is installed at the requested version, or at the version pinned in `ironstate.lock.yaml`.
2. Verifies the checksum against the lockfile, when one exists.
3. Launches the plugin binary and completes the protocol handshake.
4. Confirms that every fully qualified handler used in the playbook (`org.plugin.handler`) is provided by a declared plugin.

Each problem includes the command that fixes it, such as `ironstate plugin install acme.hosts@v1.2.3`.

## Example

```text
$ ironstate doctor --filters-dir ./filters --playbook .
[ok]      winget   C:\Users\you\AppData\Local\Microsoft\WindowsApps\winget.exe
[missing] choco    not found on PATH
[missing] brew     not found on PATH
[missing] apt-get  not found on PATH
[ok]      pipx     C:\Users\you\.local\bin\pipx.exe
...
[ok]      ssh      C:\Windows\System32\OpenSSH\ssh.exe

script filters discovered under ./filters:
  leet

plugin validation for main.yml:
[ok]      declared plugins resolve, handshake, and provide every qualified handler in use
```

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Checks completed. Missing tools alone don't fail `doctor`. |
| `1` | Plugin validation found at least one problem. |
| `2` | The playbook couldn't be loaded. |
