---
layout: default
title: ⚙️ SDK
nav_order: 9
has_children: true
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# ⚙️ SDK
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
The ironstate SDK is a standalone Go module for building **external handler plugins**. A plugin is a normal Go program. ironstate launches it as a subprocess and calls it over gRPC.

```shell
go get github.com/TacoContent/ironstate/sdk@latest
```

The SDK is its own module, so plugins don't depend on ironstate's internal packages. For a walkthrough, start with [Create A Plugin](../plugins/crea). These pages are the API reference.

## Packages

| Package | Import path | Purpose |
| --- | --- | --- |
| [`handler`](./handler) | `github.com/TacoContent/ironstate/sdk/handler` | The `Handler` interface, `Context`, `ExecResult`, and optional capability interfaces. |
| [`plugin`](./plugin) | `github.com/TacoContent/ironstate/sdk/plugin` | `Serve`, which runs your handlers as a plugin, plus handshake constants. |
| [`testing`](./testing) | `github.com/TacoContent/ironstate/sdk/testing` | Test doubles and a profiling helper. |
| [`becomeexec`](./becomeexec) | `github.com/TacoContent/ironstate/sdk/becomeexec` | Wraps commands with `sudo` when a task requests `become`. |
| [`proto`](./protocol) | `github.com/TacoContent/ironstate/sdk/proto` | Generated gRPC types for the `ironstate.plugin.v1` protocol. |

## Requirements

- Go **1.27.3** or newer. This is the SDK module's `go` directive.
- The SDK uses HashiCorp [go-plugin](https://github.com/hashicorp/go-plugin) v1.8 over gRPC. You don't need to use go-plugin directly; `plugin.Serve` handles it.

## How a plugin runs

```text
ironstate (host)                          your plugin binary
────────────────                          ──────────────────
launch binary  ─── handshake (cookie + protocol v1) ───▶ plugin.Serve(...)
ListHandlers   ───────────────────────────────────────▶ names, emojis, required tools
for each task using acme.hosts.entry:
  Test         ───────────────────────────────────────▶ handlers["entry"].Test
  Describe     ───────────────────────────────────────▶ handlers["entry"].Describe   (only if not skipped)
  Install /    ───────────────────────────────────────▶ handlers["entry"].Install    (only with --apply)
  Uninstall
               ◀────── RenderTemplate / EvaluateCondition / Log (optional callbacks)
end of run     ─── kill process
```

- ironstate starts **one process per declared plugin** for each run, and stops it when the run ends.
- A plugin can serve **several handlers**. The playbook addresses each one as `<organization>.<plugin>.<handler>`, such as `acme.hosts.entry`.
- The plugin runs with the same privileges as ironstate. It is process-isolated but **not sandboxed**.

## Related

- [Plugins](../plugins/index): naming, installation, diagnostics, and remote apply.
- [Using A Plugin](../plugins/using): declaring plugins in a playbook.
- [`ironstate plugin`](../cli/commands/plugin): install, update, test, and benchmark plugins.
