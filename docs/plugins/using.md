---
layout: default
title: ✍️ Using A Plugin
nav_order: 2
parent: 🧩 Plugins
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# ✍️ Using A Plugin
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

## Declare the plugin

List the plugins a playbook needs under `plugins:` in its `main.yml`, then use their handlers with fully qualified names:

```yaml
plugins:
  - use: acme.hosts@v1.2.3

tasks:
  - name: Ensure the build host resolves locally
    acme.hosts.entry:
      path: /etc/hosts
      ip: 10.0.0.12
      hostname: build.local
```

| Part | Meaning |
| --- | --- |
| `acme` | The organization. |
| `hosts` | The plugin name. Together, `acme.hosts` maps to the Go module `github.com/acme/ironstate-handler-hosts`. |
| `@v1.2.3` | The version. Required. `@latest` means the newest version **installed** on this machine, which isn't reproducible; prefer an exact version. |
| `entry` | One handler the plugin provides. A plugin can provide several. |

Plugin tasks work like built-in ones. They support `state`, `when`, `loop`, `tags`, `become`, `id`, `failed_when`, and `continue_on_error`. See [Tasks](../playbooks/tasks).

> [!NOTE]
> `ironstate validate` doesn't know about `plugins:` or qualified handler names yet, so it reports them as schema errors. Use `ironstate doctor --playbook` (below) to check a plugin playbook.

## Install the plugin

Plugins are installed per user, with the Go toolchain:

```shell
ironstate plugin install acme.hosts@v1.2.3
```

This runs `go install github.com/acme/ironstate-handler-hosts@v1.2.3`, checks that the binary starts and completes the plugin handshake, and stores it in ironstate's plugin cache:

| OS | Plugin cache |
| --- | --- |
| Linux | `~/.cache/ironstate/plugins` |
| macOS | `~/Library/Caches/ironstate/plugins` |
| Windows | `%LocalAppData%\ironstate\plugins` |

Several versions can be installed side by side. Manage them with [`ironstate plugin`](../cli/commands/plugin): `list`, `info`, `update`, and `uninstall`.

### Installing automatically

If a declared plugin isn't installed, the run stops with the command to fix it:

```text
plugin acme.hosts@v1.2.3 is not installed; run: ironstate plugin install acme.hosts@v1.2.3
```

Pass `--allow-plugin-install` to let ironstate install missing plugins itself. This needs Go on the machine.

```shell
ironstate --playbook . --apply --allow-plugin-install
```

## Pin with a lockfile

An `ironstate.lock.yaml` beside `main.yml` pins each plugin to an exact build:

```yaml
plugins:
  acme.hosts:
    version: v1.2.3
    checksum: sha256:4f6c0e1d...
```

- The lockfile's `version` **overrides** the version in `plugins:`.
- Before launching the plugin, ironstate compares the installed binary's checksum with `checksum`. A mismatch is a hard error; ironstate never falls back to another version.

ironstate doesn't write the lockfile for you. Copy the values from `ironstate plugin info`:

```shell
$ ironstate plugin info acme.hosts
acme.hosts@v1.2.3
  source: github.com/acme/ironstate-handler-hosts
  checksum: sha256:4f6c0e1d...
  license:
  handlers: entry
  summary:
```

When you upgrade a plugin, update both `version` and `checksum`.

> [!IMPORTANT]
> The checksum covers the binary built for **one** OS and architecture. A lockfile written on Linux amd64 rejects the same plugin built for Windows or arm64. Keep one lockfile per platform you apply from, or leave plugins unlocked for mixed fleets.

## Check the playbook

```shell
ironstate doctor --playbook path/to/playbook
```

`doctor` checks that every declared plugin is installed and matches the lockfile, launches each one, and confirms every qualified handler the playbook uses is provided by its plugin. Each problem includes the command that fixes it.

To try a single handler without a playbook, use [`ironstate plugin test`](../cli/commands/plugin):

```shell
ironstate plugin test acme.hosts --handler entry \
  --item '{"path":"/tmp/hosts","ip":"10.0.0.12","hostname":"build.local"}'
```

## Remote targets

Plugin playbooks work with [remote apply](../playbooks/inventory#plugins). Handlers run on the target:

- **Same OS and architecture as the controller**: the controller copies its installed plugin, at the locked version when there is one, to the target.
- **Otherwise**: the target uses its own installed plugins. `--allow-plugin-install` is passed through, so the target can install them itself if it has Go.

See [Plugins with remote apply](./#plugins-with-remote-apply) for details.

## Trust

A plugin is native code that runs with your privileges. It is **not sandboxed**. Before you use a plugin:

- Review its source, or use plugins from publishers you trust.
- Pin an exact version and its checksum in `ironstate.lock.yaml`.
- Re-review when you upgrade.
