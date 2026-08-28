package handlers

import (
	"os/exec"
	"runtime"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

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

// ScanRole implements engine.ScanCapable - discovered packages seed
// roles/packages in a generated playbook (see internal/scan).
func (homebrewHandler) ScanRole() string { return "roles/packages" }

// Scan implements engine.ScanCapable: discovers formulae/casks Homebrew
// has installed - ports the scanning logic that used to live in
// internal/scan's packageScanner.
func (homebrewHandler) Scan(ctx engine.Context) ([]engine.ScanItem, error) {
	if runtime.GOOS == "windows" {
		return nil, nil
	}
	if _, err := exec.LookPath("brew"); err != nil {
		return nil, nil
	}
	out := make([]engine.ScanItem, 0)
	// 'brew leaves' - not 'brew list --formula' - so formulae pulled in
	// only as another formula's dependency (e.g. libde265 under
	// handbrake) are excluded; only what the user actually asked to
	// install shows up.
	if result, err := runner.Run("brew", []string{"leaves"}); err == nil {
		out = append(out, brewListToItems(result.Stdout)...)
	}
	if result, err := runner.Run("brew", []string{"list", "--cask", "-1"}); err == nil {
		out = append(out, brewListToItems(result.Stdout)...)
	}
	return out, nil
}

func brewListToItems(out string) []engine.ScanItem {
	items := make([]engine.ScanItem, 0)
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		items = append(items, engine.ScanItem{
			Module: "homebrew",
			Name:   name,
			Config: map[string]any{"package": name, "state": "present"},
			Tags:   []string{"packages"},
		})
	}
	return items
}
