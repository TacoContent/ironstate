---
layout: default
title: assert
parent: 👋 Handlers
nav_order: 4
---

# assert

The `assert` handler checks assertions.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `that` | yes | - | Assertion condition(s) to check. |
| `fail_msg` | no | - | Message to display if assertion fails. |
| `success_msg` | no | - | Message to display if assertion succeeds. |
| `quiet` | no | `false` | Suppress messages. |
| `state` | no | `present` | `present` / `absent` / `latest` |
