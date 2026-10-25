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
| `--playbook <path>` | yes | Playbook file, directory, or bare name. It's resolved the same way as for a [playbook run](../index.md#playbook-resolution). |
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

> [!NOTE]
> Every YAML file under the playbook directory is checked against the **playbook** schema. Keep other YAML files outside the playbook directory if they would not pass, such as an inventory, which uses [`inventory.schema.json`](../../playbooks/inventory.md).

## Schema lookup

`validate` looks for `ironstate.schema.json` in this order:

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
