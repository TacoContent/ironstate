---
layout: default
title: ✅ Validate
nav_order: 8
parent: 🖥️ Commands
grand_parent: 🖥️ CLI
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# ✅ Validate
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
Validates a playbook against `ironstate.schema.json` without running it.

## Usage

```shell
ironstate validate --playbook <path>
```

| Flag | Required | Description |
| --- | --- | --- |
| `--playbook <path>` | yes | Playbook file, directory, or bare name. It's resolved the same way as for a [playbook run](../#playbook-resolution). |
| `-h`, `--help` | | Show help. |

## What gets validated

The root playbook file and **every `.yml`/`.yaml` file below its directory**, recursively: `hosts/`, `variables/`, `roles/`, `packages/`, `tasks/`, and so on. Each valid file is printed as `<file>: valid`.

```text
$ ironstate validate --playbook path/to/playbook
path/to/playbook/main.yml: valid
path/to/playbook/hosts/krayt.yml: valid
path/to/playbook/roles/shells/main.yml: valid
```

Failures report the file, YAML line and column, and the JSON-schema instance path that failed:

```text
path/to/playbook/roles/shells/main.yml:12:7: /tasks/0/shell: ...
```

Some files get special handling:

| File | Handling |
| --- | --- |
| Inventory files: top-level keys are only `defaults`, `hosts`, and `groups` | Checked against [`inventory.schema.json`](../../playbooks/inventory) instead. This includes the fragments of an inventory directory. |
| `ironstate.lock.yaml` | Skipped. See [Pin with a lockfile](../../plugins/using#pin-with-a-lockfile). |
| Directories whose names start with `.`, such as `.git` and `.github` | Skipped. |

Playbooks that use [plugins](../../plugins/using) pass: the schema accepts the `plugins:` list and any `organization.plugin.handler` task key. It can't check the fields inside a plugin task, because each plugin defines its own. Use [`ironstate doctor --playbook`](./doctor) to check that each plugin handler exists.

## Schema lookup

`validate` looks for `ironstate.schema.json` (and `inventory.schema.json`, for inventory files) in this order:

1. The current directory.
2. Next to the playbook.
3. Next to the `ironstate` executable.
4. The `develop` branch on GitHub.

If no schema can be found, the command exits non-zero and reports that the playbook can't be validated.

## Editor validation

The same schema gives you completion and inline errors in editors that support the YAML language server. Add this line to the top of a task file:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/TacoContent/ironstate/develop/ironstate.schema.json
```
