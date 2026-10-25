---
layout: default
title: 🔌 plugin
nav_order: 2
parent: ⚙️ SDK
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🔌 plugin
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
```go
import "github.com/TacoContent/ironstate/sdk/plugin"
```

Package `plugin` turns a set of handlers into a runnable plugin binary.

## `Serve`

```go
func Serve(handlers map[string]handler.Handler)
```

Starts the plugin and **never returns**. Call it from `main`:

```go
func main() {
    plugin.Serve(map[string]handler.Handler{
        "entry":   hostsEntry{},
        "comment": hostsComment{},
    })
}
```

- Each map key is a **handler name**. A plugin installed as `acme.hosts` exposes these keys as `acme.hosts.entry` and `acme.hosts.comment`.
- Handler names can't be empty.
- A call for a name that isn't in the map fails with gRPC `NotFound`.
- Errors returned by your handler are sent as gRPC `Internal` errors carrying your message.

The binary is meant to be launched by ironstate. Running it by hand prints go-plugin's "This binary is a plugin" message and exits.

### Output

go-plugin owns the process's stdout and stderr. Don't print to them for user-facing output; use [`ctx.Callbacks.Log`](./handler#hostcallbacks). Write your own diagnostics to a log file.

## Constants

| Name | Value | Description |
| --- | --- | --- |
| `ProtocolVersion` | `1` | The plugin protocol version. Host and plugin must match **exactly**. |
| `HandlerServiceName` | `"handler"` | The go-plugin service name dispensed by the host. |
| `HostCallbackIDKey` | `"__ironstate_host_callback_id"` | Internal. The host adds it to `Context.Flat` to route callbacks, and `Serve` removes it before your handler sees the context. |

The handshake uses the magic cookie `IRONSTATE_PLUGIN=handler`.

## Host-side helpers

These are for **hosts** (ironstate itself, or a test harness) that launch plugins. Plugin authors don't need them.

| Function / type | Description |
| --- | --- |
| `HandshakeConfig()` | The shared go-plugin `HandshakeConfig`. |
| `ClientPluginSet()` | The plugin set a go-plugin client needs to dispense the handler service. |
| `BrokeredHandlerClient` | A `pluginpb.HandlerPluginClient` that also exposes `CallbackBroker()`, used to serve `HandlerHostCallback` for a call. |

A minimal host:

```go
client := goplugin.NewClient(&goplugin.ClientConfig{
    HandshakeConfig:  plugin.HandshakeConfig(),
    VersionedPlugins: map[int]goplugin.PluginSet{plugin.ProtocolVersion: plugin.ClientPluginSet()},
    Cmd:              exec.Command("./ironstate-handler-hosts"),
    AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
})
defer client.Kill()

rpc, err := client.Client()
if err != nil {
    return err
}
raw, err := rpc.Dispense(plugin.HandlerServiceName)
if err != nil {
    return err
}
handlers := raw.(pluginpb.HandlerPluginClient)
list, err := handlers.ListHandlers(ctx, &pluginpb.ListHandlersRequest{})
```

## Versioning

`ProtocolVersion` changes only when the wire protocol changes incompatibly. When it does, a plugin built against the old SDK won't load in the new ironstate, and the reverse. The error tells the user to upgrade one side. Within one protocol version, new SDK releases stay wire-compatible.
