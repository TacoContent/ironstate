---
layout: default
title: 🔖 Tags
nav_order: 10
parent: 📖 Playbooks
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🔖 Tags
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
Tags let you run part of a playbook. Add `tags` to tasks, then pick tags with `--tags` or `--tag`.

```yaml
tasks:
  - name: Developer tools
    tags: [dev]
    actions:
      - tags: [cli]
        winget: { package: BurntSushi.ripgrep.MSVC }   # tags: dev, cli
      - winget: { package: Git.Git }                   # tags: dev

  - name: Steam
    tags: [gaming]
    winget: { package: Valve.Steam }

  - name: Always runs
    log: { message: "no tags" }
```

```shell
ironstate --tags cli                 # ripgrep + "Always runs"
ironstate --tags dev,gaming          # everything
ironstate --tag dev --tag gaming     # same thing
```

## How tags are matched

- A leaf's **effective tags** are its own tags plus those of every enclosing group, include, and file.
- With no tags selected, everything runs.
- With tags selected, a leaf runs if **any** of its effective tags is selected.
- A leaf with **no effective tags** always runs. This keeps untagged prerequisites, such as facts that later conditions depend on, working under any `--tags` selection.

Tags only decide whether a task is considered. `when` still applies.

## Where tags come from

| Location | Applies to |
| --- | --- |
| `tags:` on a task | That task and, for groups, everything inside it. |
| `tags:` on an `include` / `uses` task (beside it, not inside) | Every included task. |
| `tags:` at the top of a `main.yml` | Every task in that file **and** its overlays. |
| `tags:` at the top of an overlay (`windows.yml`) | Only that overlay's tasks. |

> [!NOTE]
> `include: { name: x, tags: [a] }` (tags **inside** `include`) is different: it doesn't tag anything. It only passes `package.tags` to the package. See [Includes](./includes).

## Selecting tags

- `--tags a,b` and `--tag a --tag b` can be combined.
- You can also set them with the `IRONSTATE_TAGS` environment variable or `tags:` in `ironstate.yaml`; flags on the command line override both. See [Flags](../cli/flags).
- There's no exclusion syntax. To leave something out, give it a distinct tag and select the others.
