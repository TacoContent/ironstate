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

: Select result output format: **table** (the default), **json**, or **ndjson**. JSON results are written to standard output; operational messages are written to standard error. **ndjson** streams one JSON event per line (**leaf_start**, **log**, **leaf_result**, **facts**, **summary**, **error**, **done**) as the run progresses; with remote targets every event carries a **host** field and a final run-level **done** reports the overall exit code.

**-v**, **--verbose**

: Print additional information for skipped or already-satisfied tasks.

**--no-color**

: Disable colored terminal output.

**--allow-plugin-install**

: Allow plugins declared by the playbook to be installed automatically. This downloads and executes third-party code and is separate from **--apply**. With remote targets the option is forwarded to each agent.

**--allow-remote-uses**

: Pre-approve every non-isolated remote **uses:** source instead of prompting. Required for non-interactive runs that use untrusted remote sources.

# REMOTE APPLY OPTIONS

Giving **--target** or **--inventory** applies the playbook to other machines over SSH. The controller ships the ironstate agent and a bundle of the playbook to each target, runs it there, and streams results back. Facts, overlays, templates, and handlers all evaluate on the target.

**--target**=*[USER@]HOST[:PORT]*

: Add a target. The host may be an ssh config alias. **local** runs the agent on this machine without SSH. May be repeated.

**--inventory**=*FILE*

: Inventory of hosts and groups (see **inventory.schema.json**). Without **--limit** every inventory host is selected.

**--limit**=*NAME[,NAME...]*

: Select inventory hosts or groups; **all** selects every host.

**--forks**=*N*

: Number of hosts processed at once. Defaults to 5.

**--agent-binary**=*OS/ARCH=PATH*

: ironstate binary to ship for a target platform, e.g. **linux/arm64=./ironstate-linux-arm64**. May be repeated. Without it the controller ships itself for a matching platform, then its local agent cache, then (release builds only) a checksum-verified download of the matching release.

**--no-agent-download**

: Never download a release agent.

**--remote-agent-dir**=*DIR*

: Directory on targets for the cached agent binary.

**--skip-agent-verification**

: Skip target-side SHA-256 verification of the agent for trusted hosts without a hash utility. The agent is re-uploaded on every run.

**--ssh-config**=*FILE*

: ssh configuration file passed to **ssh -F**.

**--ssh-accept-new-host-keys**

: Accept and remember host keys of never-seen targets (**StrictHostKeyChecking=accept-new**). Host-key checking is otherwise governed by the user's ssh configuration and is never disabled.

**--forward-env**=*NAME*

: Forward a controller environment variable to the targets. May be repeated. **.env** and **.secrets** values are always forwarded in memory.

**--ask-become-pass**

: Prompt once for the targets' sudo password. It is sent only over the agent's standard input and redacted from all output.

**--host-timeout**=*DURATION*

: Maximum time for one host's whole operation, e.g. **5m**. 0 disables the limit.

**--remote-detach**

: With **--apply**, start each agent in the background and return its run ID; collect results later with **remote logs**. Refused when the run needs **.env**, **.secrets**, forwarded variables, or a become password.

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

## remote ping

Connect to the selected targets, probe their platform and sudo/elevation state, make sure the agent is present and verified, and run its **version** command. Nothing is applied. Accepts the target-selection, agent, and SSH options from REMOTE APPLY OPTIONS.

## remote logs HOST [RUN-ID]

Print the event log of a run kept on a target (the newest when *RUN-ID* is omitted). *HOST* is a target or, with **--inventory**, an inventory host name. Used to collect **--remote-detach** results and to inspect interrupted runs.

## remote clean

Remove cached agents, retained run logs, and staged detached jobs from the selected targets. Refused while an apply holds the target's lock; POSIX targets need the **flock** utility.

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

For remote targets on the controller's own OS and architecture, the controller copies each declared plugin it has installed into the target's plugin cache, SHA-256 verified. Other targets use their own installed plugins. Lockfile checksums are platform-specific.

Remote **uses:** sources are approved and cloned on the controller and shipped in the bundle; targets never run git. Only literal git **remote:** values can be pre-fetched.

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

**.ironstateignore**

: Glob patterns (one per line) excluded from the bundle sent to remote targets. **.git**, **.env**, and **.secrets** are always excluded.

**inventory.yml**

: Remote-apply inventory given with **--inventory**; see **inventory.schema.json** for its format.

**~/.cache/ironstate/runs/RUN-ID/events.ndjson**

: Per-run event log kept on each remote target (the last 20 runs; **%LOCALAPPDATA%\ironstate** on Windows, **~/Library/Caches/ironstate** on macOS).

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

**3**

: Remote apply only: one or more targets were unreachable or could not be prepared, and none failed during apply. Nothing was applied on those targets, so the run is safe to retry.

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

Apply to remote hosts from an inventory, four at a time:

    ironstate --playbook playbooks/site --apply --inventory inventory.yml --limit linux --forks 4

Check that a target is reachable and can run the agent:

    ironstate remote ping --target admin@build.lan

# SEE ALSO

**go**(1), **git**(1), **ssh**(1), **ssh_config**(5)

Project documentation: <https://github.com/TacoContent/ironstate>
