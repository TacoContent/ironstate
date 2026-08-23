package handlers

import (
	"fmt"
	"strings"

	"github.com/TacoContent/ironstate/internal/conditions"
	"github.com/TacoContent/ironstate/internal/engine"
)

// assertHandler ports Handlers/Assert.psm1: fails the task unless every
// 'that' condition holds. Test always reports "not installed" ('state' is
// ignored - not a meaningful field here), so this always resolves to
// Install - the check always runs when reached. Pass/fail becomes this
// leaf's rc (0/1), which is exactly what internal/engine's generic
// 'failed_when'/'continue_on_error' machinery already acts on.
type assertHandler struct{}

func (assertHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	return false, nil
}

func (assertHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	that := asList(item["that"])
	parts := make([]string, 0, len(that))
	for _, t := range that {
		if s, ok := t.(string); ok {
			parts = append(parts, s)
		}
	}
	return "assert: " + strings.Join(parts, " && "), nil
}

func (assertHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	that := asList(item["that"])
	label := describeLabel(item, name)

	var failedConditions []string
	for _, raw := range that {
		s, ok := raw.(string)
		if !ok || strings.TrimSpace(s) == "" {
			continue
		}
		ok2, err := conditions.TestCondition(s, ctx.Flat, ctx.Filters)
		if err != nil {
			return engine.ExecResult{}, err
		}
		if !ok2 {
			failedConditions = append(failedConditions, s)
		}
	}

	if len(failedConditions) > 0 {
		message := getString(item, "fail_msg")
		if message == "" {
			message = fmt.Sprintf("Assertion failed for task '%s': %s", label, strings.Join(failedConditions, "; "))
		}
		engine.Warn("%s", message)
		return engine.ExecResult{RC: 1, Stderr: message, StderrLines: []string{message}}, nil
	}

	message := getString(item, "success_msg")
	if message == "" {
		message = fmt.Sprintf("Assertion passed for task '%s' (%d condition(s)).", label, len(that))
	}
	engine.Info("%s", message)
	return engine.ExecResult{RC: 0, Stdout: message, StdoutLines: []string{message}}, nil
}

func (assertHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	// Never reached: Test always reports "not installed".
	return engine.ExecResult{}, nil
}
