---
layout: default
title: symlinks
parent: 👋 Handlers
nav_order: 41
---

# symlinks

The `symlinks` handler manages symlinks (batch operations).

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `src` | no | - | Source path. |
| `dest` | no | - | Destination path. |
| `links` | no | - | List of link mappings. |
| `state` | no | `present` | `present` / `absent` / `latest` |
