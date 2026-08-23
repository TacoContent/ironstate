//go:build !windows

package handlers

import (
	"fmt"

	"github.com/TacoContent/ironstate/internal/engine"
)

// scheduledTaskHandler is a stub on non-Windows platforms: Windows Task
// Scheduler has no equivalent here - matches docs/plans/go-rewrite.md
// §1's "Windows-only handlers stay Windows-only" scope decision.
type scheduledTaskHandler struct{}

var errScheduledTaskUnsupportedOS = fmt.Errorf("the 'scheduled_task' module is only supported on Windows")

func (scheduledTaskHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	return false, errScheduledTaskUnsupportedOS
}

func (scheduledTaskHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	return "", errScheduledTaskUnsupportedOS
}

func (scheduledTaskHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, errScheduledTaskUnsupportedOS
}

func (scheduledTaskHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, errScheduledTaskUnsupportedOS
}
