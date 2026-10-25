---
layout: default
title: yum
parent: 👋 Handlers
nav_order: 49
---

# yum

The `yum` handler manages packages via YUM/DNF (RHEL/CentOS/Fedora).

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `package` / `name` | no | - | Package name as known to YUM. Can be string or array. |
| `state` | no | `present` | `present` / `absent` / `latest` |
