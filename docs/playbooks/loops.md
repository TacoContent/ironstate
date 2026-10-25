---
render_with_liquid: false
layout: default
title: 🔁 Loops
nav_order: 8
parent: 📖 Playbooks
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🔁 Loops
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
Add `items` to repeat a task once for each entry in a list, or `with` to bind a single value. Inside the task, `item` holds the current value.

## `items`

```yaml
- name: Install ${{ item.package }}
  xget:
    package: ${{ item.package }}
    state: ${{ item.state | default('present') }}
    args: ${{ item.args }}
  items:
    - package: sharkdp/fd
      args: [--to=~/.local/bin/fd.exe]
    - package: BurntSushi/ripgrep
```

The second entry has no `args`. Because `args: ${{ item.args }}` is the whole field and can't be resolved, the field is left out and `xget` uses its default.

`items` can also come from a variable or an expression:

```yaml
vars:
  vscode_extensions: [ms-python.python, golang.go]

tasks:
  - name: VS Code extension ${{ item }}
    shell: code --install-extension ${{ item }}
    items: ${{ vscode_extensions }}
```

## `with`

`with` binds `item` to one value without repeating anything, even when the value is a list. Use it to give a task or group a short alias for a long path:

```yaml
- name: Configure ${{ item.name }}
  with: ${{ development.editors.micro }}
  when: item.enabled
  copy:
    src: files/micro/settings.json
    dest: ~/.config/micro/settings.json
```

When a task has both `items` and `with`, `with` is ignored.

## Looping a group

`items` on a grouping task repeats the **whole group** for each entry:

```yaml
- name: Set up ${{ item.name }}
  items:
    - { name: alice, shell: /bin/zsh }
    - { name: bob, shell: /bin/bash }
  actions:
    - user: { name: "${{ item.name }}", shell: "${{ item.shell }}" }
    - log: { message: "configured ${{ item.name }}" }
```

## Nested loops and `parent`

A loop inside a looped group rebinds `item` to the inner value. The outer value is then available as `parent.item`, and as `parent.parent.item` one level further out, and so on:

```yaml
- name: For each user
  items: ${{ vars.users }}        # [{ name: alice, repos: [a, b] }, { name: bob, repos: [c] }]
  actions:
    - name: For each repo
      items: ${{ item.repos }}    # still the OUTER item here
      log:
        message: "${{ parent.item.name }} owns ${{ item }}"
```

This prints `alice owns a`, `alice owns b`, and `bob owns c`.

> [!IMPORTANT]
> A loop's own `items` / `with` line is resolved **before** its loop starts, so there `item` still means the **outer** value. Everything else on that task (`when`, `name`, module fields, and children) sees the **inner** value, with the outer one in `parent.item`.

## Loops and `id`

When a looped task has an `id`, the top-level result fields hold the **last** iteration's values, and every iteration's result is kept in `.results`:

```yaml
- id: probes
  shell: echo ${{ item }}
  items: [one, two]

- log: { message: "first: ${{ probes.results[0].stdout }}, last: ${{ probes.stdout }}" }
```

See [Results](./results).
