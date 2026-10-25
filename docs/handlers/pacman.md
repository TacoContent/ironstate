---
layout: default
title: pacman
parent: 👋 Handlers
nav_order: 30
---

# pacman

The `pacman` handler manages packages via pacman (Arch Linux).

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `package` / `name` | no | - | Package name as known to pacman. Can be string or array. |
| `state` | no | `present` | `present` / `absent` / `latest` |
