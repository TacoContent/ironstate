---
layout: default
title: 📡 Protocol
nav_order: 5
parent: ⚙️ SDK
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 📡 Protocol
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
Plugins talk to ironstate over gRPC, using HashiCorp go-plugin as the transport. The Go SDK hides all of this. This page is for debugging, or for writing a plugin in another language.

- Protobuf package: `ironstate.plugin.v1`
- Source: [`sdk/proto/handler.proto`](https://github.com/TacoContent/ironstate/blob/develop/sdk/proto/handler.proto)
- Go package: `github.com/TacoContent/ironstate/sdk/proto` (imported as `pluginpb`)

## Handshake

| Setting | Value |
| --- | --- |
| Magic cookie | `IRONSTATE_PLUGIN=handler` |
| Protocol version | `1` (must match exactly) |
| Transport | gRPC only (no net/rpc) |
| Dispensed service | `handler` |

The host starts the binary with the cookie in its environment. The plugin replies with go-plugin's handshake line on stdout. The host then connects and calls `ListHandlers`.

## `HandlerPlugin` service

Implemented by the plugin.

| RPC | Request → Response | Description |
| --- | --- | --- |
| `ListHandlers` | `ListHandlersRequest` → `ListHandlersResponse` | Handler names, plus per-handler `HandlerMetadata` (`emoji`, `required_tools`, `fact_producer`), and `filter_names`. The emojis are also mirrored in `handler_emojis`. |
| `Test` | `HandlerRequest` → `TestResponse{satisfied}` | Whether the item is currently present. |
| `Describe` | `DescribeRequest{handler, action}` → `DescribeResponse{description}` | `action` is `"Install"` or `"Uninstall"`. |
| `Install` | `HandlerRequest` → `ExecResult` | Apply the item. |
| `Uninstall` | `HandlerRequest` → `ExecResult` | Remove the item. |
| `FactName` | `FactNameRequest` → `FactNameResponse{name, produced}` | Optional. Called only for handlers whose metadata sets `fact_producer: true`. See [`FactProducer`](./handler#factproducer). |
| `CallFilter` | `CallFilterRequest{name, value, args}` → `CallFilterResponse{result}` | Optional. Called only for names listed in `filter_names`. `value`, `args`, and `result` are `google.protobuf.Value`. See [`ServeWithConfig`](./plugin#servewithconfig). |
| `ScanRole` | `ScanRoleRequest` → `ScanRoleResponse{role, supported}` | Optional. |
| `Scan` | `ScanRequest` → `ScanResponse{items}` | Optional. Return `UNIMPLEMENTED` when unsupported. |

`HandlerRequest` carries `handler_name`, `item` (a `Struct`), `name`, and `context`. `Context` carries `flat` (a `Struct`), `apply`, and `become{enabled, user}`.

`fact_producer`, `filter_names`, and `CallFilter` were added within protocol version 1. A plugin built with an older SDK doesn't set them, and ironstate treats it as having no fact handlers and no filters.

## `HandlerHostCallback` service

Served **by the host**, for one call at a time, through go-plugin's connection broker:

| RPC | Description |
| --- | --- |
| `RenderTemplate` | Render a template with the given variables. |
| `EvaluateCondition` | Evaluate a condition. |
| `Log` | Print a message through ironstate's output. |

To reach it, read the numeric broker ID that the host puts in `context.flat["__ironstate_host_callback_id"]` and dial it with the broker. The ID is valid only until that call returns. If the key is missing, callbacks aren't available.

## Errors

- Return gRPC `NOT_FOUND` for an unknown `handler_name` or filter `name`.
- Any other gRPC error fails the task, and its message is shown to the user.
- A non-zero `ExecResult.rc` is a normal failed result, not an RPC error.

## Other languages

Any language with gRPC and a go-plugin-compatible handshake can implement a plugin: generate code from `handler.proto`, implement `HandlerPlugin`, and print the handshake line. go-plugin's [non-Go plugin guide](https://github.com/hashicorp/go-plugin/blob/main/docs/guide-plugin-write-non-go) describes the handshake. ironstate installs plugins with `go install`, so a non-Go plugin needs a Go module wrapper, or a manual install into the plugin cache, to be found.
