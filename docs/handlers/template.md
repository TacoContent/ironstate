---
layout: default
title: template
parent: 👋 Handlers
nav_order: 42
---

# template

The `template` handler renders `src` through the specified template `engine` and writes the result to `dest`.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `src` | yes | - | Template source file. Resolved relative to the install system directory (install/windows, or the owning package's directory for packages/<name>/main.yml) unless absolute, `~`-prefixed, or a URL. |
| `dest` | yes | - | Destination file path for the rendered output. `~` expansion is supported. |
| `engine` | yes | - | Template engine: `jinja`, `jinja2`, or `gotemplate`. `jinja` is the legacy expression-language subset. `jinja2` uses Gonja (Jinja2-compatible). `gotemplate` uses Go text/template syntax. |
| `vars` | no | - | Extra key/value pairs layered on top of facts/vars/registry for this render only (last-write-wins, shallow merge). |
| `owner` | no | - | Ownership metadata applied to `dest` after write. |
| `group` | no | - | Group metadata applied to `dest` after write. |
| `mode` | no | - | Mode bits applied to `dest` after write (e.g. `0644` or `420`). |
| `state` | no | `present` | `present` / `absent` / `latest` |
