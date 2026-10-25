---
layout: default
title: pipx
parent: 👋 Handlers
nav_order: 32
---

# pipx

The `pipx` handler manages packages via pipx (Python CLI apps).

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `package` / `name` | no | - | Package name as known to pipx. Can be string or array. |
| `state` | no | `present` | `present` / `absent` / `latest` |
