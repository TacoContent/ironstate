---
render_with_liquid: false
layout: default
title: 📝 Handler Contract
nav_order: 3
parent: 🧩 Plugins
---

# Handler contract

Every handler implements these methods:

| Method | Purpose |
| --- | --- |
| `Test(item, name, ctx)` | Return whether the desired state is already satisfied. |
| `Describe(item, action, ctx)` | Return the dry-run or execution description shown by ironstate. |
| `Install(item, name, ctx)` | Apply the present state and return an `ExecResult`. |
| `Uninstall(item, name, ctx)` | Apply the absent state and return an `ExecResult`. |

`item` is the task's handler mapping. `name` is the resolved leaf name and may be empty. `Context.Flat` contains resolved facts, variables, inputs, package data, and prior task results. `Context.Apply` indicates whether the host is applying changes. `Context.Become` contains the requested elevation metadata. When the handler is called by a callback-capable host, `Context.Callbacks` provides the host's template and condition services:

```go
rendered, err := ctx.Callbacks.RenderTemplate("{{ hostname }}", ctx.Flat)
allowed, err := ctx.Callbacks.EvaluateCondition("enabled == true", ctx.Flat)
```

Callbacks are optional. Handlers should check `ctx.Callbacks != nil` before using them and return a clear error when a callback-dependent operation is requested without a callback-capable host. The host serves callbacks over the existing go-plugin broker connection; no extra port or listener is exposed to the user.

Use `ctx.Callbacks.Log("message")` for user-facing plugin output. Ironstate renders that message through its normal output policy; plugins should keep their own diagnostic logging in a file rather than writing to standard output or standard error.

`ExecResult` contains `RC`, `Stdout`, `StdoutLines`, `Stderr`, `StderrLines`, and optional JSON-compatible `Extra` data. A non-zero `RC` is treated as a failed task. Returning a Go error reports a handler/protocol failure and stops the task unless the playbook uses `continue_on_error`.

Optional capabilities:

- Implement `handler.EmojiProvider` to set the glyph used in CLI progress and result tables. A missing or empty value uses the default `🏷️` glyph.
- Implement `handler.RequiredToolsProvider` to declare zero or more executable names ironstate should require on `PATH` before dispatch. Without it, the handler owns its own availability checks.
- Implement `handler.FactProducer` to expose `ExecResult.Extra["value"]` as a named fact. Fact handlers run before other tasks, and their `Install` runs during dry runs too, so it must only read state. See [`FactProducer`](../sdk/handler#factproducer).
- Serve `handler.Filter` functions with `plugin.ServeWithConfig` to add filters to `${{ }}` expressions and `when:` conditions. See [`ServeWithConfig`](../sdk/plugin#servewithconfig).
- Implement `handler.ScanCapable` to participate in future/current scan workflows with `ScanRole` and `Scan`.
