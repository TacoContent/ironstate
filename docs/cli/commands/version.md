---
layout: default
title: 📝 Version
nav_order: 9
parent: 🖥️ Commands
grand_parent: 🖥️ CLI
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 📝 Version
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
Prints the ironstate version, the commit it was built from, and the build date.

## Usage

```shell
ironstate version
```

```text
$ ironstate version
ironstate v1.4.0 (commit 3f2a9c1, built 2026-05-01T12:00:00Z)
```

Builds made with `go install` or `go build` don't receive release build metadata, so they report:

```text
ironstate dev (commit none, built unknown)
```

They work the same otherwise. When reporting a bug, include this output. See [Support](../../support.md).

> [!NOTE]
> The remote-apply controller ships its own binary to targets of the same OS and architecture. For other platforms, it downloads the release that matches this version. A `dev` build can't download one, so pass `--agent-binary` instead. See [Flags › Remote apply](../flags.md#remote-apply).
