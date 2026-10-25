---
layout: default
title: ✍️ Using A Plugin
nav_order: 2
parent: 🧩 Plugins
---

# Using A Plugin

To use a plugin in your playbook, first declare it under the `plugins` key with the desired version. Then, reference the handler provided by the plugin in your tasks.

For example:

```yaml
plugins:
  - use: acme.hosts@v1.2.3

tasks:
  - name: Ensure the build host resolves locally
    acme.hosts.entry:
      ip: 10.0.0.12
      hostname: build.local
```
