---
layout: default
title: scoop
parent: 👋 Handlers
nav_order: 35
---

# scoop

The `scoop` handler manages packages via Scoop (Windows).

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `package` / `name` | no | - | Package name as known to Scoop. Can be string or array. |
| `state` | no | `present` | `present` / `absent` / `latest` |
