package handlers

import "github.com/TacoContent/ironstate/internal/engine"

// npmHandler ports Handlers/Npm.psm1 (Node global packages).
type npmHandler struct{}

func (npmHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	pkg := getString(item, "package")
	result, err := runner.Run("npm", []string{"list", "-g", pkg, "--depth=0"})
	if err != nil {
		return false, nil //nolint:nilerr // any invocation failure here just means "not installed"
	}
	return result.RC == 0, nil
}

func (npmHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	pkg := getString(item, "package")
	if action == engine.ActionUninstall {
		return "npm uninstall -g " + pkg, nil
	}
	return "npm install -g " + pkg, nil
}

func (npmHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkg := getString(item, "package")
	result := runExternalCommand("npm", []string{"install", "-g", pkg})
	if result.RC != 0 {
		engine.Warn("npm install -g %s exited with code %d", pkg, result.RC)
	}
	return result, nil
}

func (npmHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkg := getString(item, "package")
	result := runExternalCommand("npm", []string{"uninstall", "-g", pkg})
	if result.RC != 0 {
		engine.Warn("npm uninstall -g %s exited with code %d", pkg, result.RC)
	}
	return result, nil
}
