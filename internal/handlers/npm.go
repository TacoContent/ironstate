package handlers

import (
	"encoding/json"
	"os/exec"
	"runtime"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

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

// ScanRole implements engine.ScanCapable - discovered packages seed
// roles/packages in a generated playbook (see internal/scan).
func (npmHandler) ScanRole() string { return "roles/packages" }

// Scan implements engine.ScanCapable: discovers globally-installed npm
// packages - ports the scanning logic that used to live in
// internal/scan's packageScanner.
func (npmHandler) Scan(ctx engine.Context) ([]engine.ScanItem, error) {
	if runtime.GOOS == "windows" {
		return nil, nil
	}
	if _, err := exec.LookPath("npm"); err != nil {
		return nil, nil
	}
	result, err := runner.Run("npm", []string{"list", "-g", "--depth=0", "--json"})
	if err != nil {
		return nil, nil //nolint:nilerr // npm invocation failure just means nothing to report
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(result.Stdout), &data); err != nil {
		return nil, nil //nolint:nilerr // malformed output just means nothing to report
	}
	out := make([]engine.ScanItem, 0)
	deps, _ := data["dependencies"].(map[string]any)
	for name := range deps {
		if strings.TrimSpace(name) == "" || name == "npm" {
			continue
		}
		out = append(out, engine.ScanItem{
			Module: "npm",
			Name:   name,
			Config: map[string]any{"package": name, "state": "present"},
			Tags:   []string{"packages"},
		})
	}
	return out, nil
}
