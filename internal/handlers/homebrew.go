package handlers

import "github.com/TacoContent/ironstate/internal/engine"

// homebrewHandler wraps Homebrew (brew) - formulae and casks on macOS and
// Linux. The dispatch loop remaps the 'homebrew' module's PATH check to
// the 'brew' binary (see engine.DefaultModuleCommandNames), matching
// chocolatey's 'choco' remap; 'brew' is also registered directly in
// handlers.All() as an alias module key (no remap needed there - the key
// already matches the binary name), since that's the more common name for
// the tool itself. Homebrew's top-level commands auto-detect formula vs.
// cask by name, so no separate flag is needed here.
type homebrewHandler struct{}

func (homebrewHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	pkg := getString(item, "package")
	result, err := runner.Run("brew", []string{"list", pkg})
	if err != nil {
		return false, nil //nolint:nilerr // any invocation failure here just means "not installed"
	}
	return result.RC == 0, nil
}

func (homebrewHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	pkg := getString(item, "package")
	if action == engine.ActionUninstall {
		return "brew uninstall " + pkg, nil
	}
	if itemState(item) == "latest" {
		return "brew upgrade " + pkg + " (installing if missing)", nil
	}
	return "brew install " + pkg, nil
}

func (homebrewHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkg := getString(item, "package")
	var result engine.ExecResult
	if itemState(item) == "latest" {
		result = runExternalCommand("brew", []string{"upgrade", pkg})
		if result.RC != 0 {
			result = runExternalCommand("brew", []string{"install", pkg})
		}
	} else {
		result = runExternalCommand("brew", []string{"install", pkg})
	}
	if result.RC != 0 {
		engine.Warn("brew install/upgrade %s exited with code %d", pkg, result.RC)
	}
	return result, nil
}

func (homebrewHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkg := getString(item, "package")
	result := runExternalCommand("brew", []string{"uninstall", pkg})
	if result.RC != 0 {
		engine.Warn("brew uninstall %s exited with code %d", pkg, result.RC)
	}
	return result, nil
}
