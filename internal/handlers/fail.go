package handlers

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// failHandler ports Handlers/Fail.psm1: if the given 'condition' is true, aborts the current leaf with the given 'message' (or a default message if none is provided). The condition is evaluated in the same context as the leaf's other fields, so it can reference any of them (e.g. a 'vars:' value).
type failHandler struct{}

func (f failHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	state := itemState(item)
	phase := "install"
	if state == "absent" {
		phase = "uninstall"
	}
	shouldRun := f.failShouldRun(item, phase)
	if state == "absent" {
		return shouldRun, nil // "installed" -> Uninstall dispatches when this phase applies
	}
	return !shouldRun, nil // "not installed" -> Install dispatches when this phase applies
}

func (f failHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	phase := f.failPhaseForAction(action)
	message := f.failMessage(item, phase)
	if message == "" {
		return fmt.Sprintf("fail (%s): <no message>", phase), nil
	}
	return fmt.Sprintf("fail (%s): %s", phase, message), nil

}

func (f failHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	message := f.writeFailMessage(item, "install")
	// split message to array of lines
	lines := strings.Split(message, "\n")
	exitCode := f.atoiOrDefault(item["exit_code"], 1)

	return engine.ExecResult{
		RC:          exitCode,
		Stderr:      message,
		StderrLines: lines,
	}, nil
}

func (f failHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return f.Install(item, name, ctx)
}

func (failHandler) atoiOrDefault(v any, def int) int {
	switch t := v.(type) {
	case int:
		return t
	case float64:
		return int(t)
	case string:
		val, err := strconv.Atoi(v.(string))
		if err != nil {
			return def
		}
		return val
	}
	return def
}

func (f failHandler) writeFailMessage(item map[string]any, phase string) string {
	spec := f.failResolvedSpec(item, phase)
	if spec == nil {
		return ""
	}
	message := getString(spec, "message")
	if message == "" {
		// Nothing to say for this phase - Test() already skips this case
		// for 'present'/'absent'; only reachable for an explicit
		// 'state: latest', which always dispatches regardless. Silent,
		// not a warning: an empty/omitted message is routinely expected
		// (e.g. a template reference that didn't resolve), not a real
		// problem.
		return ""
	}
	engine.Danger("fail: %s", message)
	return message
}

func (f failHandler) failResolvedSpec(item map[string]any, phase string) map[string]any {
	if nested, ok := item[phase].(map[string]any); ok {
		return nested
	}
	if _, hasMessage := item["message"]; hasMessage {
		return item
	}
	return nil
}

func (f failHandler) failMessage(item map[string]any, phase string) string {
	if spec := f.failResolvedSpec(item, phase); spec != nil {
		return getString(spec, "message")
	}
	return ""
}

func (f failHandler) failShouldRun(item map[string]any, phase string) bool {
	if nested, ok := item[phase].(map[string]any); ok {
		return getString(nested, "message") != ""
	}
	if _, hasMessage := item["message"]; hasMessage {
		return true
	}
	if _, hasOther := item[f.failOtherPhase(phase)].(map[string]any); hasOther {
		return false
	}
	return true
}

func (f failHandler) failPhaseForAction(action engine.Action) string {
	if action == engine.ActionUninstall {
		return "uninstall"
	}
	return "install"
}

func (f failHandler) failOtherPhase(phase string) string {
	if phase == "install" {
		return "uninstall"
	}
	return "install"
}
