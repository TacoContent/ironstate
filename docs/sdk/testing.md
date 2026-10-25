---
layout: default
title: 🧪 testing
nav_order: 3
parent: ⚙️ SDK
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🧪 testing
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
```go
import sdktest "github.com/TacoContent/ironstate/sdk/testing"
```

Package `testing` provides helpers for unit-testing handlers without ironstate. Its name clashes with Go's standard `testing` package, so **import it under an alias** such as `sdktest`.

## `Context`

```go
func Context() handler.Context
```

Returns an empty context: `Flat` is an empty map, `Apply` is `false`, `Become` is disabled, and `Callbacks` is `nil`. Fill in what your handler reads:

```go
ctx := sdktest.Context()
ctx.Apply = true
ctx.Flat["facts"] = map[string]any{"platform": "linux"}
```

## `Runner` and `RecordingRunner`

```go
type Runner interface {
    Run(exe string, args []string) (handler.ExecResult, error)
}

type RecordingRunner struct {
    Calls  []Command
    Result handler.ExecResult
    Err    error
}

type Command struct {
    Exe  string
    Args []string
}
```

`Runner` is a suggested seam for running commands. Have your handler depend on a `Runner`, and pass a `RecordingRunner` in tests. It records every call, with a copy of the args, and returns `Result` and `Err`.

```go
type serviceHandler struct {
    run sdktest.Runner
}

func (h serviceHandler) Install(item map[string]any, _ string, ctx handler.Context) (handler.ExecResult, error) {
    name, _ := item["name"].(string)
    exe, args, err := becomeexec.WrapForBecome(ctx.Become, "systemctl", []string{"enable", "--now", name})
    if err != nil {
        return handler.ExecResult{RC: 1}, err
    }
    return h.run.Run(exe, args)
}
```

```go
package main

import (
    "testing"

    sdktest "github.com/TacoContent/ironstate/sdk/testing"
)

func TestInstallEnablesService(t *testing.T) {
    runner := &sdktest.RecordingRunner{}
    h := serviceHandler{run: runner}

    if _, err := h.Install(map[string]any{"name": "sshd"}, "", sdktest.Context()); err != nil {
        t.Fatal(err)
    }
    if len(runner.Calls) != 1 || runner.Calls[0].Exe != "systemctl" {
        t.Fatalf("calls = %+v", runner.Calls)
    }
}
```

In production, implement `Runner` with `os/exec`. The SDK doesn't include one.

## `Profile`

```go
func Profile(path string, kind string, fn func() error) error
```

Runs `fn` and writes a profile to `path`:

| `kind` | Behavior |
| --- | --- |
| `cpu` | A CPU profile is recorded while `fn` runs. |
| `heap` | A heap profile is written after `fn` returns successfully. |

It returns `fn`'s error, or an error for an empty path, a nil `fn`, or an unknown kind.

```go
func TestProfileTest(t *testing.T) {
    err := sdktest.Profile("cpu.out", "cpu", func() error {
        for range 1000 {
            if _, err := (hostsHandler{}).Test(item, "", sdktest.Context()); err != nil {
                return err
            }
        }
        return nil
    })
    if err != nil {
        t.Fatal(err)
    }
}
```

```shell
go tool pprof -top cpu.out
```

## End-to-end tests

To test the real binary through the plugin protocol, install it and use [`ironstate plugin test`](../cli/commands/plugin.md) or `ironstate plugin bench`. See [Plugin Testing](../plugins/test.md).
