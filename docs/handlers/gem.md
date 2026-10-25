---
layout: default
title: gem
parent: 👋 Handlers
nav_order: 18
---

# gem

The `gem` handler manages packages via RubyGems.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `package` / `name` | no | - | Package name as known to gem. Can be string or array. |
| `state` | no | `present` | `present` / `absent` / `latest` |
