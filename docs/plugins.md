# External Handler Plugins

External handlers are standalone Go programs that ironstate launches as subprocesses over a versioned gRPC protocol. A plugin is installed into ironstate's per-user cache and can then be used by playbooks without changing ironstate source code.

## Plugin naming

Publish the module using this convention:

```text
github.com/<organization>/ironstate-handler-<name>
```

The playbook namespace is `<organization>.<name>`. A plugin can expose multiple handler names, so a task uses the fully qualified module key:

```yaml
plugins:
  - use: acme.hosts@v1.2.3

tasks:
  - name: Ensure the build host resolves locally
    acme.hosts.entry:
      ip: 10.0.0.12
      hostname: build.local
```

The plugin identity and the handler name are separate. `acme.hosts` identifies the installed binary; `entry` identifies one handler served by that binary.

## Create a plugin

Create a normal Go module and add the SDK:

```shell
mkdir ironstate-handler-hosts
cd ironstate-handler-hosts
go mod init github.com/acme/ironstate-handler-hosts
go get github.com/TacoContent/ironstate/sdk@latest
```

A minimal plugin has a handler implementation and a `main` package:

```go
package main

import (
    "fmt"
    "os"
    "strings"

    "github.com/TacoContent/ironstate/sdk/handler"
    "github.com/TacoContent/ironstate/sdk/plugin"
)

type hostsHandler struct{}

func (hostsHandler) Emoji() string { return "📇" }

func (hostsHandler) Test(item map[string]any, _ string, _ handler.Context) (bool, error) {
    path, entry, err := spec(item)
    if err != nil {
        return false, err
    }
    contents, err := os.ReadFile(path)
    if os.IsNotExist(err) {
        return false, nil
    }
    if err != nil {
        return false, err
    }
    return strings.Contains(string(contents), entry), nil
}

func (hostsHandler) Describe(item map[string]any, action handler.Action, _ handler.Context) (string, error) {
    _, entry, err := spec(item)
    if err != nil {
        return "", err
    }
    return fmt.Sprintf("%s hosts entry %s", strings.ToLower(string(action)), entry), nil
}

func (hostsHandler) Install(item map[string]any, _ string, _ handler.Context) (handler.ExecResult, error) {
    path, entry, err := spec(item)
    if err != nil {
        return handler.ExecResult{RC: 1}, err
    }
    file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
    if err != nil {
        return handler.ExecResult{RC: 1}, err
    }
    defer file.Close()
    if _, err := fmt.Fprintln(file, entry); err != nil {
        return handler.ExecResult{RC: 1}, err
    }
    return handler.ExecResult{RC: 0}, nil
}

func (hostsHandler) Uninstall(item map[string]any, _ string, _ handler.Context) (handler.ExecResult, error) {
    return handler.ExecResult{RC: 0}, nil
}

func spec(item map[string]any) (string, string, error) {
    path, _ := item["path"].(string)
    ip, _ := item["ip"].(string)
    hostname, _ := item["hostname"].(string)
    if path == "" || ip == "" || hostname == "" {
        return "", "", fmt.Errorf("path, ip, and hostname are required")
    }
    return path, ip + " " + hostname, nil
}

func main() {
    plugin.Serve(map[string]handler.Handler{
        "entry": hostsHandler{},
    })
}
```

The example is intentionally small. A production handler should make `Install` and `Uninstall` idempotent, validate all user-controlled paths and arguments, preserve unrelated file content, and return useful `ExecResult` fields. The plugin process runs with the privileges of ironstate; it is not a sandbox.

## Handler contract

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

`ExecResult` contains `RC`, `Stdout`, `StdoutLines`, `Stderr`, `StderrLines`, and optional JSON-compatible `Extra` data. A non-zero `RC` is treated as a failed task. Returning a Go error reports a handler/protocol failure and stops the task unless the playbook uses `continue_on_error`.

Optional capabilities:

- Implement `handler.EmojiProvider` to set the glyph used in CLI progress and result tables. A missing or empty value uses the default `🏷️` glyph.
- Implement `handler.FactProducer` to expose `ExecResult.Extra["value"]` as a named fact.
- Implement `handler.ScanCapable` to participate in future/current scan workflows with `ScanRole` and `Scan`.

## Commands and versions

Build and test the plugin independently of an installed ironstate binary:

