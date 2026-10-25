---
layout: default
title: homebrew | brew
parent: 👋 Handlers
nav_order: 22
---

# homebrew | brew

The `homebrew` handler manages packages via Homebrew (macOS/Linux). Also available as `brew` (alias).

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `package` / `name` | no | - | Package name as known to Homebrew. Can be string or array. |
| `state` | no | `present` | `present` / `absent` / `latest` |
