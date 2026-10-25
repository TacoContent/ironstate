---
layout: default
title: fact
parent: 👋 Handlers
nav_order: 14
---

# fact

The `fact` handler sets facts/variables.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `name` | yes | - | Name of the fact to set. |
| `value` | no | - | Value to set. |
| `shell` | no | - | Shell command to run to generate the value. |
| `state` | no | `present` | `present` / `absent` / `latest` |
