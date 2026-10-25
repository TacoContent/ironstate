---
layout: default
title: eget
parent: 👋 Handlers
nav_order: 13
---

# eget

The `eget` handler fetches GitHub release binaries with `eget`.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `package` | yes | - | GitHub repo in owner/name form, e.g. `zyedidia/eget`. |
| `args` | no | - | Extra arguments passed to `eget`. `--to=` paths support `~` expansion. |
| `state` | no | `present` | `present` / `absent` / `latest` |
