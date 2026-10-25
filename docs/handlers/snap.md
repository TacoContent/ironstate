---
layout: default
title: snap
parent: 👋 Handlers
nav_order: 38
---

# snap

The `snap` handler manages packages via Snap.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `package` / `name` | no | - | Package name as known to Snap. Can be string or array. |
| `state` | no | `present` | `present` / `absent` / `latest` |
