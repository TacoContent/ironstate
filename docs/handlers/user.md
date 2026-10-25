---
layout: default
title: user
parent: 👋 Handlers
nav_order: 44
---

# user

The `user` handler manages local users on Windows, Linux, and macOS.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `name` | yes | - | User name |
| `password` | no | - | Password value (platform behavior varies) |
| `password_env` | no | - | Environment variable name containing password |
| `uid` | no | - | User id on Unix/macOS |
| `group` / `gid` | no | - | Primary group (name or gid) |
| `groups` | no | - | Supplementary groups |
| `shell` | no | - | Login shell |
| `home` | no | - | Home directory |
| `comment` | no | - | Display/full name/comment |
| `system` | no | `false` | Create as a system account where supported |
| `create_home` | no | `true` | Linux: create home directory |
| `remove_home` | no | `false` | Linux: remove home directory on delete |
| `state` | no | `present` | `present` / `absent` / `latest` |

## Platform behavior

- Windows: uses `net user` and `net localgroup`.
- Linux: uses `id`, `useradd`, and `userdel`.
- macOS: uses `dscl`, `sysadminctl`, and `dseditgroup`.
