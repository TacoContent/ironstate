package handlers

import (
	"os/exec"
	"runtime"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// macportsHandler wraps MacPorts (the 'port' CLI) on macOS, modeled after
// homebrewHandler's shape since both are macOS package managers with a
// similar surface, plus this codebase's own 'state: latest' convention:
//
//   - package (aliases 'name'): a single port name, OR a list of names -
//     'port install'/'port uninstall' both accept multiple ports in one
//     invocation, same batching apt-get/pacman/yum/apk/snap/flatpak/
//     scoop support.
//   - state: present/installed (default), absent/removed, latest ('port
//     upgrade' only affects ports already installed, so 'latest' tries
//     upgrade first and falls back to install for one that isn't present
//     yet - same fallback shape as homebrew/yum/snap/flatpak/scoop).
//   - update_cache (bool): run 'port selfupdate' first - refreshes
//     MacPorts' own ports tree/database. Unlike apt/pacman/yum/apk,
//     there's no 'cache_valid_time' here: MacPorts' install prefix (and
//     so its ports-tree location) varies by installation, so there's no
//     single well-known path to stat like /var/cache/apt.
//
// The dispatch loop remaps this module's PATH check to the 'port' binary
// (see engine.DefaultModuleCommandNames). port install/uninstall/
// selfupdate/upgrade all require root on a real system - this handler
// issues plain 'port' commands and relies entirely on the engine's shared
// 'become'/sudo wrapping (see internal/exec/become.go) rather than
// handling elevation itself; set 'become: true' (or 'become: <user>') on
// the task.
type macportsHandler struct{}

// macportsPackageList reads 'package' (aliasing 'name') as either a
// single string or a list of strings.
func macportsPackageList(item map[string]any) []string {
	v, ok := item["package"]
	if !ok {
		v = item["name"]
	}
	return stringSlice(v)
}

func isMacportsInstalled(pkg string) bool {
	result, err := runner.Run("port", []string{"-q", "installed", pkg})
	if err != nil {
		return false //nolint:nilerr // any invocation failure here just means "not installed"
	}
	out := strings.TrimSpace(result.Stdout)
	return out != "" && !strings.Contains(out, "None of the specified ports are installed")
}

func (macportsHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	pkgs := macportsPackageList(item)
	if len(pkgs) == 0 {
		return itemState(item) == "absent", nil
	}
	if itemState(item) == "absent" {
		for _, pkg := range pkgs {
			if isMacportsInstalled(pkg) {
				return true, nil
			}
		}
		return false, nil
	}
	for _, pkg := range pkgs {
		if !isMacportsInstalled(pkg) {
			return false, nil
		}
	}
	return true, nil
}

func (macportsHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	var parts []string
	if getBool(item, "update_cache", false) {
		parts = append(parts, "port selfupdate")
	}
	pkgs := macportsPackageList(item)
	switch {
	case len(pkgs) == 0:
		if len(parts) == 0 {
			return "port (nothing to do)", nil
		}
	case action == engine.ActionUninstall:
		parts = append(parts, "port uninstall "+strings.Join(pkgs, " "))
	case itemState(item) == "latest":
		parts = append(parts, "port upgrade "+strings.Join(pkgs, " ")+" (installing if missing)")
	default:
		parts = append(parts, "port install "+strings.Join(pkgs, " "))
	}
	return strings.Join(parts, " && "), nil
}

func (macportsHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	var result engine.ExecResult
	if getBool(item, "update_cache", false) {
		result = runExternalCommand("port", []string{"selfupdate"})
		if result.RC != 0 {
			engine.Warn("port selfupdate exited with code %d", result.RC)
			return result, nil
		}
	}
	pkgs := macportsPackageList(item)
	if len(pkgs) == 0 {
		return result, nil
	}
	if itemState(item) == "latest" {
		result = runExternalCommand("port", append([]string{"upgrade"}, pkgs...))
		if result.RC != 0 {
			result = runExternalCommand("port", append([]string{"install"}, pkgs...))
		}
	} else {
		result = runExternalCommand("port", append([]string{"install"}, pkgs...))
	}
	if result.RC != 0 {
		engine.Warn("port install/upgrade %s exited with code %d", strings.Join(pkgs, " "), result.RC)
	}
	return result, nil
}

func (macportsHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkgs := macportsPackageList(item)
	if len(pkgs) == 0 {
		return engine.ExecResult{}, nil
	}
	result := runExternalCommand("port", append([]string{"uninstall"}, pkgs...))
	if result.RC != 0 {
		engine.Warn("port uninstall %s exited with code %d", strings.Join(pkgs, " "), result.RC)
	}
	return result, nil
}

// ScanRole implements engine.ScanCapable - discovered packages seed
// roles/packages in a generated playbook (see internal/scan).
func (macportsHandler) ScanRole() string { return "roles/packages" }

// Scan implements engine.ScanCapable: discovers ports explicitly
// requested by the user via 'port echo requested' - MacPorts' own
// leaf-request tracking, the same idea as 'apt-mark showmanual'/
// 'brew leaves'/'pacman -Qe' (excludes ports pulled in only as another
// port's dependency).
func (macportsHandler) Scan(ctx engine.Context) ([]engine.ScanItem, error) {
	// Unlike apt/homebrew's own GOOS gate (exclude Windows only), this
	// checks for 'darwin' specifically - MacPorts is macOS-only, so
	// there's no other non-Windows OS worth trying 'port' on.
	if runtime.GOOS != "darwin" {
		return nil, nil
	}
	if _, err := exec.LookPath("port"); err != nil {
		return nil, nil
	}
	result, err := runner.Run("port", []string{"echo", "requested"})
	if err != nil || result.RC != 0 {
		return nil, nil //nolint:nilerr // port invocation failure just means nothing to report
	}
	out := make([]engine.ScanItem, 0)
	for _, line := range strings.Split(result.Stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		out = append(out, engine.ScanItem{
			Module: "macports",
			Name:   name,
			Config: map[string]any{"package": name, "state": "present"},
			Tags:   []string{"packages"},
		})
	}
	return out, nil
}
