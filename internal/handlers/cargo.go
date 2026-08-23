package handlers

import (
	"regexp"

	"github.com/TacoContent/ironstate/internal/engine"
)

// cargoHandler ports Handlers/Cargo.psm1 (Rust crates).
type cargoHandler struct{}

func (cargoHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	pkg := getString(item, "package")
	result, err := runner.Run("cargo", []string{"install", "--list"})
	if err != nil {
		return false, nil //nolint:nilerr // any invocation failure here just means "not installed"
	}
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(pkg) + `\s+v`)
	return re.MatchString(result.Stdout), nil
}

func (cargoHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	pkg := getString(item, "package")
	if action == engine.ActionUninstall {
		return "cargo uninstall " + pkg, nil
	}
	return "cargo install " + pkg + " --force", nil
}

func (cargoHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkg := getString(item, "package")
	result := runExternalCommand("cargo", []string{"install", pkg, "--force"})
	if result.RC != 0 {
		engine.Warn("cargo install %s exited with code %d", pkg, result.RC)
	}
	return result, nil
}

func (cargoHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkg := getString(item, "package")
	result := runExternalCommand("cargo", []string{"uninstall", pkg})
	if result.RC != 0 {
		engine.Warn("cargo uninstall %s exited with code %d", pkg, result.RC)
	}
	return result, nil
}
