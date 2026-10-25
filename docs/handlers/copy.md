---
layout: default
title: copy
parent: 👋 Handlers
nav_order: 9
---

# copy

The `copy` handler copies files or directories from `src` to `dest`.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `src` | yes | - | Source file or directory path. Resolved relative to the install system directory (install/windows, or the owning package's directory for packages/<name>/main.yml) unless absolute, `~`-prefixed, or a URL. If `src` is a directory, a trailing `/` copies its contents directly into `dest`; without one, it nests as `dest/<src folder name>/...` (rsync/Ansible convention). |
| `dest` | yes | - | Destination path. `~` expansion is supported. Becomes a directory when `src` is a directory. |
| `owner` | no | - | Ownership metadata applied to copied file(s). |
| `group` | no | - | Group metadata applied to copied file(s). |
| `mode` | no | - | Mode bits applied to copied file(s) (e.g. `0644` or `420`). |
| `state` | no | `present` | `present` / `absent` / `latest` |

## Behavior

Copies files/directories recursively. Handles trailing slash on directories to copy contents directly. Applies metadata if specified.
