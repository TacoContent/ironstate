---
layout: default
title: macports
parent: 👋 Handlers
nav_order: 27
---

# macports

The `macports` handler manages packages via MacPorts (macOS).

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `package` / `name` | no | - | Package name as known to MacPorts. Can be string or array. |
| `state` | no | `present` | `present` / `absent` / `latest` |
