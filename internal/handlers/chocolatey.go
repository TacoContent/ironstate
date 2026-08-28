package handlers

import (
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// chocolateyHandler ports Handlers/Chocolatey.psm1. The dispatch loop
// remaps this module's PATH check to the 'choco' binary (see
// engine.DefaultModuleCommandNames).
type chocolateyHandler struct{}

func (chocolateyHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	pkg := getString(item, "package")
	result, err := runner.Run("choco", []string{"list", "--local-only", pkg, "--exact", "--limit-output"})
	if err != nil {
		return false, nil //nolint:nilerr // any invocation failure here just means "not installed"
	}
	return strings.TrimSpace(result.Stdout+result.Stderr) != "", nil
}

func (chocolateyHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	pkg := getString(item, "package")
	state := itemState(item)
	if action == engine.ActionUninstall {
		return "choco uninstall " + pkg + " -y", nil
	}
	if state == "latest" {
		return "choco upgrade " + pkg + " -y", nil
	}
	return "choco install " + pkg + " -y", nil
}

func (chocolateyHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkg := getString(item, "package")
	state := itemState(item)
	verb := "install"
	if state == "latest" {
		verb = "upgrade"
	}
	args := []string{verb, pkg, "-y", "--accept-license"}
	if state != "latest" {
		if version := getString(item, "version"); version != "" {
			args = append(args, "--version="+version)
		}
	}
	result := runExternalCommand("choco", args)
	if result.RC != 0 {
		engine.Warn("choco %s %s exited with code %d", verb, pkg, result.RC)
	}
	return result, nil
}

func (chocolateyHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkg := getString(item, "package")
	result := runExternalCommand("choco", []string{"uninstall", pkg, "-y"})
	if result.RC != 0 {
		engine.Warn("choco uninstall %s exited with code %d", pkg, result.RC)
	}
	return result, nil
}
