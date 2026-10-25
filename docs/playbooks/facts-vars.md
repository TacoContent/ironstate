---
render_with_liquid: false
layout: default
title: 🧠 Facts and Variables
nav_order: 6
parent: 📖 Playbooks
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🧠 Facts and Variables
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
Conditions and templates read from one shared context, made up of:

- **facts**: what ironstate knows about the machine.
- **vars**: values you define.
- **results**: what earlier tasks returned. See [Results](./results.md).

## Facts

Facts are gathered fresh at the start of every run.

| Fact | Description |
| --- | --- |
| `computer_name` | Hostname (`COMPUTERNAME` on Windows). |
| `user_name` | Current user name. |
| `home` | Current user's home directory. |
| `platform` | `windows`, `linux`, or `darwin`. |
| `arch` | `amd64`, `arm64`, … |
| `os_family` | Linux: the first `ID_LIKE` entry from `/etc/os-release`, or `ID` (for example, `archlinux` for Manjaro, `debian` for Ubuntu). `windows` / `darwin` elsewhere. |
| `distribution` | Linux: `ID` from `/etc/os-release`, or the first `ID_LIKE` entry (for example, `manjaro`, `ubuntu`). `windows` / `darwin` elsewhere. |
| `os_version` | Windows `major.minor.build`; macOS product version (`14.5`); Linux `VERSION_ID` (`24.04`) or, on rolling releases, the kernel release. |
| `os_build` | Windows build number; `0` elsewhere. |
| `is_admin` | Whether ironstate is running elevated. |
| `is_debian` / `debian_version` | Whether `/etc/debian_version` exists, and its contents. |
| `shell_pwsh` / `pwsh_version` | Whether `pwsh` is on `PATH`, and its version (or `null`). |
| `shell_bash` / `bash_version` | Same for `bash`. |
| `shell_zsh` / `zsh_version` | Same for `zsh`. |
| `shell_fish` / `fish_version` | Same for `fish`. |
| `shell_nu` / `nu_version` | Same for `nu`. |

Some Linux IDs are normalized: `arch` becomes `archlinux`, and `rhel` becomes `redhat`. In comparisons, `arch` and `archlinux` are interchangeable.

Read facts as `facts.<name>`, or by their bare name in `when`:

```yaml
- log: { message: "Hello from ${{ facts.computer_name }} (${{ facts.distribution }})" }
  when: facts.platform == 'linux'
```

### Custom facts

The [`fact`](../handlers/fact.md) module adds your own facts. Fact tasks run **before every other task**, even in a dry-run, so their values are always available:

```yaml
- name: Local bin path
  fact:
    name: local_bin_path
    value: ${{ local.bin_path | default('~/.local/bin') }}

- name: WSL installed
  fact:
    name: wsl_installed
    shell:
      command: wsl --status
    value: ${{ rc == 0 }}
```

These are read as `facts.local_bin_path` and `facts.wsl_installed`. [`mount_facts`](../handlers/mount_facts.md) adds facts about mounted filesystems.

## Variables

Define variables under `vars:` in any task file:

```yaml
vars:
  editor: nvim
  languages:
    rust: true
    java: { jdk: Eclipse.Temurin.21 }
```

Read them as `vars.<path>` or by their bare name: `vars.editor`, `editor`, `languages.java.jdk`. If a fact and a variable share a top-level name, the variable wins. Use `facts.` or `vars.` explicitly when it matters.

Variables can contain expressions, which are resolved before tasks run:

```yaml
vars:
  config_dir: "${{ facts.home | join('.config') }}"
```

## Variable precedence

When the same variable is set in several places, the later source wins. Maps **deep-merge**, so you can override one nested key without repeating the rest:

1. `main.yml`
2. `hosts/main.yml`, then `hosts/` overlays, least to most specific
3. `variables/main.yml`, then `variables/` overlays, then `variables/<USERNAME>.yml`
4. Inventory `defaults.vars` (remote apply)
5. Inventory host `vars` (remote apply)
6. `--vars-file` files, in the order given
7. `--var key=value`, applied last

```shell
ironstate --vars-file ./ci.yml --var editor=helix --var ssh.port=2222
```

`--var` values are typed: `true`, `false`, and numbers become booleans and numbers.

### Package variables

An included package's own `vars:` act as **package-local defaults**. They're visible only inside that package. If the site defines a variable with the same top-level name, the site's value replaces the package's value completely; it isn't deep-merged. To merge them instead, use the [`merge`](../filters/builtin.md#merge) filter:

```yaml
# packages/editorconfig/main.yml
vars:
  defaults:
    editorconfig: { indent_style: space, indent_size: 2 }

tasks:
  - name: Effective settings (defaults + user overrides)
    log:
      message: "${{ vars.defaults.editorconfig | merge(editorconfig) }}"
```

## Names to avoid

`package`, `inputs`, `facts`, `vars`, `item`, `parent`, and `envs` have special meanings in expressions. Don't use them as top-level variable names.
