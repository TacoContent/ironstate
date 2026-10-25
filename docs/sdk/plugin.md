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

## `ServeWithConfig`

```go
type ServeConfig struct {
    Handlers map[string]handler.Handler
    Filters  map[string]handler.Filter
}

func ServeWithConfig(config ServeConfig)
```

Like `Serve`, but the plugin can also provide **filters**. `Serve(handlers)` is the same as `ServeWithConfig(ServeConfig{Handlers: handlers})`. A plugin can provide handlers, filters, or both.

```go
func main() {
    plugin.ServeWithConfig(plugin.ServeConfig{
        Handlers: map[string]handler.Handler{"entry": hostsEntry{}},
        Filters: map[string]handler.Filter{
            "fqdn": func(value any, args []any) (any, error) {
                domain := "local"
                if len(args) > 0 {
                    domain = fmt.Sprint(args[0])
                }
                return fmt.Sprintf("%v.%s", value, domain), nil
            },
        },
    })
}
```

In a playbook that declares `acme.hosts`, the filter above is available as `acme.hosts.fqdn`. It's also available as plain `fqdn`, unless a built-in filter, a [script filter](../filters/custom), or an earlier plugin already uses that name:

```yaml
vars:
  host_fqdn: ${{ facts.computer_name | acme.hosts.fqdn('example.com') }}
```

- Filter names must be identifiers: a letter or underscore, then letters, digits, or underscores. ironstate refuses to load a plugin that declares any other name.
- `value` and `args` arrive as JSON-like values: `nil`, `bool`, `float64` (every number), `string`, `[]any`, and `map[string]any`.
- Return any JSON-compatible value. Typed slices and maps (such as `[]string`) are converted for you.
- A returned error fails the expression with your message.
- Each call has a 30-second limit.

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
