---
title: ironstate
section: 1
header: Declarative task runner
footer: TacoContent
author: TacoContent
version: 0.2.0
---
<!-- markdownlint-disable MD025 MD036 -->

# NAME

ironstate - declarative, Ansible-style task runner driven by YAML

# SYNOPSIS

**ironstate** [*OPTIONS*] [*COMMAND*]

**ironstate** [*OPTIONS*]

# DESCRIPTION

**ironstate** reconciles desired system state declared in YAML playbooks. It evaluates each task, reports what would change by default, and applies changes only when **--apply** is specified.

The program is distributed as a single binary for Windows, Linux, and macOS. A playbook may contain built-in handlers and external handlers supplied by installed Go plugins.

# OPTIONS

**--playbook**=*PATH*

: Path to the site or main YAML document. The value may be an existing file, a playbook directory, or a bare name. Directories are searched for **site.yml**, **site.yaml**, **main.yml**, and **main.yaml**. Defaults to the current directory.

**--vars-file**=*PATH*

: Merge an additional variables document after the automatic host and platform overlays. May be repeated; later files take precedence.

**--var**=*KEY=VALUE*

: Override a variable by dotted path. May be repeated and has the highest variable precedence.

**--apply**

: Apply changes. Without this option, ironstate performs a dry run except for read-only fact gathering and validation operations.

**--tags**=*TAG[,TAG...]*

: Restrict processing to tasks or actions carrying one of the specified tags.

**--output**=*FORMAT*

: Select result output format: **table** (the default) or **json**. JSON results are written to standard output; operational messages are written to standard error.

**-v**, **--verbose**

: Print additional information for skipped or already-satisfied tasks.

**--no-color**

: Disable colored terminal output.

**--allow-plugin-install**

: Allow plugins declared by the playbook to be installed automatically. This downloads and executes third-party code and is separate from **--apply**.

**-h**, **--help**

: Show command help.

# COMMANDS

## version

Show the ironstate version, commit, and build date.

## init [PLAYBOOK-NAME]

Create a minimal playbook directory structure. Existing files and directories are not overwritten.

**--scan**

: Populate the generated playbook from handlers that support system scanning.

## filters list

List built-in and discovered script filters.

**--dir**=*PATH*

: Directory to scan for external script filters. Defaults to **filters**.

## doctor

Check command availability, discovered script filters, and optionally validate plugins used by a playbook.

**--filters-dir**=*PATH*

: Directory to scan for external script filters. Defaults to **filters**.

**--playbook**=*PATH*

: Validate declared plugins and qualified handler names in the specified playbook.

## plugin

Manage external handler plugins. Plugins are installed in a per-user cache and communicate with ironstate over a versioned gRPC protocol.

### plugin install ORGANIZATION.PLUGIN[@VERSION]

Install a plugin from its Go module. The module convention is:

    github.com/ORGANIZATION/ironstate-handler-NAME

If no version is supplied, **latest** is used.

### plugin list

List installed plugins, versions, handlers, and summaries.

### plugin info ORGANIZATION.PLUGIN

Display installed plugin manifests, including source, checksum, license, protocol version, and handlers.

### plugin update ORGANIZATION.PLUGIN[@VERSION]

Install an updated version while retaining older cached versions. Use **--all** to update every installed plugin.

### plugin uninstall ORGANIZATION.PLUGIN[@VERSION]

Remove one installed plugin version. Without a version, remove all installed versions of the plugin.

### plugin test ORGANIZATION.PLUGIN

Invoke a plugin handler without loading a full playbook.

**--handler**=*NAME*

: Handler name to invoke.

**--item**=*YAML-OR-JSON*

: Handler item as a YAML or JSON mapping.

**--apply**

: Run the install or uninstall operation after testing.

### plugin bench ORGANIZATION.PLUGIN

Benchmark a plugin handler operation and emit JSON timing statistics.

**--handler**=*NAME*

: Handler name to benchmark.

**--item**=*YAML-OR-JSON*

: Handler item as a YAML or JSON mapping.

**--operation**=*OPERATION*

: Operation to benchmark: **test**, **describe**, **install**, or **uninstall**. Defaults to **test**.

**--iterations**=*N*

: Number of operation calls. Defaults to 10.

**--apply**

: Permit mutating install or uninstall operations.

# PLAYBOOK FORMAT

A playbook is a YAML document containing variables and tasks:

    vars:
      editor: vim

    tasks:
      - name: Ensure a package is installed
        apt:
          name: git
          state: present

Tasks may be placed directly in the main document or loaded through the playbook hierarchy. The hierarchy supports **hosts/**, **variables/**, **roles/**, **packages/**, and **tasks/** directories. Host and variable overlays may be selected using hostname, operating system family, platform, and architecture facts.

A playbook may declare external plugins:

    plugins:
      - use: acme.hosts@v1.2.3

    tasks:
      - name: Ensure a hosts entry
        acme.hosts.entry:
          path: /etc/hosts
          ip: 10.0.0.12
          hostname: build.local

A missing plugin is reported with the installation command. Automatic installation requires **--allow-plugin-install**. An optional **ironstate.lock.yaml** file pins plugin versions and checksums.

Built-in handlers are available under their normal names and under the namespaced aliases **ironstate.builtin.NAME**.

# FILES

**ironstate.yaml**, **.ironstate.yaml**

: Optional configuration file in the current working directory. Command-line flags take precedence.

**.env**, **.secrets**

: Optional environment files loaded from the current working directory. Secret values from **.secrets** are masked in later output.

**ironstate.lock.yaml**

: Optional playbook lockfile containing plugin versions and checksums.

**$XDG_CACHE_HOME/ironstate/plugins/**

: Plugin cache on systems following the XDG convention. On Windows, the platform user cache directory is used.

# OUTPUT

Table output is intended for interactive use. JSON output is intended for automation:

    ironstate --playbook site.yml --output json | jq

Each JSON result includes the module, package, state, action, apply status, failure status, execution result, and **duration_ms** timing.

Messages, progress, warnings, and plugin logs are written to standard error so that standard output remains machine-readable when JSON output is selected.

# EXIT STATUS

**0**

: The run completed without an unhandled task failure.

**1**

: The run stopped because of an unhandled task failure or dispatch error.

**2**

: The playbook could not be loaded or parsed.

# ENVIRONMENT

**NO_COLOR**, **IRONSTATE_NO_COLOR**

: Disable colored output.

**IRONSTATE_***

: Configuration values may be supplied through environment variables using the corresponding configuration key with the **IRONSTATE_** prefix.

# EXAMPLES

Dry run a playbook:

    ironstate --playbook playbooks/site

Apply changes:

    ironstate --playbook playbooks/site --apply

Apply only selected tags:

    ironstate --playbook playbooks/site --tags security,packages --apply

Use variable overrides:

    ironstate --playbook site.yml --vars-file ci.yml --var editor=vim --var ssh.port=2222

Validate a playbook's plugins:

    ironstate doctor --playbook site.yml

Install and test an external handler:

    ironstate plugin install acme.hosts@v1.2.3
    ironstate plugin test acme.hosts --handler entry --item '{"path":"/tmp/hosts","ip":"10.0.0.12","hostname":"build.local"}'

# SEE ALSO

**go**(1), **git**(1)

Project documentation: <https://github.com/TacoContent/ironstate>
