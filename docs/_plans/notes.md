# Notes

## Possible Handlers


- [find](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/find_module.html#ansible-collections-ansible-builtin-find-module): find files on the target system
- [get_url](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/get_url_module.html#ansible-collections-ansible-builtin-get-url-module): download a file from a HTTP/HTTPS/FTP
- [hostname](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/hostname_module.html#ansible-collections-ansible-builtin-hostname-module): manage the hostname of the target system
- choice: prompt the user to choose from a list of options (Linux/macOS) or alias for Windows pause
- service: manage services (Linux/macOS) or alias for Windows service management
- service_facts: gather facts about services (Linux/macOS) or alias for Windows service management
- tempfile: create a temporary file or directory (Linux/macOS) or alias for Windows tempfile

- unarchive: extract an archive (Linux/macOS) or alias for Windows unarchive



- AppImage: Download -> Extract to path

## GitHub

- create issue templates

## Handlers

IronState Package-Manager Handler System — Research Report
1. Location
All package-manager handlers live in a single flat Go package:

d:\Development\projects\ironstate\internal\handlers\ (package handlers)

The apt handler specifically is at internal/handlers/apt.go. There is no per-module subdirectory — this is a deliberate deviation from the original master-plan layout (see internal/handlers/handlers.go:9-15): "one Go package per module" was rejected in favor of one flat package, since handlers share small unexported helpers.

Related packages:

internal/engine/engine.go — the Handler/ScanCapable interfaces and dispatch loop
internal/exec/exec.go — the Runner interface used for shelling out (mockable)
internal/exec/become.go — sudo/elevation wrapping
internal/scan/scan.go — the Scanner/Registry that drives ironstate init --scan
internal/handlers/handlers.go — the registry (All() map + AllModuleNames)
internal/handlers/util.go — shared helpers (getString, getBool, itemState, runExternalCommand, etc.)
internal/handlers/scanutil.go — parseJSONList[T] generic helper for Windows JSON scans
internal/handlers/packagemanagers_test.go — tests for apt/winget/chocolatey/homebrew/npm/pipx/go/eget
internal/handlers/handlers_test.go — shared testCtx() helper + tests for other module families
Go module path: github.com/TacoContent/ironstate (go.mod: go 1.27.0).

2. Full structure of existing package-manager handlers
Every package-manager handler is one file, one unexported empty struct, four/six methods, all in package handlers. None currently import subpackages of their own. Here's each:

File	Type	Test/Describe/Install/Uninstall	ScanCapable (ScanRole+Scan)?
internal/handlers/apt.go	aptHandler	yes	yes
internal/handlers/homebrew.go	homebrewHandler	yes	yes
internal/handlers/winget.go	wingetHandler	yes	yes
internal/handlers/chocolatey.go	chocolateyHandler	yes	yes
internal/handlers/npm.go	npmHandler	yes	yes
internal/handlers/pipx.go	pipxHandler	yes	no
internal/handlers/cargo.go	cargoHandler	yes	no
internal/handlers/gomodule.go	goHandler	yes	no (installs via go install, tracks by binary file existence, no scan)
internal/handlers/rubygem.go	rubyGemHandler	yes	no
internal/handlers/eget.go	egetHandler	yes	no
Note: only 5 of 10 existing handlers implement ScanCapable (apt, homebrew, winget, chocolatey, npm) — the ones with a natural "list installed" command. pipx/cargo/go/gem/eget don't (no clean discovery command / not worth it yet, presumably an omission you may want to fill in for the new 8).

apt.go — the fullest, most representative example (verbatim, 267 lines)

package handlers

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/TacoContent/ironstate/internal/engine"
)

// aptHandler wraps Debian/Ubuntu's apt-get, modeled on the core surface of
// ansible.builtin.apt (...): package/name, state, update_cache+cache_valid_time,
// upgrade, purge, autoremove/autoclean, install_recommends/only_upgrade/
// allow_unauthenticated/force.
//
// apt-get install/remove/purge/autoremove/update all require root — this
// handler issues plain 'apt-get' commands and relies entirely on the
// engine's shared 'become'/sudo wrapping (see internal/exec/become.go)
// rather than handling elevation itself.
type aptHandler struct{}

func aptPackageList(item map[string]any) []string { ... }               // reads 'package' aliasing 'name', string or list
func isAptPackageInstalled(pkg string) bool { ... }                     // dpkg-query -W -f='${Status}' pkg

func (aptHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) { ... }

func aptCacheValid(cacheValidSeconds int) bool { ... }                  // checks mtime of /var/cache/apt/pkgcache.bin
func aptInstallFlags(item map[string]any) []string { ... }

type aptStep struct {
	description string
	args        []string
}

func aptPlan(item map[string]any, action engine.Action) []aptStep { ... } // builds ordered apt-get invocations

func (aptHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) { ... }

func runAptPlan(item map[string]any, action engine.Action) engine.ExecResult { ... }

func (aptHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return runAptPlan(item, engine.ActionInstall), nil
}
func (aptHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return runAptPlan(item, engine.ActionUninstall), nil
}

// ScanRole implements engine.ScanCapable
func (aptHandler) ScanRole() string { return "roles/packages" }

// Scan implements engine.ScanCapable: 'apt-mark showmanual' (explicitly
// user-installed, excludes dependency-only packages, mirrors 'brew leaves')
func (aptHandler) Scan(ctx engine.Context) ([]engine.ScanItem, error) { ... }
Key implementation approach worth replicating:

Test shells out to a read-only "is it installed" check (dpkg-query, brew list, winget list --id ... --exact, choco list --local-only ... --exact --limit-output, npm list -g pkg --depth=0, pipx list --short, cargo install --list, gem list pkg -i) and treats any invocation error as "not installed" (return false, nil //nolint:nilerr).
Describe returns a plain human-readable string of the command that would run (used for dry-run -would install- lines).
Install/Uninstall call runExternalCommand(exe, args) (from util.go) which handles become elevation, echoes stdout/stderr via engine.Info/engine.Warn, and normalizes to engine.ExecResult. On non-zero exit they call engine.Warn(...) but still return the result (don't error) — a non-zero RC is what marks the leaf failed upstream in engine.go.
latest state pattern (homebrew, pipx): try upgrade first, fall back to install if upgrade fails (e.g. package not yet installed).
Scan shells out to a "list what I explicitly installed" command, converts each line/JSON entry into an engine.ScanItem{Module, Name, Config, Tags}, skips blank lines, and gates on runtime.GOOS/exec.LookPath (e.g. apt/homebrew/npm bail early on Windows; winget/chocolatey bail on non-Windows).
homebrew.go, winget.go, chocolatey.go, npm.go, pipx.go, cargo.go, gomodule.go, rubygem.go, eget.go
All shown in full above during research — each follows the same 4-method shape. Notable variations:

winget.go is the most complex non-apt handler: Scan does a winget export to a temp file (os.CreateTemp, deleted via defer os.Remove), parses JSON (wingetExportFile struct), and resolves Microsoft Store package IDs to friendly names via a separate winget list lookup (wingetDisplayNameLookup, overridable var for tests).
gomodule.go / eget.go don't shell out to check installed state — they check for file existence at a computed binary path (fileExists, os.Remove for uninstall). No Scan.
cargo.go uses regexp.MustCompile to match package name against cargo install --list output.
3. The Handler interface (contract every handler implements)
From internal/engine/engine.go:99-104, verbatim:


// Handler is the uniform Test/Describe/Install/Uninstall shape every
// module implements — ports modules/Handlers/*.psm1's PSCustomObject
// script-block contract (docs/plans/go-rewrite.md §4.8).
type Handler interface {
	Test(item map[string]any, name string, ctx Context) (bool, error)
	Describe(item map[string]any, action Action, ctx Context) (string, error)
	Install(item map[string]any, name string, ctx Context) (ExecResult, error)
	Uninstall(item map[string]any, name string, ctx Context) (ExecResult, error)
}
Optional extra interface for scanning (internal/engine/engine.go:140-154), verbatim:


// ScanCapable is an optional extra a Handler implements when it can also
// discover its own module's current state on this system and report it
// back as playbook items - the discovery-side mirror of Install/
// Uninstall. internal/scan's Registry type-asserts each handlers.All()
// entry for this interface, so a module's scan logic lives next to its
// own Handler instead of internal/scan maintaining a separate, hardcoded
// scanner per module.
type ScanCapable interface {
	// ScanRole names the playbook role directory (e.g.
	// "roles/system/users") this handler's scanned items are grouped
	// under when internal/scan.GeneratePlaybook writes the role tree.
	ScanRole() string
	// Scan discovers this handler's current system state.
	Scan(ctx Context) ([]ScanItem, error)
}
Another optional extra, FactProducer (engine.go:116-121) — not relevant to package managers, used by mount_facts/fact.

Supporting types (all internal/engine/engine.go):


type Action string
const (
	ActionSkip      Action = "Skip"
	ActionInstall   Action = "Install"
	ActionUninstall Action = "Uninstall"
)

type ExecResult struct {
	RC          int
	Stdout      string
	StdoutLines []string
	Stderr      string
	StderrLines []string
	Extra       map[string]any
}

type Context struct {
	Flat    map[string]any
	Filters expr.Filters
	Apply   bool
	Become  ironexec.Become
}

type ScanItem struct {
	Module string         `yaml:"-"`
	Name   string         `yaml:"name"`
	Config map[string]any `yaml:"config"`
	Tags   []string       `yaml:"tags,omitempty"`
	Role   string         `yaml:"-"`
}
state resolution logic (resolvePackageAction, engine.go:738-755) — governs when Install vs Uninstall vs skip is called: present/installed → skip if Test reports installed else Install; latest/build-dep/fixed → always Install (handler must self-gate idempotency); absent/removed → Uninstall if installed else skip; anything else is a hard error (unknown state %q).

4. Registration / wiring
No dynamic discovery, no DI container — a plain map literal. internal/handlers/handlers.go:


var AllModuleNames = []string{
	"winget", "chocolatey", "homebrew", "brew", "apt", "gem", "pipx", "npm", "cargo", "go", "eget",
	"git", "cron", "cron_unix", "cron_file", "iptables", "ufw", "advfirewall", "firewall", "zip", "symlinks", "file", "copy", "template", "shell", "blockinfile", "lineinfile",
	"ssh_host_block", "log", "fail", "path", "fact", "mount_facts", "registry", "scheduled_task", "group", "user",
	"assert", "async", "wait_for", "service",
}

func All() map[string]engine.Handler {
	return map[string]engine.Handler{
		...
		"apt":            aptHandler{},
		"pipx":           pipxHandler{},
		"npm":            npmHandler{},
		"cargo":          cargoHandler{},
		"go":             goHandler{},
		"gem":            rubyGemHandler{},
		"eget":           egetHandler{},
		...
	}
}
To add a new handler you must touch two spots: add its zero-value struct literal to the All() map (module-name key → xHandler{}), and add the module name string to AllModuleNames.

Three other places that need the module name registered, all in internal/engine/engine.go:

DefaultNoCommandCheckModules (engine.go:227-233) — set of modules with no external CLI to PATH-check. Package-manager modules are deliberately excluded from this map (they DO need PATH checks) — so you do NOT add new CLI-backed handlers here.
DefaultModuleCommandNames (engine.go:237) — remaps a module's task-tree key to its actual binary name where they differ: {"chocolatey": "choco", "homebrew": "brew", "apt": "apt-get", "advfirewall": "netsh"}. Only needed if the module key != binary name (e.g. a future flatpak handler keyed "flatpak" running binary flatpak needs no entry; homebrew→brew does).
RunLeaves's dispatch loop does the actual PATH-check/skip via LookPath(commandName), caching per-module in state.CommandAvailability.
The scan side wires up automatically — no registration needed there. internal/scan/scan.go's defaultScanners() iterates handlers.All(), type-asserts each entry for engine.ScanCapable, and wraps matches in a handlerScanner. So as long as your new handler struct implements ScanRole()/Scan(), it's picked up by ironstate init --scan automatically once it's in the All() map — no separate registry edit.

One more place: the JSON Schema (ironstate.schema.json) declares each module's field shape under $defs and references it from taskItem.properties. Note there's documented schema drift — homebrew/apt/gem are NOT in the schema despite being implemented (called out explicitly in apt.go's and rubygem.go's doc comments as "a pre-existing schema/docs drift bug ... implemented here regardless"). So schema entries are not strictly required to ship a working handler, but doing it properly for new handlers may be expected — you should ask whether to fix schema drift as part of this work or follow the existing (imperfect) precedent.

Also update README.md's "Supported modules" table (README.md:707-743) and the rc/stdout CLI-backed-modules list (README.md:620), matching the existing doc pattern.

5. What Scan is supposed to do (per the "own scanner" commit)
Commit 2be765d ("fix: handlers now implement their own scanner...") moved scan logic that used to live centrally in internal/scan's packageScanner into each handler itself, via the ScanCapable interface (see stat: internal/scan/scan.go shrank from 897 to ~270 lines; apt.go/chocolatey.go/homebrew.go/npm.go/winget.go each grew ~40-150 lines).

Scan(ctx engine.Context) ([]engine.ScanItem, error) must:

Detect whether the package manager itself is present/applicable — either by runtime.GOOS check (winget/chocolatey: if runtime.GOOS != "windows" { return nil, nil }; apt/homebrew/npm: if runtime.GOOS == "windows" { return nil, nil }) and/or exec.LookPath("binary") check, returning (nil, nil) — not an error — when unavailable.
Enumerate explicitly-user-installed packages (not transitive dependencies) via the package manager's own "leaves"/"manual"/"top-level" query: apt-mark showmanual, brew leaves + brew list --cask -1, choco list --local-only --limit-output, npm list -g --depth=0 --json, winget export.
Convert each discovered package into an engine.ScanItem: Module = the handler's own module key, Name = display name, Config = a ready-to-emit task config map (typically {"package": name, "state": "present"}), Tags = []string{"packages"}.
Implement ScanRole() string — nearly always return "roles/packages" for package managers (used to route into roles/packages/main.yml when GeneratePlaybook writes the baseline tree).
Treat command-invocation failures as "nothing to report" (return nil, nil //nolint:nilerr), not as errors — errors should be reserved for genuine problems (see winget's export-parsing path, which does propagate err).
internal/scan/scan.go's Registry.ScanAll()/ScanAllWithProgress() calls every registered Scanner.Scan(), stamps Role from Scanner.Role() if the item didn't already set one, and aggregates into one []Item slice, which GeneratePlaybook then buckets by Role and writes out as roles/<role>/main.yml YAML task lists (this backs ironstate init --scan).

6. Documentation on intended design for pacman/yum/apk/appimage/snap/flatpak/scoop/macports
I found no documentation anywhere in the repo describing these 8 specific handlers. I searched (case-insensitively) for pacman|yum|apk|appimage|snap|flatpak|scoop|macports|dnf|zypper across the whole repo. Matches were all false positives/unrelated (.goreleaser.yaml's --snapshot flag, internal/engine/async.go's "wait_for"/"snapshot" doc comments about async task state, internal/handlers/waitfor.go, CI workflow "snap"-shot builds). None of the design docs (docs/plans/go-rewrite.md, docs/plans/go-rewrite-progress.md, docs/plans/notes.md, docs/plans/ironstate-plugins.md, docs/plans/secrets-masking.md) or README.md or ironstate.schema.json mention any of these 8 package managers by name.

docs/plans/notes.md's "Possible Handlers" section only lists non-package-manager ideas (find, get_url, hostname, choice, service, service_facts, tempfile, unarchive) — nothing about pacman/yum/etc.

This means there is no existing design intent to follow for these 8 handlers beyond the general pattern established by apt/homebrew/winget/chocolatey/npm/pipx/cargo/go/gem/eget — you'll be extrapolating the pattern, not implementing a pre-specified design. Given the org's "ask when you need clarification, don't assume context" instruction, you may want to confirm scope/behavior choices (e.g., should pacman/yum/apk/dnf-equivalents batch multiple packages like apt does, or one-at-a-time like the rest? should appimage/snap/flatpak have distinct install semantics like go/eget's file-existence-based Test? should all 8 implement ScanCapable?) before implementation.

docs/plans/ironstate-plugins.md (9 lines) is a separate, unrelated future-looking note about an external plugin system (ironstate-handler-<name> Go modules discovered/installed dynamically) — not relevant to built-in handlers and not yet implemented.

7. Test conventions (from packagemanagers_test.go and handlers_test.go)
Tests for package-manager handlers all live in internal/handlers/packagemanagers_test.go (one shared file, not per-handler — matches the flat-package convention). Non-package-manager handler tests live in per-concern files (handlers_test.go, group_test.go, user_test.go, template_test.go, mountfacts_test.go, async_test.go, registry_windows_test.go, scheduledtask_windows_test.go).
Mocking pattern: runner is a package-level var runner ironexec.Runner = ironexec.Default (defined in util.go:147). Tests swap it out via a helper:

type recordingRunner struct {
	calls     [][]string
	exes      []string
	responses []ironexec.Result
	errs      []error
}
func (r *recordingRunner) Run(exe string, args []string) (ironexec.Result, error) { ... }

func withRunner(t *testing.T, r *recordingRunner) {
	t.Helper()
	orig := runner
	runner = r
	t.Cleanup(func() { runner = orig })
}
recordingRunner queues canned ironexec.Result/error per call index (repeating the last response once exhausted) and records every (exe, args) invocation for assertion. An alternative, simpler mock used elsewhere: type fakeRunnerFunc func(exe string, args []string) (ironexec.Result, error) implementing Run directly (seen in handlers_test.go for group/user/git/iptables/ufw/firewall tests) — useful when you need conditional/stateful behavior across calls rather than a fixed queue.

Context helper: testCtx() engine.Context { return engine.Context{Flat: map[string]any{}, Apply: true} } (handlers_test.go:17) — used everywhere instead of constructing engine.Context inline.
Typical test shape: build an item := map[string]any{"package": "x", ...}, call h.Install(item, "", testCtx()) or h.Test(...), assert on rec.exes[0]/rec.calls[0] (exact argv, joined with spaces via strings.Join(rec.calls[0], " ")) and/or the returned result.RC.
Tests assert exact argv (e.g. want := []string{"install", "-y", "git", "curl"}), not just "was some install call made" — replicate this precision for new handlers.
ScanRole()/Scan() get their own lightweight assertions, e.g. TestWingetHandlerScanRoleAndRoleAreRolesPackages just checks the returned string constant per handler.
TestBrewIsRegisteredAsHomebrewAlias demonstrates the pattern for testing alias registration in All()/AllModuleNames if a new handler needs one.
File-based handlers (go, eget) use t.TempDir() + real file I/O rather than mocking runner at all, since they don't shell out for Test.
8. Module path, naming conventions, error handling, lint config
Module path: github.com/TacoContent/ironstate, go 1.27.0.
Package: everything package-manager-related is package handlers (no subpackages).
Type naming: <name>Handler (lowercase, unexported), e.g. aptHandler, homebrewHandler, wingetHandler, chocolateyHandler, npmHandler, pipxHandler, cargoHandler, goHandler, rubyGemHandler, egetHandler. Always an empty struct (type xHandler struct{}), stateless.
Receiver naming: receivers are consistently anonymous/unnamed since the struct carries no state: func (aptHandler) Test(...), func (homebrewHandler) Install(...), etc. — not func (h aptHandler) .... Follow this exactly for new handlers.
Interface name: the contract type itself is named Handler (not PackageManager or similar) — it's generic across all module kinds, not package-manager-specific. There is no separate "PackageManager" interface; package-manager handlers are just Handler + optionally ScanCapable.
Function/parameter naming: item map[string]any, name string, ctx engine.Context/Context throughout — exact same three Test/Describe params and (item, name, ctx) for Install/Uninstall, matched verbatim across every handler.
Error handling style:
Most Test methods swallow invocation errors and report "not installed": return false, nil //nolint:nilerr // any invocation failure here just means "not installed" — a documented, deliberate //nolint:nilerr convention, always with an inline justification comment.
Install/Uninstall generally do not return a Go error for a failed external command — they return engine.ExecResult{RC: nonzero} and call engine.Warn("... exited with code %d", pkg, result.RC). A returned error is reserved for truly exceptional/programmer-facing conditions (e.g. advFirewallHandler errors if name is missing).
runExternalCommand (in util.go) is the single shared chokepoint for all mutating CLI calls — wraps become, echoes stdout/stderr via engine.Info/engine.Warn per-line, normalizes to engine.ExecResult. New handlers should call this, not runner.Run directly, for Install/Uninstall; runner.Run directly is used only for read-only Test/Scan queries (bypassing become/echo, since those are non-mutating checks).
nolint directives are always accompanied by a specific linter name and a one-line reason (//nolint:gosec // ..., //nolint:nilerr // ...) — per .golangci.yml's explicit policy comment: "Suppress per-callsite with a //nolint:gosec plus a one-line justification rather than disabling the rule globally."
Lint config (.golangci.yml, version "2"): enabled linters are staticcheck, errcheck, gosec, unused, ineffassign; run.timeout: 5m. Notably permissive on gosec G204 (subprocess with variable args) since that's this tool's entire purpose — but still requires per-call //nolint:gosec justification, not blanket suppression.
Doc-comment style: every handler file has a package-doc-style comment above its type xHandler struct{} explaining what real-world CLI/module it ports, referencing either an Ansible module (ansible.builtin.apt) or a prior PowerShell module (Handlers/Winget.psm1) it's replacing, plus any elevation/PATH-remap caveat. Follow this pattern for the 8 new handlers (e.g. "pacmanHandler ports pacman (Arch Linux packages)... modeled on ansible.builtin.pacman's core surface").
Helper reuse: getString, getStringOr, getBool, getMap, getFloat, stringSlice, asList, itemState, resolvePath, fileExists, firstNonEmptyLine, itemLabel, describeLabel all live in util.go and are reused, not reimplemented, by every handler.
CLI binary invocation via runner.Run(exe, args) for read checks; runExternalCommand(exe, args) for mutating calls — this split is consistent across all 10 existing handlers.
