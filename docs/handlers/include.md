---
layout: default
title: include
parent: 👋 Handlers
nav_order: 23
---

# include

The `include` handler pulls in packages/\<name\>/main.yml's task list as if written inline in place of this action (like `actions:`, but sourced from a file). None of `state`/`tags`/`with` here are applied to the included tasks automatically - the package opts in by referencing `${{ package.state }}`, `${{ package.tags }}`, or `${{ inputs.<key> }}` on its own fields.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `name` | yes | - | Package directory name under install/windows/packages/. |
| `state` | no | `present` | Made available inside the package as `${{ package.state }}`. |
| `tags` | no | - | Made available inside the package as `${{ package.tags }}`. |
| `with` | no | - | Arbitrary key/value inputs made available inside the package as `${{ inputs.<key> }}` (dotted paths work for nested values). |