```shell
go test ./...
go build -o ironstate-handler-hosts .
```

Install a published plugin from its module path:

```shell
ironstate plugin install acme.hosts@v1.2.3
ironstate plugin install acme.hosts@latest
```

`latest` resolves to the greatest installed semantic version at run time. Exact versions can coexist in the cache. Use a lockfile next to the playbook when reproducibility matters; the lockfile pins the version and checksum used by the playbook.

Useful lifecycle commands:

```shell
ironstate plugin list
ironstate plugin info acme.hosts
ironstate plugin update acme.hosts
ironstate plugin update --all
ironstate plugin uninstall acme.hosts@v1.2.3
```

Installation runs `go install github.com/<organization>/ironstate-handler-<name>@<version>`, starts the resulting binary, performs the protocol handshake, and records a manifest. The manifest is stored below the platform user cache directory at `ironstate/plugins/<organization>/<name>/<version>/`.

A playbook does not silently fetch plugin code. If a declared plugin is missing, the run reports the exact `ironstate plugin install ...` command. CI or another explicitly trusted automation context can opt in with:

```shell
ironstate --playbook main.yml --allow-plugin-install
```

The separate flag is deliberate: installing a plugin downloads and executes arbitrary third-party Go code, which is a different trust boundary from `--apply` and OS package-manager operations.

## Isolated testing and profiling

Run a handler without a full playbook. The item accepts YAML or JSON:

```shell
ironstate plugin test acme.hosts \
  --handler entry \
  --item '{"path":"/tmp/hosts","ip":"10.0.0.12","hostname":"build.local"}'

ironstate plugin test acme.hosts \
  --handler entry \
  --item 'path: /tmp/hosts\nip: 10.0.0.12\nhostname: build.local' \
  --apply
```

Benchmark one operation over repeated calls. The plugin process is launched once for the benchmark and closed when it finishes:

```shell
ironstate plugin bench acme.hosts \
  --handler entry \
  --item '{"path":"/tmp/hosts","ip":"10.0.0.12","hostname":"build.local"}' \
  --operation test \
  --iterations 100
```

Supported operations are `test`, `describe`, `install`, and `uninstall`. The result is JSON with total, average, minimum, and maximum timings in nanoseconds and milliseconds. Only benchmark mutating operations with `--apply` when changing the target system is intentional.

Plugin unit tests can use the public helpers without importing ironstate internals:

```go
func TestHandler(t *testing.T) {
    ctx := testing.Context()
    runner := &testing.RecordingRunner{}
    _ = ctx
    _ = runner
}
```

For opt-in diagnostics, `sdk/testing.Profile(path, "cpu", fn)` writes a CPU profile while `fn` runs; `sdk/testing.Profile(path, "heap", fn)` writes a heap profile after it returns. Analyze the result with the Go toolchain's `pprof` commands.

## Playbook diagnostics

Validate plugin declarations and qualified handler names before applying a playbook:

```shell
ironstate doctor --playbook path/to/main.yml
```

Doctor checks that declared plugins are installed, their manifests/checksums resolve, the binaries launch and handshake, and every qualified handler used by the playbook is declared by the selected plugin. Errors include the install or update command that addresses the problem.

A playbook declaration requires an exact `organization.plugin@version` value:

```yaml
plugins:
  - use: acme.hosts@v1.2.3
```

The lockfile takes precedence for version selection. A checksum mismatch is a hard error; ironstate does not silently fall back to another installed version.

## Protocol and compatibility

Plugins communicate over HashiCorp go-plugin's gRPC transport. The SDK centralizes the magic-cookie handshake and protocol version. Protocol version compatibility is exact in v1: a host and plugin with different protocol versions do not run together. Upgrade the plugin or ironstate as indicated by the error.

The plugin API is process-isolated from the host, but native plugin code is not sandboxed. A plugin can access the filesystem, network, environment, and any privileges granted to its process. Review plugin source and pin versions/checksums before using community plugins in automation.

## Release checklist

1. Keep the module path `github.com/<organization>/ironstate-handler-<name>` stable.
2. Tag releases with Go module-compatible semantic versions such as `v1.2.3`.
3. Run `go test ./...` and build the plugin for the supported target platforms.
4. Publish documentation showing each handler's item fields, states, examples, external commands, and required privileges.
5. Record protocol/API compatibility in release notes when the SDK dependency changes.
