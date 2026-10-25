---
render_with_liquid: false
layout: default
title: 🧱 handler
nav_order: 1
parent: ⚙️ SDK
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🧱 handler
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
```go
import "github.com/TacoContent/ironstate/sdk/handler"
```

Package `handler` defines the contract every plugin handler implements.

## `Handler`

```go
type Handler interface {
    Test(item map[string]any, name string, ctx Context) (bool, error)
    Describe(item map[string]any, action Action, ctx Context) (string, error)
    Install(item map[string]any, name string, ctx Context) (ExecResult, error)
    Uninstall(item map[string]any, name string, ctx Context) (ExecResult, error)
}
```

| Parameter | Description |
| --- | --- |
| `item` | The task's handler mapping, with every `${{ }}` expression already resolved. For `acme.hosts.entry: { ip: 10.0.0.12 }`, `item` is `{"ip": "10.0.0.12"}`. |
| `name` | The resolved leaf name, which may be empty. `Describe` always receives an empty name. |
| `ctx` | Task-wide state. See [`Context`](#context). |

### How ironstate calls a handler

For each task, ironstate:

1. Calls **`Test`**. Return `true` if the item **currently exists or is applied** on the system, and `false` if it doesn't. `Test` runs in dry runs too, so it must **never change anything**.
2. Picks an [`Action`](#action) from the task's `state` and your answer:

   | `state` | `Test` returned `true` | `Test` returned `false` |
   | --- | --- | --- |
   | `present` (default), `installed` | `Skip` | `Install` |
   | `absent`, `removed` | `Uninstall` | `Skip` |
   | `latest`, `build-dep`, `fixed` | `Install` | `Install` |

   Any other `state` value is an error.
3. If the action isn't `Skip`, calls **`Describe`** for the line shown to the user, such as `› would install [acme.hosts.entry] <description>`.
4. Only with `--apply`, calls **`Install`** or **`Uninstall`**.

> [!IMPORTANT]
> `Test` answers "is it there?", not "is the desired state reached?". ironstate handles `state: absent` itself by calling `Uninstall` when `Test` returns `true`.

### Errors and return codes

| You return | Result |
| --- | --- |
| `ExecResult{RC: 0}`, `nil` | The task succeeded and is reported as changed. |
| `ExecResult{RC: n}` with `n != 0`, `nil` | The task failed. `Stderr` becomes the error output. |
| any result, non-nil `error` from `Install`/`Uninstall` | The task failed with `rc: 1` and your error message as `stderr`. |
| non-nil `error` from `Test`/`Describe` | The **whole run stops** with that error, and `continue_on_error` doesn't apply. Reserve errors for broken input; report "not there" as `false`. |

Failed results (not `Test`/`Describe` errors) follow the playbook's [`failed_when` and `continue_on_error`](../playbooks/results).

### Timeouts

| Calls | Timeout |
| --- | --- |
| `Test`, `Describe` (and `ListHandlers`, `ScanRole`) | 30 seconds |
| `Install`, `Uninstall`, `Scan` | 5 minutes |

## `Action`

```go
type Action string

const (
    ActionSkip      Action = "Skip"
    ActionInstall   Action = "Install"
    ActionUninstall Action = "Uninstall"
)
```

`Describe` receives `ActionInstall` or `ActionUninstall`.

## `Context`

```go
type Context struct {
    Flat      map[string]any
    Apply     bool
    Become    Become
    Callbacks HostCallbacks
}
```

| Field | Description |
| --- | --- |
| `Flat` | The resolved expression context for this task: `facts`, `vars`, `package`, `inputs`, `envs`, earlier `id` results, and so on. |
| `Apply` | `true` when ironstate runs with `--apply`. |
| `Become` | The elevation the task asked for. See [`Become`](#become). |
| `Callbacks` | Host services, or `nil`. See [`HostCallbacks`](#hostcallbacks). |

## `Become`

```go
type Become struct {
    Enabled bool
    User    string
}
```

ironstate **doesn't** elevate the plugin process. When `Enabled` is `true`, your handler must elevate the commands it runs itself. [`becomeexec.WrapForBecome`](./becomeexec) does this for Linux and macOS.

`Enabled` is already `false` when elevation isn't allowed: inside an isolated [`uses`](../playbooks/uses#isolation), or on a remote inventory host with `become: false`.

## `HostCallbacks`

```go
type HostCallbacks interface {
    RenderTemplate(template string, variables map[string]any) (string, error)
    EvaluateCondition(expression string, variables map[string]any) (bool, error)
    Log(message string) error
}
```

Callbacks run ironstate's own template engine, condition engine, and output, over the existing plugin connection. They're available during playbook runs and `ironstate plugin test`/`bench`, but `Callbacks` is **`nil`** during `ironstate init --scan`. Always check before using them:

```go
if ctx.Callbacks == nil {
    return handler.ExecResult{RC: 1}, errors.New("this operation needs a callback-capable ironstate host")
}
if err := ctx.Callbacks.Log("rewriting /etc/hosts"); err != nil {
    return handler.ExecResult{RC: 1}, err
}
```

| Method | Description |
| --- | --- |
| `RenderTemplate` | Renders a template with ironstate's engine and the given variables, such as `ctx.Flat`. |
| `EvaluateCondition` | Evaluates a `when`-style expression. |
| `Log` | Prints a message through ironstate's output. Use this rather than writing to stdout or stderr, which go-plugin captures. |

## `ExecResult`

```go
type ExecResult struct {
    RC          int
    Stdout      string
    StdoutLines []string
    Stderr      string
    StderrLines []string
    Extra       map[string]any
}
```

When a task registers itself with `id`, these fields are available as `<id>.rc`, `<id>.stdout`, `<id>.stdout_lines`, `<id>.stderr`, and `<id>.stderr_lines`. Each `Extra` key is added too, unless it clashes with those names or `changed`/`failed`:

```go
return handler.ExecResult{RC: 0, Extra: map[string]any{"entries": float64(3)}}, nil
```

```yaml
- id: hosts
  acme.hosts.entry: { path: /etc/hosts, ip: 10.0.0.12, hostname: build.local }
- log: { message: "entries: ${{ hosts.entries }}" }
```

## Data types

`item`, `Context.Flat`, and `Extra` travel as protobuf `Struct` values, so only JSON-like types survive:

- **Numbers arrive as `float64`**, even when the YAML says `port: 22`. Convert explicitly.
- Lists arrive as `[]any` and maps as `map[string]any`.
- `Extra` can only contain `nil`, `bool`, numbers, `string`, `[]byte`, `[]any`, and `map[string]any`. A `[]string` or `map[string]string` in `Extra` **fails the call** (`result extra: …`). Convert them first.

```go
port := 22
if raw, ok := item["port"].(float64); ok {
    port = int(raw)
}

names := []string{"a", "b"}
list := make([]any, len(names))
for i, n := range names {
    list[i] = n
}
result.Extra = map[string]any{"names": list}
```

## Optional interfaces

Implement these on your handler type to opt in.

### `EmojiProvider`

```go
type EmojiProvider interface { Emoji() string }
```

The glyph shown in progress lines and result tables. An empty value uses the default 🏷️.

### `RequiredToolsProvider`

```go
type RequiredToolsProvider interface { RequiredTools() []string }
```

Executables that must be on `PATH` before ironstate dispatches the handler, such as `[]string{"git"}`.

### `FactProducer`

```go
type FactProducer interface {
    FactName(item map[string]any) (name string, ok bool)
}
```

Declares that a handler's result should be stored as a fact named `name`, with the value taken from `ExecResult.Extra["value"]`.

> [!WARNING]
> The current ironstate host doesn't call `FactName` for plugin handlers, so plugin facts aren't stored yet. Until it does, have users register the task with `id` and read your `Extra` keys from the result instead (see [`ExecResult`](#execresult)).

### `ScanCapable`

```go
type ScanCapable interface {
    ScanRole() string
    Scan(ctx Context) ([]ScanItem, error)
}

type ScanItem struct {
    Module string
    Name   string
    Config map[string]any
    Tags   []string
    Role   string
}
```

Lets your handler contribute to [`ironstate init --scan`](../cli/commands/init), which generates a playbook from the current system. Every installed plugin, at its latest version, is scanned.

| Field / method | Description |
| --- | --- |
| `ScanRole()` | The role folder the items are written to, such as `roles/system/hosts`. Return `""` to opt out. |
| `Scan(ctx)` | Returns the discovered items. `ctx` is empty, and `Callbacks` is `nil`. |
| `Module` | Leave it empty. ironstate fills in the qualified handler name, such as `acme.hosts.entry`. |
| `Name` | Used in the generated task name, `Ensure <role> <Name>`. Items without a name are dropped. |
| `Config` | The generated task's handler mapping. If it's empty, ironstate writes `state: present`. |
| `Tags` | Tags for the generated task. |
| `Role` | Not sent over the protocol. The role always comes from `ScanRole()`. |

The generated `main.yml` gets a `plugins:` entry for your plugin automatically.
