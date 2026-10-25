---
layout: default
title: apk
parent: 👋 Handlers
nav_order: 2
---

# apk

The `apk` handler manages packages via APK (Alpine Linux).

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `package` / `name` | no | - | Package name as known to APK. Can be string or array. |
| `state` | no | `present` | `present` / `absent` / `latest` |
