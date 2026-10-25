---
layout: default
title: mount_facts
parent: 👋 Handlers
nav_order: 28
---

# mount_facts

The `mount_facts` handler gathers mount point information as facts.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `state` | no | `present` | `present` / `absent` / `latest` |

This handler produces facts (implements `FactProducer`).
