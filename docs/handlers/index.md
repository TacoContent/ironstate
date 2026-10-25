---
layout: default
title: 👋 Handlers
nav_order: 5
has_children: true
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 👋 Handlers
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

A **handler** is the part of a task that does the work. The handler's name is the task key, and its settings are the value:

```yaml
- name: Install ripgrep
  apt:
    package: ripgrep
    state: present
```

Most handlers support `state`:

| `state` | Effect |
| --- | --- |
| `present` (default) | Install or create it if it's missing. Otherwise, skip. |
| `absent` | Remove it if it's there. Otherwise, skip. |
| `latest` | Always run, so package managers can upgrade. |

Every task option, such as `when`, `loop`, `tags`, `become`, `id`, and `failed_when`, works with every handler. See [Tasks](../playbooks/tasks.md).

Built-in handlers can also be written with an `ironstate.builtin.` prefix, such as `ironstate.builtin.shell`. To add your own handlers, see [Plugins](../plugins/index.md).

## Package managers

| Handler | Manages |
| --- | --- |
| [`apk`](./apk.md) | Alpine Linux packages |
| [`apt`](./apt.md) | Debian and Ubuntu packages |
| [`cargo`](./cargo.md) | Rust crates |
| [`chocolatey`](./chocolatey.md) | Chocolatey packages (Windows) |
| [`flatpak`](./flatpak.md) | Flatpak apps |
| [`gem`](./gem.md) | RubyGems |
| [`go`](./go.md) | Go binaries via `go install` |
| [`homebrew`](./homebrew.md) / `brew` | Homebrew packages (macOS and Linux) |
| [`macports`](./macports.md) | MacPorts packages (macOS) |
| [`npm`](./npm.md) | Global npm packages |
| [`pacman`](./pacman.md) | Arch Linux packages |
| [`pipx`](./pipx.md) | Python CLI apps |
| [`scoop`](./scoop.md) | Scoop packages (Windows) |
| [`snap`](./snap.md) | Snap packages |
| [`winget`](./winget.md) | Windows Package Manager packages |
| [`yum`](./yum.md) | YUM and DNF packages (RHEL, CentOS, Fedora) |

## Release downloads

| Handler | Manages |
| --- | --- |
| [`eget`](./eget.md) | GitHub release binaries, using `eget` |
| [`xget`](./xget.md) | GitHub release binaries, using `xget` |

## Files and directories

| Handler | Manages |
| --- | --- |
| [`blockinfile`](./blockinfile.md) | A marker-delimited block of text in a file |
| [`copy`](./copy.md) | Copying files and directories |
| [`file`](./file.md) | Files, directories, symlinks, and hard links |
| [`git`](./git.md) | A git checkout |
| [`lineinfile`](./lineinfile.md) | A single line in a file |
| [`path`](./path.md) | Entries on the user's `PATH` |
| [`ssh_host_block`](./ssh_host_block.md) | `Host` blocks in an ssh config file |
| [`stat`](./stat.md) | File and directory information (read-only) |
| [`symlinks`](./symlinks.md) | Batches of symlinks |
| [`template`](./template.md) | Files rendered from templates |
| [`zip`](./zip.md) | Creating and extracting ZIP archives |

## System

| Handler | Manages |
| --- | --- |
| [`cron`](./cron.md) | Scheduled jobs on any platform |
| [`cron_file`](./cron_file.md) | `cron.d`-style cron files |
| [`cron_unix`](./cron_unix.md) | crontab entries |
| [`group`](./group.md) | Local groups |
| [`mount_facts`](./mount_facts.md) | Mount point facts (read-only) |
| [`registry`](./registry.md) | Windows Registry entries |
| [`scheduled_task`](./scheduled_task.md) | Windows Scheduled Tasks |
| [`service`](./service.md) | System services |
| [`user`](./user.md) | Local users |

## Firewalls

| Handler | Manages |
| --- | --- |
| [`firewall`](./firewall.md) | Firewall rules on any platform |
| [`advfirewall`](./advfirewall.md) | Windows Advanced Firewall rules |
| [`iptables`](./iptables.md) | iptables rules (Linux) |
| [`ufw`](./ufw.md) | UFW rules (Linux) |

## Running commands

| Handler | Does |
| --- | --- |
| [`shell`](./shell.md) | Runs commands or scripts in any shell or interpreter |
| [`async`](./async.md) | Runs tasks asynchronously |
| [`wait_for`](./wait_for.md) | Waits for a condition |

## Flow and data

| Handler | Does |
| --- | --- |
| [`assert`](./assert.md) | Checks conditions and fails if they're false |
| [`fact`](./fact.md) | Sets a fact for later tasks |
| [`fail`](./fail.md) | Fails the task with a message |
| [`include`](./include.md) | Runs another task file from the playbook. See [Includes](../playbooks/includes.md). |
| [`log`](./log.md) | Prints a message |
| [`uses`](./uses.md) | Runs tasks from a git repository or another folder. See [Remote Sources](../playbooks/uses.md). |
