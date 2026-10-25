---
layout: default
title: 🔑 becomeexec
nav_order: 4
parent: ⚙️ SDK
---

<!-- markdownlint-disable-next-line MD025 MD022 -->
# 🔑 becomeexec
{: .no_toc }

<!-- markdownlint-disable-next-line MD022 -->
## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---
```go
import "github.com/TacoContent/ironstate/sdk/becomeexec"
```

ironstate doesn't elevate plugin processes. When a task sets `become`, the request reaches your handler as `ctx.Become`, and it's up to you to run commands elevated. `becomeexec` does this the same way ironstate's built-in handlers do.

## `WrapForBecome`

```go
func WrapForBecome(become handler.Become, executable string, args []string) (string, []string, error)
```

| `become` | Returns |
| --- | --- |
| `Enabled: false` | `executable` and `args` unchanged. |
| `Enabled: true`, `User` empty or `root` | `sudo`'s full path, and `[executable, args...]`. |
| `Enabled: true`, `User: "deploy"` | `sudo`'s full path, and `["-u", "deploy", executable, args...]`. |
| `Enabled: true`, no `sudo` on `PATH` | An error: `become requested but 'sudo' was not found on PATH`. |

```go
func (h aptRepo) Install(item map[string]any, _ string, ctx handler.Context) (handler.ExecResult, error) {
    exe, args, err := becomeexec.WrapForBecome(ctx.Become, "apt-get", []string{"update"})
    if err != nil {
        return handler.ExecResult{RC: 1}, err
    }
    out, err := exec.Command(exe, args...).CombinedOutput()
    rc := 0
    if exitErr, ok := err.(*exec.ExitError); ok {
        rc = exitErr.ExitCode()
    } else if err != nil {
        return handler.ExecResult{RC: 1}, err
    }
    return handler.ExecResult{RC: rc, Stdout: string(out)}, nil
}
```

## Notes

- `sudo` runs **non-interactively** from the plugin's point of view. Users need passwordless sudo, or a cached sudo session, for elevated tasks.
- **Windows**: ironstate treats `become` as a no-op on Windows, where users run from an elevated session. `WrapForBecome` only knows about `sudo`, so skip it there:

  ```go
  if runtime.GOOS != "windows" {
      exe, args, err = becomeexec.WrapForBecome(ctx.Become, exe, args)
  }
  ```

- `ctx.Become.Enabled` is already `false` inside an isolated `uses` and on remote inventory hosts with `become: false`.
