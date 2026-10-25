---
render_with_liquid: false
layout: default
title: 🔐 Secrets
nav_order: 14
parent: 📖 Playbooks
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🔐 Secrets
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
ironstate has two ways to keep sensitive values out of files and output: **environment files** for supplying values, and **secret markers** for hiding them.

## `.env` and `.secrets`

Before a run starts, ironstate loads `KEY=VALUE` lines from `.env`, then `.secrets`, in the **current working directory**, into its environment:

```text
# .env
EDITOR=nvim
API_URL="https://api.example.com"

# .secrets
GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxx
```

- Both files are optional.
- Blank lines and `#` comments are ignored. Matching surrounding quotes are removed.
- Loaded values override variables already set in the environment, and `.secrets` loads second, so it overrides `.env`.
- Every value from `.secrets` is automatically treated as a **secret** and masked in output.

Read the values with `lookup('env', 'NAME')`, the [`env`](../filters/builtin#env) filter, or `envs.NAME`:

```yaml
- name: Clone private repo
  git:
    repo: "https://${{ lookup('env', 'GITHUB_TOKEN') }}@github.com/me/private.git"
    dest: ~/src/private
```

> [!IMPORTANT]
> Add `.env` and `.secrets` to `.gitignore`. Neither file is ever copied to a remote target's disk. See [Remote apply](#remote-apply).

## Marking values as secret (`$`)

Prefix a variable, fact, or `id` name with `$` to mask its value in logs, tables, and JSON output. The `$` isn't part of the name you use to read it:

```yaml
vars:
  $github_token: "${{ lookup('env', 'GITHUB_TOKEN') }}"

tasks:
  - fact:
      name: $api_token
      value: ${{ lookup('file', '~/.config/api/token') | trim }}

  - id: $login
    shell: ./get-session.sh
```

| Declared as | Read as |
| --- | --- |
| `vars.$github_token` | `vars.github_token` |
| `fact: { name: $api_token }` | `facts.api_token` |
| `id: $login` | `login.stdout` |

Conditions and templates still see the **real** value. Only displayed output is replaced with `***`. For a secret `id` task, the task's name, command, and output are also hidden in verbose logs.

### Limits

- Masking matches the **exact** secret string. A transformed value, such as `${{ token | upper }}`, isn't masked.
- Values shorter than 6 characters aren't masked, to avoid hiding common words such as `true` everywhere.
- Masking protects ironstate's output. A command you run can still print or write the value itself.

## Remote apply

During a remote run, `.env` and `.secrets` values (and `--forward-env` variables) are sent **in memory** over the SSH session and are never written to the target's disk. `.secrets` values stay masked in output.

`--remote-detach` refuses to run when `.env`, `.secrets`, forwarded variables, or a become password are present, because a detached run must store its job on the target. Never put secrets directly into playbook files or `--vars-file` files.
