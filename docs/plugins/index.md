---
layout: default
title: 🧩 Plugins
nav_order: 8
has_children: true
---

# External Handler Plugins

External handlers are standalone Go programs that ironstate launches as subprocesses over a versioned gRPC protocol. A plugin is installed into ironstate's per-user cache and can then be used by playbooks without changing ironstate source code.

## Plugin naming

Publish the module using this convention:

```text
github.com/<organization>/ironstate-handler-<name>
```

Add `ironstate-handler` and `ironstate` topic to your plugin repository to make it discoverable on GitHub.

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

## Plugins with remote apply

Playbooks that declare plugins can be applied to remote targets (`--target`/`--inventory`, see [Inventory](../playbooks/inventory)). Handlers always run on the target, so the target needs the plugin binary for its own OS/arch:

- **Same OS/arch as the controller:** before the run, the controller copies each declared plugin it has installed (binary and `manifest.json`, at the lockfile's version when one exists) into the target's plugin cache. Each file is SHA-256 verified on the target and re-uploaded only when it changes, and the copied binary is byte-identical to the controller's, so `ironstate.lock.yaml` checksums still match. Shipped plugins appear in the host's status line (`plugins=...`) and as `plugins_shipped` in JSON output.
- **Different OS/arch:** the target uses plugins already installed there (`ironstate plugin install` on the target). `--allow-plugin-install` is forwarded, so the agent may install a missing plugin itself, which needs Go on the target.

Lockfile checksums describe one platform's binary. A lockfile written on one OS/arch only verifies on targets of that same platform; for mixed fleets, either keep the lockfile on the platform you apply to, or install the plugin on each target platform and accept that the lockfile will reject the other platforms' binaries.

## Protocol and compatibility

Plugins communicate over HashiCorp go-plugin's gRPC transport. The SDK centralizes the magic-cookie handshake and protocol version. Protocol version compatibility is exact in v1: a host and plugin with different protocol versions do not run together. Upgrade the plugin or ironstate as indicated by the error.

The plugin API is process-isolated from the host, but native plugin code is not sandboxed. A plugin can access the filesystem, network, environment, and any privileges granted to its process. Review plugin source and pin versions/checksums before using community plugins in automation.

## Release checklist

1. Keep the module path `github.com/<organization>/ironstate-handler-<name>` stable.
2. Tag releases with Go module-compatible semantic versions such as `v1.2.3`.
3. Run `go test ./...` and build the plugin for the supported target platforms.
4. Publish documentation showing each handler's item fields, states, examples, external commands, and required privileges.
5. Record protocol/API compatibility in release notes when the SDK dependency changes.
