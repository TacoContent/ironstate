---
layout: default
title: uses
parent: 👋 Handlers
nav_order: 45
---

# uses

The `uses` handler pulls in another playbook's task list from a remote or external source - a git repository (ssh or https), a local directory, or a network share - as if written inline in place of this action.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `remote` | yes | - | Source location: git URL (`git@host:owner/repo.git`, `ssh://...`, `https://...`, `git://...`), local absolute/relative directory path, or UNC network share (`\\server\share\playbooks`). Relative path resolved against consuming playbook's directory. |
| `path` | no | - | Optional sub-directory within the source holding the role/task/package to use. Must stay inside the source. |
| `ref` | no | - | Git branch, tag, or commit to check out. Ignored for local/network-share sources. |
| `trusted` | no | `false` | Mark remote source as trusted (skips interactive confirmation). |
| `isolate` | no | `false` | Run in isolation. |
| `allow` | no | - | Allowlist configuration. |
| `deny` | no | - | Denylist configuration. |
| `state` | no | `present` | `present` / `absent` / `latest` |
