package handlers

import (
	"fmt"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
	"github.com/TacoContent/ironstate/internal/secrets"
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
// for real, delegating to the same shellStateConfig
// resolution/invocation the 'shell' module itself uses, exactly like
// ironstate.ps1's Handlers/Fact.psm1 delegating to Shell.psm1.
type factHandler struct{}

func (factHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	return itemState(item) == "absent", nil
}

func (factHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	factName := getStringOr(item, "name", getStringOr(item, "package", "<unnamed>"))
	secretName := strings.HasPrefix(factName, "$")
	if secretName {
		factName = strings.TrimPrefix(factName, "$")
	}
	if action == engine.ActionUninstall {
		return fmt.Sprintf("unset fact '%s'", factName), nil
	}
	if shellSpec := getMap(item, "shell"); shellSpec != nil {
		state := itemState(item)
		label := shellItemLabel(shellSpec, state)
		cfg := resolveShellStateConfig(shellSpec, state)
		return fmt.Sprintf("run shell '%s' via '%s' -> fact '%s'", label, cfg.HostSpec, factName), nil
	}
	value := item["value"]
	if secretName {
		if s, ok := value.(string); ok {
			secrets.Register(s)
			value = secrets.Redact(s)
		}
	}
	return fmt.Sprintf("set fact '%s' = %v", factName, value), nil
}

func (factHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	shellSpec := getMap(item, "shell")
	if shellSpec == nil {
		return engine.ExecResult{}, nil
	}
	state := itemState(item)
	cfg := resolveShellStateConfig(shellSpec, state)
	return invokeShellItem(cfg, shellItemLabel(shellSpec, state)), nil
}

func (factHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, nil
}
