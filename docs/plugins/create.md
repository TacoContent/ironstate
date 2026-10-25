---
layout: default
title: ✨ Create A Plugin
parent: 🧩 Plugins
nav_order: 1
---

## Create A Plugin

Create a normal Go module and add the SDK:

```shell
mkdir ironstate-handler-hosts
cd ironstate-handler-hosts
go mod init github.com/acme/ironstate-handler-hosts
go get github.com/TacoContent/ironstate/sdk@latest
```

A minimal plugin has a handler implementation and a `main` package at the module root:

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