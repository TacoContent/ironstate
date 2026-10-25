---

layout: default
title: 🏗️ Building A Plugin
nav_order: 5
parent: 🧩 Plugins
---

# Commands and versions

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
