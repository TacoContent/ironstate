//go:build !windows

package handlers

import (
	"fmt"

	"github.com/TacoContent/ironstate/internal/engine"
)

// registryHandler is a stub on non-Windows platforms: the Windows
// registry has no equivalent here - matches docs/plans/go-rewrite.md
// §1's "Windows-only handlers stay Windows-only" scope decision.
type registryHandler struct{}

var errRegistryUnsupportedOS = fmt.Errorf("the 'registry' module is only supported on Windows")

func (registryHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	return false, errRegistryUnsupportedOS
}

func (registryHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	return "", errRegistryUnsupportedOS
}

func (registryHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, errRegistryUnsupportedOS
}

func (registryHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, errRegistryUnsupportedOS
}
