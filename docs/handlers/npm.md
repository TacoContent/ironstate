---
layout: default
title: npm
parent: 👋 Handlers
nav_order: 29
---

# npm

The `npm` handler manages packages via npm (global packages).

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `package` / `name` | no | - | Package name as known to npm. Can be string or array. |
| `state` | no | `present` | `present` / `absent` / `latest` |
