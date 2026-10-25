---
layout: default
title: flatpak
parent: 👋 Handlers
nav_order: 17
---

# flatpak

The `flatpak` handler manages packages via Flatpak.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `package` / `name` | no | - | Package name as known to Flatpak. Can be string or array. |
| `state` | no | `present` | `present` / `absent` / `latest` |
