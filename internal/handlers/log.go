package handlers

import (
	"fmt"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// logHandler ports Handlers/Log.psm1: reuses the present/absent/latest
// state machine instead of a second "always run" module shape. Test
// reports "installed" exactly when state is 'absent', so
// 'present'/'latest' always resolve to Install (prints the 'install'
// message) and 'absent' always resolves to Uninstall (prints the
// 'uninstall' message) - log has no real idempotent "already applied"
// concept.
type logHandler struct{}

// logPhaseSpec resolves { message, level } for phase ('install'/
// 'uninstall'): the nested form if present, else - only for 'install' when
// neither nested key exists at all - the flat shorthand
// '{ message, level }' directly on item.
func logPhaseSpec(item map[string]any, phase string) map[string]any {
	if nested := getMap(item, phase); nested != nil {
		return nested
	}
	_, hasInstall := item["install"]
	_, hasUninstall := item["uninstall"]
	if phase == "install" && !hasInstall && !hasUninstall {
		return item
	}
	return nil
}

func logPhaseForAction(action engine.Action) string {
	if action == engine.ActionUninstall {
		return "uninstall"
	}
	return "install"
}

func (logHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	return itemState(item) == "absent", nil
}

func (logHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	phase := logPhaseForAction(action)
	spec := logPhaseSpec(item, phase)
	message := ""
	if spec != nil {
		message = getString(spec, "message")
	}
	if message == "" {
		return fmt.Sprintf("log (%s): <no message>", phase), nil
	}
	return fmt.Sprintf("log (%s): %s", phase, message), nil
}

func (logHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	writeLogMessage(item, "install")
	return engine.ExecResult{}, nil
}

func (logHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	writeLogMessage(item, "uninstall")
	return engine.ExecResult{}, nil
}

func writeLogMessage(item map[string]any, phase string) {
	spec := logPhaseSpec(item, phase)
	if spec == nil {
		return
	}
	message := getString(spec, "message")
	if message == "" {
		engine.Warn("log action's '%s' phase has no 'message'", phase)
		return
	}
	switch strings.ToLower(getStringOr(spec, "level", "info")) {
	case "warning":
		engine.Warn("%s", message)
	case "error":
		engine.Warn("%s", message) // no separate error-severity output stream today; treated like warning
	case "debug", "verbose":
		// dropped: these are non-default-visible in the original too (Write-Debug/-Verbose)
	default:
		engine.Info("%s", message)
	}
}
