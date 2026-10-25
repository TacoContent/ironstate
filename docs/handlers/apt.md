---
layout: default
title: apt
parent: 👋 Handlers
nav_order: 3
---

# apt

The `apt` handler manages packages via APT (Debian/Ubuntu).

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `package` / `name` | no | - | Package name as known to APT. Can be string or array. |
| `state` | no | `present` | `present` / `absent` / `latest` |
