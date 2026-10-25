---
render_with_liquid: false
layout: default
title: 📖 Playbooks
nav_order: 4
has_children: true
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 📖 Playbooks
{: .no_toc }

A **playbook** is a folder of YAML files that describes the state you want a machine to be in: which packages are installed, which files exist, which settings are applied. ironstate reads the playbook, works out what's different on this machine, and changes only that.

```yaml
# main.yml
vars:
  editor: nvim

tasks:
  - name: Core CLI tools
    tags: [cli]
    actions:
      - winget: { package: BurntSushi.ripgrep.MSVC }
        when: facts.platform == 'windows'
      - pacman: { package: [ripgrep, fd] }
        become: true
        when: facts.os_family == 'archlinux'

  - name: Editor
    include:
      name: packages/editor
      with:
        editor: ${{ vars.editor }}
```

## How a run works

1. **Load**: read `main.yml`, then merge the overlays from `hosts/` and `variables/` that match this machine.
2. **Gather facts**: collect built-in facts (hostname, OS, architecture, and so on), then run every `fact` task.
3. **Flatten**: expand groups, includes, `uses`, and loops into one ordered list of leaf actions, carrying `tags` and `when` down to each leaf.
4. **Filter**: drop leaves that don't match `--tags`.
5. **Dispatch**: for each leaf, in order, evaluate `when`, resolve `${{ }}` expressions, and ask the handler whether the leaf is already satisfied. If it isn't, describe the change (dry-run) or make it (`--apply`).

Because each leaf is resolved just before it runs, later tasks can react to earlier tasks' results.

## In this section

| Page | Covers |
| --- | --- |
| [Folder structure](./structure) | What each folder in a playbook is for. |
| [Task files](./task-files) | The top-level keys of a YAML file: `tasks`, `vars`, `envs`, `tags`, `plugins`. |
| [Tasks](./tasks) | Leaf actions, grouping tasks, modules, and the shared task fields. |
| [File naming](./naming) | How overlay file names select machines, and their precedence. |
| [Hosts and variables](./hosts) | Per-machine and per-user overlays in `hosts/` and `variables/`. |
| [Facts and variables](./facts-vars) | Built-in facts, `vars`, and variable precedence. |
| [Conditions and expressions](./conditions) | `when`, `${{ }}` templates, operators, and type tests. |
| [Loops](./loops) | `items`, `with`, and nested loops with `parent`. |
| [Results and failures](./results) | `id`, `failed_when`, `continue_on_error`, and `become`. |
| [Tags](./tags) | Tagging tasks and running a subset with `--tags`. |
| [Environment variables](./environment) | `envs` / `environment` and `valueFrom` sources. |
| [Includes](./includes) | Reusable packages and roles with `include`. |
| [Remote sources](./uses) | Pulling tasks from git or shared folders with `uses`. |
| [Secrets](./secrets) | `.env`, `.secrets`, and redacting sensitive values. |
| [Inventory](./inventory) | Running a playbook on other machines over SSH. |

For each module's fields, such as `winget`, `shell`, or `copy`, see [Handlers](../handlers/).
