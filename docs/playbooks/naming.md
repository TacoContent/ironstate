---
render_with_liquid: false
layout: default
title: 🏷️ File Naming
nav_order: 4
parent: 📖 Playbooks
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🏷️ File Naming
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
Overlay files are named after **facts about the machine**. ironstate merges every overlay whose name matches the current machine, in a predictable order. This lets one playbook serve many machines.

Naming applies in three places:

- `hosts/`
- `variables/`
- next to the `main.yml` of any [included](./includes) folder, such as `roles/shells/windows.yml`

## Name parts

A file name is one or more **parts** joined with `.`, followed by `.yml` or `.yaml`. Each part must match one of these facts:

| Fact | Weight | Examples |
| --- | --- | --- |
| `computer_name` (hostname) | 16 | `krayt`, `snoke` |
| `os_family` | 8 | `windows`, `darwin`, `debian`, `archlinux`, `redhat` |
| `distribution` | 4 | `windows`, `darwin`, `ubuntu`, `manjaro`, `fedora` |
| `platform` | 2 | `windows`, `linux`, `darwin` |
| `arch` | 1 | `amd64`, `arm64` |

**Every** part must match this machine, or the file is skipped. Files are found by exact name, so case sensitivity follows the filesystem: case-insensitive on Windows and (by default) macOS, case-sensitive on Linux. Use lowercase names; Linux facts are lowercase.

```text
windows.yml                 # any Windows machine
linux.yml                   # any Linux machine
archlinux.yml / arch.yml    # Arch and Arch-based distributions (Manjaro, EndeavourOS, …)
manjaro.yml                 # only Manjaro
ubuntu.yml                  # only Ubuntu
arm64.yml                   # any arm64 machine
arm64.linux.yml             # arm64 AND Linux
krayt.yml                   # the machine named krayt
krayt.amd64.yml             # krayt, when it's amd64
krayt.amd64.archlinux.linux.yml
```

**Order doesn't matter.** `amd64.krayt.yml` and `krayt.amd64.yml` are the same overlay.

## OS family and distribution

On Linux, the values come from `/etc/os-release`:

- `os_family` is the **first `ID_LIKE` entry**, or `ID` if there's no `ID_LIKE`. Use it to target a whole family of distributions.
- `distribution` is **`ID`**, or the first `ID_LIKE` entry if there's no `ID`. Use it to target one exact distribution.

| Machine | `ID` | `ID_LIKE` | `os_family` | `distribution` |
| --- | --- | --- | --- | --- |
| Arch Linux | `arch` | — | `archlinux` | `archlinux` |
| Manjaro | `manjaro` | `arch` | `archlinux` | `manjaro` |
| Ubuntu | `ubuntu` | `debian` | `debian` | `ubuntu` |
| Debian | `debian` | — | `debian` | `debian` |
| Fedora | `fedora` | — | `fedora` | `fedora` |
| Windows | | | `windows` | `windows` |
| macOS | | | `darwin` | `darwin` |

`arch` and `archlinux` are treated as the same name, both in file names and in conditions such as `facts.os_family == 'arch'`.

## Precedence

Every matching file is merged, from the **lowest** total weight to the **highest**, so the most specific file wins. A file's weight is the sum of its parts' weights:

| File | Parts | Weight |
| --- | --- | --- |
| `main.yml` | — | merged first |
| `amd64.yml` | arch | 1 |
| `linux.yml` | platform | 2 |
| `manjaro.yml` | distribution | 4 |
| `archlinux.yml` | os_family | 8 |
| `archlinux.linux.amd64.yml` | os_family + platform + arch | 11 |
| `traya.yml` | hostname | 16 |
| `traya.amd64.yml` | hostname + arch | 17 |
| `ryan.yml` (in `variables/` only) | `USERNAME` env var | merged last |

A hostname is worth more than all the other parts together (16 > 8 + 4 + 2 + 1), so **any file that names the machine outranks any file that doesn't**.

What "wins" means depends on the key:

- **`vars`**: a later file overrides earlier values, key by key (deep merge).
- **`tasks`**: tasks from every matching file run, in merge order.

## Tips

- Put things that every machine needs in `main.yml`, and narrow them down with overlays.
- Prefer `os_family` (`debian.yml`, `archlinux.yml`) for package-manager differences, and `distribution` for distro-specific quirks.
- To check which facts a machine reports, add a `log` task:

  ```yaml
  - log:
      message: "${{ facts.computer_name }} ${{ facts.os_family }} ${{ facts.distribution }} ${{ facts.platform }} ${{ facts.arch }}"
  ```
