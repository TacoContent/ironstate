---
layout: default
title: xget
parent: 👋 Handlers
nav_order: 48
---

# xget

The `xget` handler fetches GitHub release binaries with [xget](https://github.com/camalot/xget).

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `package` | yes | - | GitHub repo in owner/name form, e.g. `camalot/xget`. |
| `args` | no | - | Extra arguments passed to `xget`. `--to=` paths support `~` expansion. |
| `state` | no | `present` | `present` / `absent` / `latest` |
