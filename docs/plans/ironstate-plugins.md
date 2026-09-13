# External handler plugins

Status: IN PROGRESS (Phases 0-1 and 3-5 complete; Phase 2 host callbacks remain)
Owner: unassigned
Target: unscheduled — phased rollout, see §14

## 0. Summary

ironstate's ~40 builtin handlers (`internal/handlers/handlers.go`) are compiled into the
binary and dispatched by a string-keyed registry (`engine.Handler` map) built in
`handlers.All()`. This plan adds a second, external source of handlers: standalone Go
binaries, one per plugin, published under the naming convention
`github.com/<org>/ironstate-handler-<name>`, installed via `go install ...@<version>`,
and invoked by ironstate as a subprocess over a versioned RPC protocol built on
[`hashicorp/go-plugin`](https://github.com/hashicorp/go-plugin) — the same mechanism
Terraform, Vault, Packer, and Nomad use for exactly this problem (a Go host, third-party
Go plugin binaries, cross-platform including Windows, no CGO).

Plugin-provided handlers are referenced in playbooks as `<org>.<name>.<handler>:`,
exactly like a builtin (`shell:`, `apt:`), so once a plugin is installed it needs no
further wiring. Builtins gain a parallel `ironstate.builtin.<name>` alias for symmetry,
with existing bare names (`shell:`) kept working indefinitely.

This supersedes the informal brain-dump this file previously contained (preserved
verbatim in Appendix A for traceability). The prior draft used "should be able to"
language for a very large surface (dependency auto-install, license enforcement,
monitoring dashboards, execution visualization). This plan keeps the core promise —
discover, install, invoke, test, document — and explicitly scopes down or defers the
parts that don't hold up under "how would this actually get built and who would
maintain it," with reasoning given in each place so the tradeoff is visible rather than
silently dropped.

## 1. Goals

1. A third party can write a handler in Go, publish it as
   `github.com/<org>/ironstate-handler-<name>`, and a user can install and use it in a
   playbook without ironstate's source ever changing.
2. Plugin handlers implement the **same conceptual interface** builtins do
   (`Test`/`Describe`/`Install`/`Uninstall`, optional `FactProducer`/`ScanCapable`), so
   there is one mental model for "how do I write a handler," not two.
3. `<org>.<name>` namespacing prevents collisions between unrelated plugins and between
   plugins and builtins.
4. Installing, listing, updating, uninstalling, and inspecting plugins is a first-class
   `ironstate plugin ...` CLI surface.
5. `ironstate doctor` and playbook loading can tell a user *why* a task will fail
   because of a plugin problem (not installed, wrong version, protocol mismatch,
   handler name not found) and what command fixes it.
6. A plugin author can write, unit-test, and debug a handler entirely with `go test`,
   with no dependency on ironstate itself being installed, then verify it end-to-end
   against a real ironstate host.
7. A real, working sample plugin exists and is used as the acceptance test for the
   whole pipeline, not just a toy.

## 2. Non-goals (and why)

These were present in the original brain-dump. Each is either deferred to a later,
clearly-labeled phase, or intentionally cut, with reasoning — flag any of these in
review if the reasoning doesn't hold up:

- **Automatic dependency installation** ("handlers should define dependencies which
  ironstate installs automatically if missing"). Silently installing arbitrary
  third-party software on a user's machine because a plugin's manifest asked for it is
  a supply-chain risk disproportionate to the convenience. Scoped down to: a plugin can
  *declare* required external commands (for `doctor` to check and for
  `NoCommandCheckModules`/`ModuleCommandNames`-equivalent PATH gating, mirroring what
  builtins already do), but ironstate never installs them on the plugin's behalf. A
  plugin's own `Install` method remains free to shell out to a package manager the way
  `apt.go`/`winget.go` do today — that's the author's call, made visibly, not
  ironstate's call made invisibly.
- **License enforcement** ("ironstate should respect [handlers'] licensing terms when
  distributing them"). ironstate never distributes plugin binaries — it invokes `go
  install` against the plugin's own source repository, so the plugin's license already
  governs that transaction directly between the user and `go install`/the module proxy.
  A plugin manifest carries a free-text `license` field for **display only** (`plugin
  list`, `plugin info`), with no enforcement logic. Building real license-compliance
  tooling (SPDX parsing, policy gates) is out of scope unless a concrete need shows up.
- **Handler-defined custom versioning schemes**. The plugin *protocol* (the RPC
  contract) is versioned with a single integer negotiated at handshake (see §6); the
  plugin *release* is versioned with whatever the author's Go module tags use (in
  practice, semver, because `go install ...@vX.Y.Z` requires it). There is no room for
  a handler to invent its own scheme without breaking `go install` compatibility, so
  this requirement is already satisfied by "use Go modules like everything else."
- **Monitoring dashboards / execution visualization** ("graphs and charts that show
  [handler] behavior and performance over time"). This is a full observability product
  bolted onto a CLI automation tool. Scoped down (Phase 6) to a machine-readable JSON
  timing report per leaf (extending the existing `--output json` path) that a user can
  feed into whatever charting tool they already have. If real demand shows up for a
  built-in dashboard, that's a separate proposal, not a subsection of this one.
- **Sandboxing / capability restriction of plugin code.** `go install`-distributed
  native binaries run with ironstate's own privileges (and can further elevate via
  `become`), same as builtins do today via `os/exec`. WASM-based sandboxing was
  considered and rejected for v1: it's fundamentally incompatible with "author writes
  normal Go, ships via `go install`" without a second compile target and toolchain,
  which would double the plugin-author burden for a benefit (sandboxing) that builtins
  themselves don't have either. Documented as a known risk in §13, not solved here.

## 3. Current system, as of this writing

(Established by direct inspection of the code, not assumed — cited so the design below
can be checked against it.)

- `engine.Handler` (`internal/engine/engine.go`) is the interface every handler
  implements:
  ```go
  type Handler interface {
      Test(item map[string]any, name string, ctx Context) (bool, error)
      Describe(item map[string]any, action Action, ctx Context) (string, error)
      Install(item map[string]any, name string, ctx Context) (ExecResult, error)
      Uninstall(item map[string]any, name string, ctx Context) (ExecResult, error)
  }
  ```
  Two optional interfaces a handler may also satisfy: `FactProducer` (contributes a
  named fact from its result) and `ScanCapable` (participates in `ironstate init
  --scan`).
- `handlers.All()` returns a plain `map[string]engine.Handler` literal (~40 entries);
  `handlers.AllModuleNames` is the parallel `[]string` of recognized module keys.
- `tasks.Expand(taskList, tasks.Options{ModuleNames: ...})` (`internal/tasks/tasks.go`)
  flattens a playbook's task tree into `[]tasks.Leaf{Module, Item, Name, Tags, When,
  ID, ...}`, deciding a leaf's `Module` by matching its map keys against
  `ModuleNames`.
- `engine.Run`/`RunLeaves` (`internal/engine/engine.go`) dispatches each leaf by
  looking up `opts.Handlers[leaf.Module]`; an unregistered module **warns and skips**,
  it does not hard-fail the run.
- Both are wired together in `internal/cli/root.go`'s `runApply`:
  `tasks.Expand(..., tasks.Options{ModuleNames: handlers.AllModuleNames, ...})` then
  `engine.Run(leaves, engine.Options{Handlers: handlers.All(), ...})`. **This is the
  seam this plan extends** — both the module-name list used at flatten time and the
  handler map used at dispatch time need external entries merged in alongside the
  builtins.
- `internal/cli/doctor.go` today is a static PATH-check list of package-manager
  binaries plus discovered script filters; it has no handler-registry awareness at
  all.
- There is no existing dynamic-loading, RPC, or plugin mechanism for *handlers*. The
  closest precedent in the repo is `internal/filters` (script filters): a per-language
  embedded shim subprocess speaking one-line JSON-over-stdio, kept warm as a persistent
  worker, discovered by scanning a directory for `<name><ext>` files. That precedent
  informed the "long-lived subprocess, versioned handshake, structured RPC" shape of
  §6 below, but is not reused verbatim: filters are *interpreted scripts* discovered by
  directory scan, whereas handler plugins are *compiled, `go install`-distributed
  binaries* discovered by an explicit install step — different enough constraints that
  hand-rolling the same JSON-line protocol would mean re-solving handshake/versioning/
  crash-recovery that `go-plugin` already solves.
- No existing dependency on `hashicorp/go-plugin`, gRPC, native Go `plugin`, or WASM.

## 4. Naming and namespacing

- Plugin module path: `github.com/<org>/ironstate-handler-<name>` (org = GitHub
  org/user, name = short plugin name, e.g. `github.com/acme/ironstate-handler-hosts`).
- Plugin identity inside ironstate: `<org>.<name>` (e.g. `acme.hosts`) — this is what
  `ironstate plugin install/update/uninstall/list` and the playbook `plugins:` block
  use as the key.
- A single plugin binary may expose **one or more handler names** (analogous to a
  single Ansible collection shipping several modules). Each is addressed in a playbook
  as `<org>.<name>.<handler>:`, e.g. `acme.hosts.ensure_entry:`. This matches the
  example given in the original request (`<org>.<name>.my_custom_handler:`) and avoids
  forcing a 1-binary-1-handler split that would otherwise multiply the number of
  separate Go modules a plugin author has to publish and version together.
- Builtins get a namespaced alias `ironstate.builtin.<name>` (e.g.
  `ironstate.builtin.shell`) registered **in addition to** the existing bare name
  (`shell:`). Bare names are not deprecated — this is purely additive, so no existing
  playbook changes. Implementation: extend the map literal in `handlers.All()` and the
  `AllModuleNames` slice with the aliased forms pointing at the same `engine.Handler`
  values; zero changes to individual handler implementations.

## 5. Wire protocol: hashicorp/go-plugin over gRPC

Chosen over (a) hand-rolled JSON-over-stdio and (b) raw gRPC without go-plugin, because
`go-plugin` already provides, battle-tested, exactly the pieces a hand-rolled protocol
would otherwise have to reinvent: magic-cookie handshake (so a plugin binary refuses to
run standalone and prints a clear error instead of hanging), protocol-version
negotiation, subprocess lifecycle management (start, health-check, graceful `Kill`),
and log-line forwarding from the plugin's stderr into the host's own logging — all
cross-platform including Windows named pipes. Using it also means ironstate's plugin
story looks and feels like Terraform/Vault/Packer's to anyone who's used those, which
is a real (if soft) benefit for adoption.

Sketch of the service contract (`sdk/proto/handler.proto`, illustrative, not final):

```protobuf
syntax = "proto3";
package ironstate.plugin.v1;
import "google/protobuf/struct.proto";

service HandlerPlugin {
  rpc ListHandlers(ListHandlersRequest) returns (ListHandlersResponse);
  rpc Test(TestRequest) returns (TestResponse);
  rpc Describe(DescribeRequest) returns (DescribeResponse);
  rpc Install(InstallRequest) returns (ExecResult);
  rpc Uninstall(UninstallRequest) returns (ExecResult);
  rpc FactName(FactNameRequest) returns (FactNameResponse);   // optional capability
  rpc ScanRole(ScanRoleRequest) returns (ScanRoleResponse);    // optional capability
  rpc Scan(ScanRequest) returns (ScanResponse);                // optional capability
}

message ExecResult {
  int32 rc = 1;
  string stdout = 2;
  repeated string stdout_lines = 3;
  string stderr = 4;
  repeated string stderr_lines = 5;
  google.protobuf.Struct extra = 6;
}

// Implemented by the HOST, called BACK by the plugin over go-plugin's
// GRPCBroker (bidirectional channel) — see the Context.Filters discussion below.
service HandlerHostCallback {
  rpc RenderTemplate(RenderTemplateRequest) returns (RenderTemplateResponse);
  rpc EvaluateCondition(EvaluateConditionRequest) returns (EvaluateConditionResponse);
}
```

Every request carries `handler_name` (which of the plugin's declared handlers this call
targets) plus `item` (the task's fields, as `google.protobuf.Struct`) and a `Context`
message (flattened vars as `Struct`, `apply bool`, a `Become` descriptor as plain data
— username/method, not a live object).

**Design decision — a host callback service, not "fully-rendered items" (revised after
review; see §15/Appendix B).** An earlier draft of this plan assumed builtin handlers
mostly read concrete, already-rendered values and that a plugin protocol could avoid
live filter access entirely by rendering everything before dispatch. That assumption
does not survive contact with the actual code: `assert.go` calls
`conditions.TestCondition(s, ctx.Flat, ctx.Filters)` on its `that:` expression at
`Install`-time; `template.go` calls `templateengines.RenderJinja` using `ctx.Filters`;
`lineinfile.go`, `mountfacts.go`, `waitfor.go`, and `async.go` all thread live
`ctx.Filters` into their own runtime evaluation. Dynamic expression/condition
evaluation isn't an edge case for a handful of obscure handlers — it's core to some of
the most-used ones. A plugin protocol that can't support an `assert`-like or
`template`-like plugin would silently exclude an entire class of handler, which is not
an acceptable scope cut to make implicitly.

Instead: `go-plugin`'s gRPC transport supports bidirectional calls via its
`GRPCBroker` (the same mechanism Terraform uses for provisioners to stream UI output
back to the host mid-call). The host exposes a small callback service —
`RenderTemplate(template string, vars Struct) -> string` and
`EvaluateCondition(expr string, vars Struct) -> bool` — backed by the *same*
`expr.Filters`/`templateengines` implementations builtins already use; a plugin never
receives Go closures (still impossible across a process boundary), it sends a string
expression and gets back a value, on demand, as many times as its `Install`/`Test`
logic needs. `item`/`Context.Flat` are still passed pre-flattened as before — the
callback only exists for the subset of plugins that need dynamic evaluation, so a
simple plugin (like the §13 sample) never has to touch it. This keeps the "no reentrancy
for the common case" benefit while not lying about what the harder case actually needs.

`Become` is passed as **data only** (method + username), matching how CLI-backed
builtins already do their own privilege escalation inside `runExternalCommand` — the
plugin process, not the host, is responsible for actually invoking `sudo`/elevation
when it shells out, using an SDK helper that mirrors `internal/exec/become.go`'s
behavior (see §7). The host does not need to elevate the plugin subprocess itself.

## 6. Public SDK module

**Critical constraint, not optional:** everything under `internal/` in this module is
compiler-enforced unimportable from another Go module (`github.com/<org>/ironstate-
handler-<name>` cannot `import ".../ironstate/internal/engine"` — Go's `internal/`
visibility rule blocks it at the import-path level, full stop). A plugin-facing SDK
**must** live outside `internal/`.

Proposed: a second Go module in this repository, `sdk/` (`github.com/TacoContent/
ironstate/sdk`, own `go.mod`), tied to the main module via a `go.work` workspace file
during development so the SDK and the host stay in lockstep with the protocol version
while both are actively changing; can be split into its own repository later without
breaking existing plugin authors, since its import path wouldn't need to change if kept
as a sub-module path. `go.work` is dev-only and irrelevant to external plugin authors —
their own `go.mod` resolves `github.com/TacoContent/ironstate/sdk` through the module
proxy exactly like any other dependency, `go.work` or not.

**Release requirement, easy to get wrong once:** a nested module only resolves via `go
install`/the module proxy if it's tagged with its own path prefix — `sdk/v1.2.3`, not
a bare `v1.2.3` (which would tag the *root* module). Every release process/CI step that
tags this repo must remember to double-tag (`vX.Y.Z` for the root module,
`sdk/vX.Y.Z` for the SDK) or every external plugin author's `go get` breaks silently.
Call this out explicitly in the release checklist added in Phase 1, not left as tribal
knowledge.

Packages:

- `sdk/handler` — the public interface plugin authors implement. Deliberately mirrors
  `internal/engine.Handler` field-for-field (`Test`/`Describe`/`Install`/`Uninstall`,
  optional `FactProducer`/`ScanCapable`) so there is exactly one interface shape to
  learn, whether you're reading `internal/handlers/shell.go` as a reference or writing
  a new plugin. Also defines the public `Context`, `ExecResult`, `Action` types (plain
  structs — no gRPC/protobuf types leak into this package, so a plugin author can unit
  test against it with nothing but `go test`).
- `sdk/plugin` — `func Serve(handlers map[string]handler.Handler)` — the one line a
  plugin's `main()` calls. Wraps `go-plugin`'s `plugin.Serve` with ironstate's
  handshake config (magic cookie, protocol version) and a gRPC server that adapts
  `sdk/handler.Handler` calls to the wire messages in §5.
- `sdk/becomeexec` — a small helper mirroring `internal/exec/become.go`'s
  elevation-wrapping behavior, so a plugin's `Install`/`Uninstall` that shells out gets
  the same `become` semantics builtins get, without duplicating that logic ad hoc per
  plugin. This is a thin delegation wrapper, not a reimplementation — it must reuse
  whatever `internal/exec/become.go` already does per platform (its Windows-vs-POSIX
  handling, whatever that is today) rather than inventing new elevation behavior for
  plugins alone. Flagged as needing an explicit read of that file's current behavior
  during Phase 1 implementation, not assumed here.
- `sdk/testing` — fake `Context` builder and a fake command-`Runner` (mirroring
  `internal/exec.Runner`'s shape) so a plugin's tests never need a real subprocess or a
  real ironstate host, matching this repo's own existing handler-test conventions
  (`internal/handlers/handlers_test.go`'s `testCtx()`/`recordingRunner` pattern).

A plugin's `main.go` ends up close to:

```go
func main() {
    plugin.Serve(map[string]handler.Handler{
        "ensure_entry": hostsHandler{},
    })
}
```

## 7. Host-side loading

New package `internal/pluginhost`:

- **Install-time**: resolves and runs `go install github.com/<org>/ironstate-handler-
  <name>@<version>`, locates the resulting binary (via `go env GOBIN`/`GOPATH/bin`),
  copies it into ironstate's own versioned plugin store, and performs a one-shot
  handshake (launch, `ListHandlers`, `Kill`) to capture the plugin's declared handler
  names, protocol version, and manifest metadata (doc summary, license string) into an
  install-time manifest file. This also means installation itself proves the plugin
  starts and speaks the protocol, catching a broken plugin at install time instead of
  first use.
- **Run-time**: for each installed plugin actually referenced by a playbook's
  `plugins:` block, launches (or reuses, if already running this session) a `go-
  plugin` client, and wraps the resulting gRPC stub in an adapter type implementing
  `internal/engine.Handler` — one adapter instance per declared handler name, so from
  `engine.Run`'s point of view an external handler is indistinguishable from a builtin.
  Adapters carry the `handler_name` to send on every call, so one running plugin
  process backs N registry entries.
- **Registry merge**: `handlers.All()`/`AllModuleNames` become inputs to a new
  `handlers.Registry` type that merges builtins with whatever `pluginhost.Load(...)`
  returns for the plugins a given playbook declares, before being handed to
  `tasks.Expand`/`engine.Run` in `runApply`. Builtins remain compiled in and require no
  subprocess; only plugin-backed entries pay the subprocess/RPC cost.
- **Lifecycle**: all launched plugin processes are `Kill()`ed on run completion (normal
  or error exit) via the existing cleanup path in `root.go`'s `runApply`; a call
  timeout (configurable, sane default e.g. 5 min for `Install`, shorter for `Test`)
  guards against a hung plugin blocking a whole run.
- **Host callback registration**: when launching a plugin, the host registers its
  `HandlerHostCallback` implementation on the same `go-plugin` connection via
  `GRPCBroker`, so a plugin's `Install`/`Test` call can dial back for
  `RenderTemplate`/`EvaluateCondition` mid-call (see §5). No new listener/port is
  opened — this rides the existing plugin↔host connection.
- **Cost, stated plainly, not left implicit**: ironstate is not a daemon — every CLI
  invocation is a fresh process, so every plugin referenced by a playbook pays a full
  launch + magic-cookie handshake + gRPC dial on **every single `ironstate` run**
  (interactive, cron, CI), not just once ever. There is no cross-invocation warm pool;
  reuse in §7 only means "don't relaunch the same plugin twice for two leaves within
  one run." This mirrors the cost Terraform/Vault/Packer accept for the same
  architecture, but a user should not discover it by surprise — it's the direct
  tradeoff of choosing process isolation and `go install`-based distribution over
  in-process builtins, and is one reason builtins remain the right choice for
  hot/frequently-invoked functionality.
- **Protocol version mismatch policy**: the handshake's protocol version is a single
  integer, checked for **exact equality** between host and plugin — no partial/forward
  compatibility in v1 (the surface is too new to justify the complexity of a
  compatibility matrix). A mismatch is a hard failure with a message naming both
  versions and pointing at whichever side is behind (`ironstate plugin update
  <org>.<name>` if the plugin is older; "upgrade ironstate" if the host is older). This
  same check is what `doctor` (§11) surfaces proactively before a run.

## 8. Storage layout

```
<user cache dir>/ironstate/plugins/<org>/<name>/<version>/
    ironstate-handler-<name>[.exe]
    manifest.json     # org, name, version, source module, checksum, installed_at,
                       # handler_names[], protocol_version, license, doc_summary
```

Reuses the existing `internal/pathutil` package for platform-correct base directories
(`%LOCALAPPDATA%` on Windows via `os.UserCacheDir()`, XDG-style on Linux/macOS).
Multiple versions of the same plugin can coexist side by side; `manifest.json` per
version means `plugin list`/`plugin info` never has to re-launch a plugin just to
describe it.

An optional **per-playbook-tree lockfile** (`ironstate.lock.yaml`, sitting next to
`main.yml` the way `hosts/`/`variables/` already do) pins the exact resolved
version + checksum for reproducibility across machines/CI, analogous to `go.sum`. If
present, `plugins:` resolution is checked against it rather than "whatever's newest
installed." Precedence when a per-version `manifest.json` and `ironstate.lock.yaml`
disagree (e.g. the lockfile pins a version that's no longer installed, or an installed
version's checksum no longer matches its manifest): the lockfile always wins for *which
version to run*, and a checksum mismatch against the manifest is a hard failure (not a
silent fall-through to "whatever's installed") — the whole point of a lockfile is that
a mismatch is exactly the thing it exists to catch.

## 9. Playbook syntax

```yaml
plugins:
  - use: acme.hosts@latest
  - use: acme.dns@v1.2.3
```

```yaml
- name: pin a hostname to the build server
  acme.hosts.ensure_entry:
    ip: 10.0.0.12
    hostname: build.local
```

**Design decision — explicit install, not silent install, and why this isn't just
`--apply` again.** Running a playbook whose `plugins:` block names a plugin that isn't
installed **fails fast** with the exact `ironstate plugin install acme.hosts@latest`
command to fix it, or — with a new, explicitly-named `--allow-plugin-install` flag for
scripted/CI use — installs it automatically and proceeds.

This was reviewed against an objection worth recording: today, `--apply` is already the
one gate ironstate has for "do something real" (without it, everything is a dry run),
and existing package-manager handlers (`pacman.go`, `yum.go`) already pass
`--noconfirm`/`-y` unconditionally once `--apply` is set — i.e. ironstate already lets a
playbook trigger unattended OS package installation under nothing but `--apply`. So why
does a plugin need a *second*, stricter gate instead of just also honoring `--apply`?
The distinction kept: `apt`/`pacman`/`yum`/`winget` install from repositories the user's
OS already vets and signs (a trust boundary that predates and is independent of
ironstate). `go install github.com/<org>/ironstate-handler-<name>` fetches and *runs*
unreviewed source code from wherever `<org>` happens to point on GitHub — a location
that, in the `plugins:` case, was named by whoever wrote the playbook, who may not be
the person running it. That is a materially different trust boundary, so it gets its
own, separately-named, opt-in flag rather than silently riding along on `--apply`.
"No additional configuration" in the original request is read here as *no code changes
or registry wiring required to use an already-installed plugin*, not as *arbitrary code
gets fetched and executed with zero action from whoever's running the playbook*.

## 10. `ironstate plugin` CLI surface

| Command | Behavior |
|---|---|
| `ironstate plugin install <org>.<name>[@version\|@latest]` | Resolves + installs per §7/§8. |
| `ironstate plugin list` | Table of installed plugins: identity, version, handler names, one-line doc summary (from manifest). |
| `ironstate plugin info <org>.<name>` | Full manifest: source module, version, checksum, license, per-handler doc. |
| `ironstate plugin update <org>.<name>[@version] \| --all` | Re-runs install for a newer version; old version kept until confirmed working (rollback = reinstall old version, still on disk). |
| `ironstate plugin uninstall <org>.<name>[@version]` | Removes a version (or all versions) from the store. |
| `ironstate plugin test <org>.<name> --handler <name> --item <yaml\|json> [--apply]` | Loads the plugin standalone (no playbook) and invokes `Test`/`Describe`/`Install` directly, printing the `ExecResult` — isolated testing without a full run. |

## 11. `doctor` integration

Extends `internal/cli/doctor.go` (today a static PATH-check list) with, for the
playbook(s) it's pointed at:

- For every plugin in `plugins:`: installed? which version resolves? does it still
  launch and handshake (protocol version compatible with this ironstate build)?
- For every `<org>.<name>.<handler>:` module key actually used in the playbook: does
  it resolve to a handler the installed plugin actually declares (catches a typo'd
  handler name or a version that dropped a handler)?
- Failure output names the exact fix (`ironstate plugin install ...` /
  `ironstate plugin update ...`), matching the existing doctor style of "tell the user
  the one command that fixes this."

## 12. Testing strategy

- **Plugin authors** never need ironstate installed to develop: `sdk/handler.Handler`
  is a plain Go interface, `sdk/testing` supplies fakes, so `go test ./...` in a
  plugin's own repo covers `Test`/`Install`/`Uninstall`/`Describe` logic exactly like
  `internal/handlers/*_test.go` does for builtins today (table-driven, fake `Runner`,
  `testCtx()`-style helper).
- **SDK package** (`sdk/...`): unit tests for the type-conversion layer between public
  `handler.*` types and the generated proto messages.
- **`internal/pluginhost`**: tests against a real, tiny compiled fixture plugin
  (checked into `internal/pluginhost/testdata/`, built as part of `go test` via
  `go build` in a `TestMain`) — proves the actual subprocess/RPC round trip, not just
  mocked interfaces, the same way `internal/handlers/realfixture_test.go` validates
  against real playbook fixtures today.
- **End-to-end acceptance**: the sample plugin from §13, installed and run through a
  real playbook in CI, is the acceptance test for the whole feature — install, list,
  run, doctor, update, uninstall, in sequence.

## 13. Sample real-world plugin: `ironstate-handler-hosts`

Manages OS hosts-file entries (`/etc/hosts`, `C:\Windows\System32\drivers\etc\hosts`) —
add/remove/ensure an `IP → hostname` mapping. Chosen because it's genuinely useful
(pinning a hostname to a dev box or test server is a real task), needs no secrets or
external API to demonstrate, is naturally cross-platform (exercises Windows path
handling deliberately, since that's a real constraint for this project per its
goreleaser targets), and exercises the full interface surface:

- `Test` — is this exact mapping already present?
- `Describe` — human-readable "would add/remove `10.0.0.12 build.local`" preview.
- `Install`/`Uninstall` — idempotent line add/remove in the hosts file, with `become`
  support (the hosts file needs elevated permissions on both platforms).
- `FactProducer` — exposes the resolved hostname→IP mapping as a fact for later tasks.
- `ScanCapable` — lets `ironstate init --scan` seed a playbook from existing hosts-file
  entries.

Developed in-repo initially under `examples/ironstate-handler-hosts/` (its own
`go.mod`, so it's built and tested exactly as an external plugin would be, not
special-cased), with its own README, `docs/` examples, and full test suite. Once the
pipeline is proven, it's a natural candidate to actually publish as a real, separate,
tagged repository — proving `go install github.com/<org>/ironstate-handler-hosts@v0.1.0`
end to end, not just in a monorepo subdirectory.

## 14. Phased implementation

| Phase | Scope | Key files/dirs touched |
|---|---|---|
| 0 — Spike | Complete: validated a `go-plugin` gRPC round-trip (launch, handshake, one RPC call) on Windows before committing to the design above. The retained contract test lives in `internal/pluginhost`. | `internal/pluginhost/transport_spike_test.go` |
| 1 — SDK & protocol | Complete: the public `handler`, `plugin`, `becomeexec`, and `testing` SDK packages are implemented; the versioned `.proto` has generated Go/gRPC stubs; protocol version and handshake configuration are centralized in `sdk/plugin`. A real subprocess gRPC acceptance test validates `Serve`, handler discovery, dispatch, and structured results. | `sdk/**`, `go.work` |
| 2 — Host loader & registry merge | In progress: implemented the gRPC adapter, process launch/handshake/handler discovery, qualified handler naming, explicit client cleanup, `handlers.Registry`, builtin `ironstate.builtin.<name>` aliases, JSON `ExecResult.Extra` preservation, and apply-time loading/cleanup integration. Remaining: host callback brokering. | `internal/pluginhost/**`, `internal/handlers/handlers.go`, `internal/cli/root.go`, `internal/engine/output.go` |
| 3 — Install/CLI & playbook syntax | Complete: `plugins: - use: organization.plugin@version` parsing; versioned user-cache manifest store; checksum lockfile; `plugin install/list/info/update/uninstall` commands; exact/latest resolution before task expansion; explicit `--allow-plugin-install`; and lifecycle-safe registry loading. | `internal/cli/plugin.go`, `internal/model/**`, `internal/pluginhost/store.go`, `internal/pluginhost/lock.go` |
| 4 — `doctor` integration | Complete: `doctor --playbook <path>` loads the supplied hierarchy, resolves lock-pinned or declared plugin versions, verifies lockfile checksums, launches each plugin to validate its handshake, and reports missing/undeclared qualified handlers with install/update commands. | `internal/cli/doctor.go`, `internal/cli/doctor_test.go` |
| 5 — Isolated testing/debug | Complete: `ironstate plugin test <org>.<name> --handler <name> --item <yaml\|json> [--apply]` resolves the latest installed version, validates its declared handler, runs `Test`/`Describe`, conditionally runs `Install` or `Uninstall` from `state`, and emits a structured `ExecResult`. Plugin subprocess logs are forwarded through go-plugin's trace-level `hclog` bridge. | `internal/cli/plugin.go`, `internal/pluginhost/loader.go` |
| 6 — Bench & timing report (scoped down from "profile/monitor/visualize", see §2) | `ironstate plugin bench`; opt-in pprof helper in `sdk/testing`; per-leaf JSON timing extending existing `--output json`. | `internal/cli/plugin.go`, `sdk/testing` |
| 7 — Documentation | Author guide (`docs/plugins.md`), README "external plugin handlers" section, CLI reference. | `docs/plugins.md`, `README.md` |
| 8 — Sample plugin | `examples/ironstate-handler-hosts/**`, its own docs/tests, used as the end-to-end acceptance test for phases 1–5. | `examples/ironstate-handler-hosts/**` |

Rollback/blast-radius note: every phase is additive on top of the existing
`handlers.All()`/`tasks.Expand`/`engine.Run` seam — no existing bare module name or
playbook behavior changes. A user who never runs `ironstate plugin install` or writes a
`plugins:` block sees no behavior change at all.

## 15. Rubber-duck review

This plan was reviewed by an independent agent role-playing a skeptical senior Go
engineer, specifically probing: the `go-plugin` choice vs. alternatives, the
explicit-vs-automatic install decision in §9, the "render before invoking" answer to
the `Context.Filters` problem in §5, and whether the §2 scope cuts (dependency
auto-install, monitoring/visualization) are actually reasonable or just convenient.
Findings and disposition are recorded in Appendix B.

---

## Appendix A — Original request (verbatim, superseded by this plan)

> a way to external go modules to provide additional handlers. they should be
> discovered by a naming convention, e.g. `ironstate-handler-<name>` and be able to be
> installed via `go install github.com/<org>/ironstate-handler-<name>@latest` or
> `go install github.com/<org>/ironstate-handler-<name>@vX.Y.Z`. The handlers should be
> referenced by scope of the `<org>.<name>` to prevent naming conflicts. The handlers
> should be able to be used in playbooks without any additional configuration, e.g.
> `- name: do something with my custom handler` `<org>.<name>.my_custom_handler:`.
> ironstate should be able to discover the installed handlers and make them available
> for use in playbooks. The handlers should be able to define their own parameters and
> options, and should be able to return results that can be used in subsequent tasks.
> The handlers should be able to be tested independently of ironstate, and should have
> their own documentation and examples.
>
> playbook should have syntax that can handle installing defined community plugin.
> maybe something like:
>
> plugins:
>   - use: org.name@latest
>   - use: org2.name@latest
>
> communication from handler to ironstate should be done via a well-defined interface,
> e.g. a Go interface that the handler implements, and using standard plugin
> communication mechanisms. Follow best practices for plugin development. The
> interface should define methods for initializing the handler, executing tasks, and
> returning results. The interface should also define methods for handling errors and
> logging. Handlers should be able to register themselves with ironstate at runtime,
> and ironstate should be able to discover and load the handlers dynamically. Handlers
> should be able to define their own configuration options, which can be specified in
> the playbook or in a separate configuration file. Handlers should be able to define
> their own dependencies, which can be installed automatically by ironstate if they are
> not already present on the system. Handlers should be able to define their own
> versioning scheme, and ironstate should be able to manage multiple versions of the
> same handler. Handlers should be able to define their own licensing terms, and
> ironstate should respect those terms when distributing the handlers.
>
> ironstate should provide a way to list all installed handlers, along with their
> versions and documentation. ironstate should provide a way to install a handler by
> means of something like: `ironstate plugin install <org>.<name>[@<version>|@latest]`.
> ironstate should provide a way to update installed handlers to the latest version, or
> to a specific version. ironstate should provide a way to uninstall handlers that are
> no longer needed. ironstate doctor should be able to detect playbook errors that are
> caused by missing or incompatible handlers, and provide suggestions for resolving
> them. current builtin handlers should be accessible via the similar naming
> convention, e.g. `ironstate.builtin.<name>`. The builtin handlers should use the same
> interface as external handlers.
>
> ironstate should provide a way to test handlers in isolation, without running the
> entire playbook. ironstate should provide a way to debug handlers, with detailed
> logging and error reporting. ironstate should provide a way to profile handlers, to
> identify performance bottlenecks and optimize their execution. ironstate should
> provide a way to benchmark handlers, to compare their performance against other
> implementations. ironstate should provide a way to monitor handlers, to track their
> resource usage and performance over time. ironstate should provide a way to visualize
> handler execution, with graphs and charts that show their behavior and performance.
> ironstate should provide a way to document handlers, with examples and tutorials that
> show how to use them effectively.

## Appendix B — Rubber-duck review findings

Review agent verified every code claim below directly against the repository (not
taken on faith) before critiquing the design. Disposition recorded for each finding.

### Must-fix — both adopted, plan body updated

1. **§5's original "builtins already work this way" claim was false.**
   `assert.go:44` calls `conditions.TestCondition(s, ctx.Flat, ctx.Filters)` at
   `Install`-time; `template.go`, `lineinfile.go`, `mountfacts.go`, `waitfor.go`, and
   `async.go` all thread live `ctx.Filters` into their own runtime evaluation. These
   are core handlers, not edge cases. **Adopted**: §5 now specifies a
   `HandlerHostCallback` service over `go-plugin`'s `GRPCBroker` (the same
   bidirectional pattern Terraform uses for provisioner UI output) so a plugin can
   request template rendering / condition evaluation on demand, backed by the host's
   real `expr.Filters`/`templateengines` implementations. Simple plugins never touch
   it; an `assert`-like or `template`-like plugin now can.
2. **§9's `--yes` flag had no precedent and wasn't reconciled with `--apply`**, while
   `pacman.go`/`yum.go` already auto-confirm OS package installs under plain `--apply`.
   **Adopted**: §9 now names the flag `--allow-plugin-install` (distinct from
   `--apply`) and states explicitly why plugin code execution gets a stricter,
   separately-named gate — a vetted OS package repo vs. arbitrary `go install`-fetched
   source named by whoever wrote the playbook are different trust boundaries, and
   collapsing them into one flag would hide that difference rather than resolve it.

### Should-consider — adopted as documentation/design clarifications

1. **Per-invocation subprocess/handshake cost was implied, not stated.** ironstate has
   no daemon mode; every plugin pays a full launch+handshake on every single CLI
   invocation, with reuse only *within* one run. **Adopted**: §7 now states this
   plainly as an accepted, Terraform/Vault-style tradeoff of process isolation, and
   notes it's a reason hot-path functionality belongs in builtins, not plugins.
2. **Nested-module release tagging (`sdk/vX.Y.Z`) was never mentioned**, and getting it
   wrong silently breaks every external plugin author's `go get`. **Adopted**: called
   out explicitly in §6 as a release-checklist item for Phase 1.
3. **Protocol version "negotiation" was asserted, not specified.** **Adopted**: §7 now
   states the actual policy — exact-match required, hard failure naming both versions
   and which side to upgrade, surfaced proactively by `doctor` (§11).
4. **`ExecResult.Extra` is already silently dropped by `--output json` today**
   (`internal/engine/output.go`'s `jsonResult`/`jsonExecResult`), a pre-existing gap
   independent of this plan but one that would make a plugin's `Extra`/fact data look
   like "a plugin bug" once plugins start relying on it. **Adopted**: added as an
   explicit fix inside Phase 2's scope (§14), not deferred to Phase 6, since Phase 2 is
   when plugin-contributed `Extra` data starts flowing at all.

### Nitpicks — adopted, one line each

1. Lockfile-vs-manifest precedence on disagreement was unstated — §8 now says the
   lockfile wins for version selection and a checksum mismatch hard-fails rather than
   silently falling through.
2. `sdk/becomeexec` was a one-bullet mention despite being the SDK piece most likely to
   have OS-specific edge cases — §6 now states it must delegate to whatever
   `internal/exec/become.go` already does per platform rather than inventing new
   elevation behavior, and flags that its actual current behavior needs to be read (not
   assumed) during Phase 1.

### Findings not adopted, with reasoning

None. Every finding the review raised was grounded in a direct code citation (verified
independently by the reviewing agent reading the source, not by trusting the plan's
own claims about it), and each pointed at either a factual error in the plan (must-fix
1) or a real, unstated gap (all others). There was nothing to push back on this pass —
if a future review disagrees with the `--allow-plugin-install` vs. `--apply` split
(must-fix 2) specifically, that's the one judgment call in this batch (as opposed to a
factual correction) and worth a second look if it proves annoying in practice for CI
users who already fully trust their playbook sources.
