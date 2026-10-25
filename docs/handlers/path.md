---
layout: default
title: path
parent: 👋 Handlers
nav_order: 31
---

# path

The `path` handler ensures directories are present on (or absent from) the current user's persistent PATH environment variable (User scope, no admin required). Also updates the current process's PATH for entries it adds/removes, so later tasks in the same run can find them immediately.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `paths` | yes | - | Directories to ensure on PATH. `~` expansion is supported. |
| `state` | no | `present` | `present` / `absent` / `latest` |
