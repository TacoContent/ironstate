package handlers

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// factHandler ports Handlers/Fact.psm1: sets an arbitrary named value
// ('name'/'value') for later leaves to reference under 'facts.<name>'.
// Reuses logHandler's trick for a module with no real idempotency: Test
// reports "installed" exactly when state is 'absent', so
// 'present'/'latest' always resolve to Install (fact gets (re)set) and
// 'absent' always resolves to Uninstall (fact gets unset).
//
// The actual registry mutation (and the embedded-shell/deferred-'value'
// choreography) happens in internal/engine's dispatch loop, which reads
// this same 'name'/'value'/'shell' straight off the leaf's Item - this
// handler's own Install only needs to run the embedded 'shell' (if any)
// for real, exactly like ironstate.ps1's Handlers/Fact.psm1 delegating to
// Shell.psm1.
//
// Deviation from Handlers/Fact.psm1 (documented gap, not a silent one):
// the original delegates a fact's embedded 'shell' to the full Shell.psm1
// handler (host presets, per-state present/absent/latest fallback,
// 'creates', native-object merge) - none of that exists yet (Phase 4).
// This Phase 3 stand-in only supports the one real shape used anywhere in
// this repo today (roles/shell/main.yml): a bare '{ command: <string> }'
// run through the default 'pwsh' host as a subprocess. Revisit once
// Phase 4's real 'shell' handler lands and have this delegate to it
// instead.
type factHandler struct{}

func (factHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	return itemState(item) == "absent", nil
}

func (factHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	factName := getStringOr(item, "name", getStringOr(item, "package", "<unnamed>"))
	if action == engine.ActionUninstall {
		return fmt.Sprintf("unset fact '%s'", factName), nil
	}
	if shellSpec := getMap(item, "shell"); shellSpec != nil {
		return fmt.Sprintf("run shell '%s' -> fact '%s'", getString(shellSpec, "command"), factName), nil
	}
	return fmt.Sprintf("set fact '%s' = %v", factName, item["value"]), nil
}

func (factHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	shellSpec := getMap(item, "shell")
	if shellSpec == nil {
		return engine.ExecResult{}, nil
	}
	command := getString(shellSpec, "command")
	if command == "" {
		return engine.ExecResult{}, fmt.Errorf("fact '%s': embedded 'shell' has no 'command' (script-based embedded shell isn't supported yet, see Phase 4)", getString(item, "name"))
	}
	return runPwshCommand(command)
}

func (factHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, nil
}

// runPwshCommand is the Phase 3 minimal embedded-shell runner: overridable
// for tests so they never depend on a real 'pwsh' being on PATH.
var runPwshCommand = func(command string) (engine.ExecResult, error) {
	cmd := exec.Command("pwsh", "-NoLogo", "-NoProfile", "-Command", command) //nolint:gosec // 'command' is authored YAML content, same trust boundary as every other shell-shaped module in this tool
	stdout, stderr, err := runCaptured(cmd)
	rc := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			rc = exitErr.ExitCode()
		} else {
			return engine.ExecResult{}, err
		}
	}
	return engine.ExecResult{
		RC:          rc,
		Stdout:      stdout,
		StdoutLines: splitLines(stdout),
		Stderr:      stderr,
		StderrLines: splitLines(stderr),
	}, nil
}

func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return []string{}
	}
	return strings.Split(s, "\n")
}
