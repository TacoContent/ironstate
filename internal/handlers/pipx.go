package handlers

import (
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// pipxHandler ports Handlers/Pipx.psm1 (Python isolated tools).
type pipxHandler struct{}

func (pipxHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	pkg := getString(item, "package")
	result, err := runner.Run("pipx", []string{"list", "--short"})
	if err != nil {
		return false, nil //nolint:nilerr // any invocation failure here just means "not installed"
	}
	for _, line := range strings.Split(result.Stdout, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), pkg+" ") {
			return true, nil
		}
	}
	return false, nil
}

func (pipxHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	pkg := getString(item, "package")
	state := itemState(item)
	if action == engine.ActionUninstall {
		return "pipx uninstall " + pkg, nil
	}
	if state == "latest" {
		return "pipx upgrade " + pkg + " (installing if missing)", nil
	}
	return "pipx install " + pkg, nil
}

func (pipxHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkg := getString(item, "package")
	var result engine.ExecResult
	if itemState(item) == "latest" {
		result = runExternalCommand("pipx", []string{"upgrade", pkg})
		if result.RC != 0 {
			result = runExternalCommand("pipx", []string{"install", pkg})
		}
	} else {
		result = runExternalCommand("pipx", []string{"install", pkg})
	}
	if result.RC != 0 {
		engine.Warn("pipx install/upgrade %s exited with code %d", pkg, result.RC)
	}
	return result, nil
}

func (pipxHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkg := getString(item, "package")
	result := runExternalCommand("pipx", []string{"uninstall", pkg})
	if result.RC != 0 {
		engine.Warn("pipx uninstall %s exited with code %d", pkg, result.RC)
	}
	return result, nil
}
