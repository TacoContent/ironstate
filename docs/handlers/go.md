---
layout: default
title: go
parent: 👋 Handlers
nav_order: 20
---

# go

The `go` handler installs Go binaries with `go install`.

## Fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `package` | yes | - | Go module path, e.g. `github.com/dhth/act3`. |
| `version` | no | `latest` | Version passed as `package@version` to `go install`. |
| `state` | no | `present` | `present` / `absent` / `latest` |
