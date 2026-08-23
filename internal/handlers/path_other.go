//go:build !windows

package handlers

import (
	"fmt"

	"github.com/TacoContent/ironstate/internal/engine"
)

// pathHandler is a stub on non-Windows platforms: this module manages the
// current user's persistent Windows PATH registry entry, which has no
// equivalent here - matches docs/plans/go-rewrite.md §1's "Windows-only
// handlers stay Windows-only" scope decision.
type pathHandler struct{}

var errPathUnsupportedOS = fmt.Errorf("the 'path' module is only supported on Windows")

func (pathHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	return false, errPathUnsupportedOS
}

func (pathHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	return "", errPathUnsupportedOS
}

func (pathHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, errPathUnsupportedOS
}

func (pathHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, errPathUnsupportedOS
}
