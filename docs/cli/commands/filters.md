---
render_with_liquid: false
layout: default
title: 🔎 Filters
nav_order: 3
parent: 🖥️ Commands
grand_parent: 🖥️ CLI
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🧹 Filters
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
Inspects the filters available to `when` conditions and `${{ }}` expressions.

## Usage

```shell
ironstate filters list [--dir <dir>]
```

## `filters list`

Prints every available filter, sorted by name, one per line, with its kind:

- `built-in`: compiled into ironstate. See [Built-in filters](../../filters/builtin.md).
- `script`: a script discovered under `--dir`. See [Custom filters](../../filters/custom.md).

| Flag | Default | Description |
| --- | --- | --- |
| `--dir <dir>` | `filters` | Directory to scan for script filters. Relative to the current directory. |
| `-h`, `--help` | | Show help. |

```text
$ ironstate filters list --dir path/to/playbook/filters
basename             built-in
concat               built-in
contains             built-in
default              built-in
...
upper                built-in
leet                 script
```

A script whose name matches a built-in filter is never registered; the built-in always wins and is listed as `built-in`.

> [!NOTE]
> During a playbook run, ironstate discovers script filters in the `filters/` folder next to the playbook's `main.yml`, or the folder set by `filters.dir` in `ironstate.yaml`. `filters list` scans `--dir` relative to your current directory, so point it at the playbook's folder when you run it from elsewhere.
