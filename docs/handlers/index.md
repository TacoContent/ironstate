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

Every task option, such as `when`, `loop`, `tags`, `become`, `id`, and `failed_when`, works with every handler. See [Tasks](../playbooks/tasks).

Built-in handlers can also be written with an `ironstate.builtin.` prefix, such as `ironstate.builtin.shell`. To add your own handlers, see [Plugins](../plugins/).

## Package managers

| Handler | Manages |
| --- | --- |
| [`apk`](./apk) | Alpine Linux packages |
| [`apt`](./apt) | Debian and Ubuntu packages |
| [`cargo`](./cargo) | Rust crates |
| [`chocolatey`](./chocolatey) | Chocolatey packages (Windows) |
| [`flatpak`](./flatpak) | Flatpak apps |
| [`gem`](./gem) | RubyGems |
| [`go`](./go) | Go binaries via `go install` |
| [`homebrew`](./homebrew) / `brew` | Homebrew packages (macOS and Linux) |
| [`macports`](./macports) | MacPorts packages (macOS) |
| [`npm`](./npm) | Global npm packages |
| [`pacman`](./pacman) | Arch Linux packages |
| [`pipx`](./pipx) | Python CLI apps |
| [`scoop`](./scoop) | Scoop packages (Windows) |
| [`snap`](./snap) | Snap packages |
| [`winget`](./winget) | Windows Package Manager packages |
| [`yum`](./yum) | YUM and DNF packages (RHEL, CentOS, Fedora) |

## Release downloads

| Handler | Manages |
| --- | --- |
| [`eget`](./eget) | GitHub release binaries, using `eget` |
| [`xget`](./xget) | GitHub release binaries, using `xget` |

## Files and directories

| Handler | Manages |
| --- | --- |
| [`blockinfile`](./blockinfile) | A marker-delimited block of text in a file |
| [`copy`](./copy) | Copying files and directories |
| [`file`](./file) | Files, directories, symlinks, and hard links |
| [`git`](./git) | A git checkout |
| [`lineinfile`](./lineinfile) | A single line in a file |
| [`path`](./path) | Entries on the user's `PATH` |
| [`ssh_host_block`](./ssh_host_block) | `Host` blocks in an ssh config file |
| [`stat`](./stat) | File and directory information (read-only) |
| [`symlinks`](./symlinks) | Batches of symlinks |
| [`template`](./template) | Files rendered from templates |
| [`zip`](./zip) | Creating and extracting ZIP archives |

## System

| Handler | Manages |
| --- | --- |
| [`cron`](./cron) | Scheduled jobs on any platform |
| [`cron_file`](./cron_file) | `cron.d`-style cron files |
| [`cron_unix`](./cron_unix) | crontab entries |
| [`group`](./group) | Local groups |
| [`mount_facts`](./mount_facts) | Mount point facts (read-only) |
| [`registry`](./registry) | Windows Registry entries |
| [`scheduled_task`](./scheduled_task) | Windows Scheduled Tasks |
| [`service`](./service) | System services |
| [`user`](./user) | Local users |

## Firewalls

| Handler | Manages |
| --- | --- |
| [`firewall`](./firewall) | Firewall rules on any platform |
| [`advfirewall`](./advfirewall) | Windows Advanced Firewall rules |
| [`iptables`](./iptables) | iptables rules (Linux) |
| [`ufw`](./ufw) | UFW rules (Linux) |

## Running commands

| Handler | Does |
| --- | --- |
| [`shell`](./shell) | Runs commands or scripts in any shell or interpreter |
| [`async`](./async) | Runs tasks asynchronously |
| [`wait_for`](./wait_for) | Waits for a condition |

## Flow and data

| Handler | Does |
| --- | --- |
| [`assert`](./assert) | Checks conditions and fails if they're false |
| [`fact`](./fact) | Sets a fact for later tasks |
| [`fail`](./fail) | Fails the task with a message |
| [`include`](./include) | Runs another task file from the playbook. See [Includes](../playbooks/includes). |
| [`log`](./log) | Prints a message |
| [`uses`](./uses) | Runs tasks from a git repository or another folder. See [Remote Sources](../playbooks/uses). |
