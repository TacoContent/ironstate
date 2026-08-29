package handlers

import (
	"encoding/json"
	"os/exec"
	"runtime"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// scoopHandler wraps Scoop, Windows' user-scoped command-line installer,
// modeled on the core surface of community.windows.win_scoop
// (https://docs.ansible.com/projects/ansible/latest/collections/community/windows/win_scoop_module.html),
// plus this codebase's own 'state: latest' convention (see apt/homebrew):
//
//   - package (aliases 'name'): a single app name, OR a list of names -
//     'scoop install'/'scoop uninstall' both accept multiple apps in one
//     invocation, same batching apt-get/pacman/yum/apk/snap/flatpak
//     support.
//   - state: present/installed (default), absent/removed, latest
//     ('scoop update' only affects apps already installed, so 'latest'
//     tries update first and falls back to install for one that isn't
//     present yet - same fallback shape as homebrew/yum/snap/flatpak).
//   - global (bool): '-g' (install/update/uninstall in the machine-wide
//     scope instead of the current user's).
//   - architecture (string): '--arch=x' (e.g. '32bit'/'64bit'/'arm64'),
//     install only.
//
// Unlike winget/chocolatey, a scoop install is per-user by default and
// needs no elevation - only 'global: true' does, and Windows elevation
// support is limited (see internal/handlers/util.go's runExternalCommand
// warning for a non-default become user on Windows).
type scoopHandler struct{}

// scoopPackageList reads 'package' (aliasing 'name') as either a single
// string or a list of strings.
func scoopPackageList(item map[string]any) []string {
	v, ok := item["package"]
	if !ok {
		v = item["name"]
	}
	return stringSlice(v)
}

func isScoopPackageInstalled(pkg string) bool {
	result, err := runner.Run("scoop", []string{"list", pkg})
	if err != nil {
		return false //nolint:nilerr // any invocation failure here just means "not installed"
	}
	for _, line := range strings.Split(result.Stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if strings.EqualFold(fields[0], pkg) {
			return true
		}
	}
	return false
}

func (scoopHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	pkgs := scoopPackageList(item)
	if len(pkgs) == 0 {
		return itemState(item) == "absent", nil
	}
	if itemState(item) == "absent" {
		for _, pkg := range pkgs {
			if isScoopPackageInstalled(pkg) {
				return true, nil
			}
		}
		return false, nil
	}
	for _, pkg := range pkgs {
		if !isScoopPackageInstalled(pkg) {
			return false, nil
		}
	}
	return true, nil
}

func (scoopHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	pkgs := scoopPackageList(item)
	if len(pkgs) == 0 {
		return "scoop (nothing to do)", nil
	}
	if action == engine.ActionUninstall {
		return "scoop uninstall " + strings.Join(pkgs, " "), nil
	}
	if itemState(item) == "latest" {
		return "scoop update " + strings.Join(pkgs, " ") + " (installing if missing)", nil
	}
	return "scoop install " + strings.Join(pkgs, " "), nil
}

func (scoopHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkgs := scoopPackageList(item)
	if len(pkgs) == 0 {
		return engine.ExecResult{}, nil
	}
	var globalFlag []string
	if getBool(item, "global", false) {
		globalFlag = append(globalFlag, "-g")
	}
	var result engine.ExecResult
	if itemState(item) == "latest" {
		result = runExternalCommand("scoop", append(append([]string{"update"}, globalFlag...), pkgs...))
		if result.RC != 0 {
			args := append([]string{"install"}, globalFlag...)
			if arch := getString(item, "architecture"); arch != "" {
				args = append(args, "--arch="+arch)
			}
			args = append(args, pkgs...)
			result = runExternalCommand("scoop", args)
		}
	} else {
		args := append([]string{"install"}, globalFlag...)
		if arch := getString(item, "architecture"); arch != "" {
			args = append(args, "--arch="+arch)
		}
		args = append(args, pkgs...)
		result = runExternalCommand("scoop", args)
	}
	if result.RC != 0 {
		engine.Warn("scoop install/update %s exited with code %d", strings.Join(pkgs, " "), result.RC)
	}
	return result, nil
}

func (scoopHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkgs := scoopPackageList(item)
	if len(pkgs) == 0 {
		return engine.ExecResult{}, nil
	}
	var globalFlag []string
	if getBool(item, "global", false) {
		globalFlag = append(globalFlag, "-g")
	}
	result := runExternalCommand("scoop", append(append([]string{"uninstall"}, globalFlag...), pkgs...))
	if result.RC != 0 {
		engine.Warn("scoop uninstall %s exited with code %d", strings.Join(pkgs, " "), result.RC)
	}
	return result, nil
}

// ScanRole implements engine.ScanCapable - discovered packages seed
// roles/packages in a generated playbook (see internal/scan).
func (scoopHandler) ScanRole() string { return "roles/packages" }

// scoopExportFile mirrors the JSON 'scoop export' prints to stdout - only
// the 'apps' array's names matter here. Go's json package matches struct
// field names case-insensitively when no exact key match is found, so
// this one tag also covers scoop versions that emit lowercase 'name'.
type scoopExportFile struct {
	Apps []struct {
		Name string `json:"Name"`
	} `json:"apps"`
}

// Scan implements engine.ScanCapable: discovers installed apps via
// 'scoop export', which - unlike 'scoop list' - reports only apps the
// user explicitly installed, not their bucket-declared dependencies.
func (scoopHandler) Scan(ctx engine.Context) ([]engine.ScanItem, error) {
	if runtime.GOOS != "windows" {
		return nil, nil
	}
	if _, err := exec.LookPath("scoop"); err != nil {
		return nil, nil
	}
	result, err := runner.Run("scoop", []string{"export"})
	if err != nil || result.RC != 0 {
		return nil, nil //nolint:nilerr // scoop invocation failure just means nothing to report
	}
	var export scoopExportFile
	if err := json.Unmarshal([]byte(result.Stdout), &export); err != nil {
		return nil, nil //nolint:nilerr // unparseable export just means nothing to report
	}
	out := make([]engine.ScanItem, 0)
	for _, app := range export.Apps {
		name := strings.TrimSpace(app.Name)
		if name == "" {
			continue
		}
		out = append(out, engine.ScanItem{
			Module: "scoop",
			Name:   name,
			Config: map[string]any{"package": name, "state": "present"},
			Tags:   []string{"packages"},
		})
	}
	return out, nil
}
