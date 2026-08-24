package handlers

import (
	"fmt"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// logHandler ports Handlers/Log.psm1: reuses the present/absent/latest
// state machine instead of a second "always run" module shape. A log
// with no phase-specific 'install:'/'uninstall:' section at all - just
// the flat '{ message, level }' shorthand, or nothing at all - always
// runs, matching the original's "log has no real idempotent 'already
// applied' concept" (its message may still end up empty at print time,
// e.g. an unresolved '${{ }}' reference - that's fine, it just prints
// nothing). Test() only reports a phase "already satisfied" (skip) when
// this item explicitly scopes itself to the OTHER phase only (an
// 'install:'/'uninstall:' section for the other phase, nothing for this
// one and no flat default), or when the phase-specific section that DOES
// exist for this phase has no message of its own - see logShouldRun.
type logHandler struct{}

// logResolvedSpec resolves { message, level } for phase ('install'/
// 'uninstall'): the nested '<phase>: { message, level }' form if the
// 'phase' key is present at all, else the flat '{ message, level }'
// shorthand directly on item - its "default", shared by whichever phase
// has no dedicated section - or nil if neither exists, meaning this
// phase has nothing to log.
func logResolvedSpec(item map[string]any, phase string) map[string]any {
	if nested, ok := item[phase].(map[string]any); ok {
		return nested
	}
	if _, hasMessage := item["message"]; hasMessage {
		return item
	}
	return nil
}

func logMessage(item map[string]any, phase string) string {
	if spec := logResolvedSpec(item, phase); spec != nil {
		return getString(spec, "message")
	}
	return ""
}

func logOtherPhase(phase string) string {
	if phase == "install" {
		return "uninstall"
	}
	return "install"
}

// logShouldRun decides, structurally (not by message content alone),
// whether phase applies to this log item at all:
//   - an explicit '<phase>:' section applies to phase only if it has its
//     own message (an explicit but message-less section means "nothing
//     to do here", deliberately left empty).
//   - the flat '{ message, level }' shorthand always applies to whichever
//     phase has no section of its own, regardless of whether its message
//     currently resolves to anything.
//   - with no section for phase, no flat shorthand, but an explicit
//     section for the OTHER phase, this item is scoped to that other
//     phase only - phase does not apply.
//   - with nothing scoped at all, phase always applies (the "no real
//     idempotent concept" default).
func logShouldRun(item map[string]any, phase string) bool {
	if nested, ok := item[phase].(map[string]any); ok {
		return getString(nested, "message") != ""
	}
	if _, hasMessage := item["message"]; hasMessage {
		return true
	}
	if _, hasOther := item[logOtherPhase(phase)].(map[string]any); hasOther {
		return false
	}
	return true
}

func logPhaseForAction(action engine.Action) string {
	if action == engine.ActionUninstall {
		return "uninstall"
	}
	return "install"
}

func (logHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	state := itemState(item)
	phase := "install"
	if state == "absent" {
		phase = "uninstall"
	}
	shouldRun := logShouldRun(item, phase)
	if state == "absent" {
		return shouldRun, nil // "installed" -> Uninstall dispatches when this phase applies
	}
	return !shouldRun, nil // "not installed" -> Install dispatches when this phase applies
}

func (logHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	phase := logPhaseForAction(action)
	message := logMessage(item, phase)
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
	spec := logResolvedSpec(item, phase)
	if spec == nil {
		return
	}
	message := getString(spec, "message")
	if message == "" {
		// Nothing to say for this phase - Test() already skips this case
		// for 'present'/'absent'; only reachable for an explicit
		// 'state: latest', which always dispatches regardless. Silent,
		// not a warning: an empty/omitted message is routinely expected
		// (e.g. a template reference that didn't resolve), not a real
		// problem.
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
