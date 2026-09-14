# ironstate-handler-hosts

A real sample external handler plugin for ironstate. It manages an idempotent `IP hostname` mapping in the local hosts file.

The sample exposes the `` handler, so its fully qualified playbook key is `acme.hosts.entry` when installed as `acme.hosts`.

## Development

This directory is an independent Go module, just like a plugin hosted in its own repository. The local `replace` directive in `go.mod` points at this checkout's SDK so the sample can be tested while the SDK is changing.

```shell
go test ./...
go build -o ironstate-handler-hosts .
```

The plugin binary is served with `sdk/plugin.Serve` and is compatible with the repository's `internal/pluginhost` loader. It can be launched directly by `ironstate plugin install` after publishing the module under its final module path.

## Handler item

```yaml
plugins:
  - use: acme.hosts@v0.1.0

tasks:
  - name: Ensure the build host resolves locally
    acme.hosts.entry:
      path: /etc/hosts
      ip: 10.0.0.12
      hostname: build.local
```

`path` is optional. It defaults to `/etc/hosts` on Unix-like systems and `%SystemRoot%\\System32\\drivers\\etc\\hosts` on Windows. `ip` and `hostname` are required.

`Install` removes existing entries for the hostname before adding the requested mapping, preserving comments and unrelated entries. `Uninstall` removes the requested IP/hostname mapping. Both operations are idempotent. `become: true` uses the SDK's `sudo` wrapper and re-enters the plugin for the privileged file write.

The plugin's internal privileged re-entry uses `--ironstate-plugin-become <destination> <temporary-file>`. This flag is not a playbook option, is only recognized by the plugin executable itself, and is used exclusively for `become` writes. It does not need a plugin-specific name because the elevated process is launched from the same plugin binary; the generic convention is shared by external plugins that use this pattern.

The handler also implements `FactProducer`, returning the resolved mapping under the task's `name`, and `ScanCapable`, which discovers entries from the default or `hosts_path` scan context.

## Host acceptance

From the repository root, build and run the production loader acceptance test:

```shell
go test ./internal/pluginhost -run TestSampleHostsPluginRoundTrip -count=1
```
