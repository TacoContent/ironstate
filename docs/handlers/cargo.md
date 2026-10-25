---
layout: default
title: cargo
parent: 👋 Handlers
nav_order: 7
---

# cargo

The `cargo` handler manages packages via cargo (Rust crates).

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `package` / `name` | no | - | Package name as known to cargo. Can be string or array. |
| `state` | no | `present` | `present` / `absent` / `latest` |
