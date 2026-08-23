package handlers

import (
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// wingetHandler ports Handlers/Winget.psm1 (Windows Package Manager).
type wingetHandler struct{}

func (wingetHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	pkg := getString(item, "package")
	result, err := runner.Run("winget", []string{"list", "--id", pkg, "--exact", "--accept-source-agreements"})
	if err != nil {
		return false, nil //nolint:nilerr // winget not being on PATH is filtered out before Test ever runs; any other invocation failure just means "not installed"
	}
	out := result.Stdout + result.Stderr
	return result.RC == 0 && !strings.Contains(out, "No installed package found"), nil
}

func (wingetHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	pkg := getString(item, "package")
	if action == engine.ActionUninstall {
		return "winget uninstall --id " + pkg + " --exact", nil
	}
	desc := "winget install --id " + pkg + " --exact"
	if source := getString(item, "source"); source != "" {
		desc += " --source " + source
	}
	if override := getString(item, "override"); override != "" {
		desc += " --override " + override
	}
	return desc, nil
}

func (wingetHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkg := getString(item, "package")
	args := []string{"install", "--id", pkg, "--exact", "--accept-source-agreements", "--accept-package-agreements"}
	if source := getString(item, "source"); source != "" {
		args = append(args, "--source", source)
	}
	if override := getString(item, "override"); override != "" {
		args = append(args, "--override", override)
	}
	result := runExternalCommand("winget", args)
	if result.RC != 0 {
		engine.Warn("winget install %s exited with code %d", pkg, result.RC)
	}
	return result, nil
}

func (wingetHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkg := getString(item, "package")
	result := runExternalCommand("winget", []string{"uninstall", "--id", pkg, "--exact"})
	if result.RC != 0 {
		engine.Warn("winget uninstall %s exited with code %d", pkg, result.RC)
	}
	return result, nil
}
