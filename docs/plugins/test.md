---
layout: default
title: 🧪 Plugin Testing
nav_order: 4
parent: 🧩 Plugins
---

# Isolated testing and profiling

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
