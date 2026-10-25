---
render_with_liquid: false
layout: default
title: 🌐 Remote Sources
nav_order: 13
parent: 📖 Playbooks
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🌐 Remote Sources
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
`uses` is like [`include`](./includes.md), but it can pull tasks from **outside** the playbook: a git repository, another local folder, or a network share. Use it to share roles between playbooks or teams.

```yaml
- name: Shared SSH role
  tags: [ssh]
  uses:
    remote: git@github.com:camalot/ironstate-playbook-shared.git
    path: roles/ssh
    ref: v1.2.0
    with:
      vars: { ssh_port: 2222 }
      authorized_users: [ryan]
```

## Fields

| Field | Description |
| --- | --- |
| `remote` | **Required.** The source. See [Sources](#sources). |
| `path` | A folder inside the source, such as `roles/ssh`. It must stay inside the source. |
| `ref` | Git branch, tag, or commit. Ignored for local folders. |
| `isolate` | Run the tasks in a sandbox. See [Isolation](#isolation). Default `false`. |
| `trusted` | Pre-approve this source and skip the [trust prompt](#trust-prompt). |
| `name`, `state`, `tags` | Exposed as `package.name`, `package.state`, and `package.tags`, like `include`. |
| `with.vars`, `with.facts` | Variables and facts to hand to the tasks. |
| `with.<other>` | Inputs exposed as `${{ inputs.<key> }}`. |

`tags`, `when`, and `envs` **beside** `uses:` apply to every pulled-in task. Inside the source, relative paths, including nested `include` names, resolve from the source's own folder.

## Sources

| `remote` | Type |
| --- | --- |
| `git@github.com:owner/repo.git` | git over SSH |
| `ssh://git@host/owner/repo.git` | git over SSH |
| `https://host/owner/repo.git` | git over HTTPS. The `.git` suffix is required; without it, the value is treated as a local path. |
| `git://host/owner/repo`, `git+ssh://…`, `git+https://…` | git |
| `./shared`, `/srv/playbooks/shared`, `C:\playbooks\shared` | local folder (a relative path resolves from the playbook) |
| `\\server\share\playbooks` | network share |

Git sources are cloned once for each `remote` and `ref` pair, and the clone is cached under your user cache folder: `~/.cache/ironstate/remotes` on Linux, `~/Library/Caches/ironstate/remotes` on macOS, or `%LocalAppData%\ironstate\remotes` on Windows. Each clone or fetch times out after 2 minutes. Delete the cache folder to force a fresh clone. Authentication uses your normal git setup; ironstate never handles credentials.

## Isolation

With `isolate: true`, the pulled-in tasks:

- **can't elevate**: `become` is ignored, with a warning.
- **see only what you pass**: `with.vars` and `with.facts`. They don't see this machine's facts, your `vars`, or earlier results.

Isolation carries down: anything nested inside an isolated source is isolated too.

With `isolate: false` (the default), the tasks run like your own: they see the same facts and vars (with `with.vars` merged on top), and `become` works.

## Trust prompt

A **remote**, **non-isolated** source runs someone else's code with full access to your machine, so ironstate asks before fetching it:

```text
⚠ This playbook uses a remote, non-isolated source:
    git@github.com:camalot/ironstate-playbook-shared.git@main (roles/ssh) [git]
  It will run with full access to your facts/vars and may request elevation (become).
  Fetch and run it? [y/N]:
```

If you decline, the source is skipped and the run continues. Local folders, network shares, and isolated sources aren't prompted.

To pre-approve:

- **One source**: add `trusted: true`, and pin `ref` to a commit:

  ```yaml
  uses:
    remote: git@github.com:camalot/ironstate-playbook-shared.git
    path: roles/ssh
    ref: 9f2c1ab
    trusted: true
  ```

- **Every source in a run**: pass `--allow-remote-uses`.

In non-interactive runs (CI, scheduled tasks), an unapproved source is **declined**.

## With remote apply

When you run against other machines with `--target` or `--inventory`, the **controller** prompts, clones, and caches the sources, then sends them to each target inside the bundle. Targets don't need git or network access.

- Only a literal `remote:` can be pre-fetched. A templated `remote: "${{ … }}"` fails on the target.
- Keep local `uses` paths inside the playbook folder so they're included in the bundle.
