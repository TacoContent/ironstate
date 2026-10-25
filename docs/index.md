---
render_with_liquid: false
layout: home
title: 🏠 Home
nav_order: 1
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# ![Ironstate](assets/images/logo-64x64.png){: style="vertical-align: middle;" } Ironstate
{: .fs-9 }

A declarative, Ansible-style task runner driven by YAML. Describe the state a machine should be in, preview the changes, then apply them, from a single cross-platform binary.
{: .fs-6 .fw-300 }

[Get started](./gettingstarted){: .btn .btn-primary .fs-5 .mb-4 .mb-md-0 .mr-2 }
[Install](./installation){: .btn .fs-5 .mb-4 .mb-md-0 }

---

## What is ironstate?

`ironstate` reads a **playbook**, a directory of YAML task files, and reconciles each task's desired `state` (`present`, `absent`, `latest`) against what is already on the machine. By default every run is a **dry-run**: ironstate prints what it *would* change. Nothing is modified until you pass `--apply`.

```yaml
# main.yml
tasks:
  - name: Install ripgrep
    tags: [cli]
    winget:
      package: BurntSushi.ripgrep.MSVC
      state: present
    when: platform == "windows"

  - name: Install ripgrep
    tags: [cli]
    become: true
    apt:
      package: ripgrep
    when: os_family == "debian"
```

```shell
ironstate --playbook .            # preview
ironstate --playbook . --apply    # make it so
```

## Highlights

- **One binary, every desktop OS.** Windows, Linux (amd64, arm64, armv7), and macOS (Intel and Apple Silicon).
- **Dry-run first.** Every run predicts what would change. `--apply` is always explicit.
- **Batteries included.** Built-in [handlers](./handlers/) for winget, Chocolatey, Scoop, Homebrew, apt, pacman, yum, apk, snap, flatpak, pipx, npm, cargo, go, git, files, templates, shell commands, firewall rules, users, cron, the Windows registry, and more.
- **Host-aware playbooks.** `hosts/` and `variables/` overlays are picked automatically from the machine's hostname, OS family, distribution, platform, and architecture. See [Task file naming](./playbooks/naming).
- **Expressions and filters.** `when` conditions and `${{ }}` templates share one expression language with a [filter pipeline](./filters/). You can extend it with your own PowerShell, bash, zsh, fish, or nushell filters.
- **Reusable packages.** Split work into `roles/`, `packages/`, and `tasks/` and pull them in with `include`, or share them across repositories with `uses`.
- **Remote apply.** Run the same playbook on other machines over SSH with `--target` or an [inventory](./playbooks/inventory).
- **Extensible.** Write external handler [plugins](./plugins/) in Go with the [SDK](./sdk/).

## Where to next

| I want to... | Read |
| --- | --- |
| Install ironstate | [Installation](./installation) |
| Write and run my first playbook | [Getting Started](./gettingstarted) |
| Learn the playbook format in depth | [Playbooks](./playbooks/) |
| See every command and flag | [CLI](./cli/) |
| Look up a handler's fields | [Handlers](./handlers/) |
| Transform values in expressions | [Filters](./filters/) |
| Build my own handler | [Plugins](./plugins/) and [SDK](./sdk/) |
