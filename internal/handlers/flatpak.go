package handlers

import (
	"os/exec"
	"runtime"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// flatpakHandler wraps Flatpak, modeled on the core surface of
// community.general.flatpak (https://docs.ansible.com/projects/ansible/latest/collections/community/general/flatpak_module.html),
// plus this codebase's own 'state: latest' convention (see apt/homebrew):
//
//   - package (aliases 'name'): a single application ID, OR a list of
//     IDs - 'flatpak install'/'flatpak uninstall' both accept multiple
//     refs in one invocation, same batching apt-get/pacman/yum/apk/snap
//     support.
//   - state: present/installed (default), absent/removed, latest
//     ('flatpak update' only affects apps already installed, so 'latest'
//     tries update first and falls back to install for one that isn't
//     present yet - same fallback shape as homebrew/yum/snap).
//   - remote (string, default 'flathub'): which configured remote to
//     install from.
//   - method (string): 'user' installs into the per-user scope
//     ('--user'); anything else (the default) leaves scope unset,
//     matching flatpak's own system-wide default.
//
// A system-scope flatpak install/uninstall needs root on a real system -
// this handler issues plain 'flatpak' commands and relies entirely on the
// engine's shared 'become'/sudo wrapping (see internal/exec/become.go)
// rather than handling elevation itself; set 'become: true' (or
// 'become: <user>') on the task. A '--user' scoped task should omit
// 'become' - a per-user Flatpak install doesn't need root.
type flatpakHandler struct{}

// flatpakPackageList reads 'package' (aliasing 'name') as either a single
// string or a list of strings.
func flatpakPackageList(item map[string]any) []string {
	v, ok := item["package"]
	if !ok {
		v = item["name"]
	}
	return stringSlice(v)
}

func isFlatpakInstalled(pkg string) bool {
	result, err := runner.Run("flatpak", []string{"info", pkg})
	return err == nil && result.RC == 0
}

func (flatpakHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	pkgs := flatpakPackageList(item)
	if len(pkgs) == 0 {
		return itemState(item) == "absent", nil
	}
	if itemState(item) == "absent" {
		for _, pkg := range pkgs {
			if isFlatpakInstalled(pkg) {
				return true, nil
			}
		}
		return false, nil
	}
	for _, pkg := range pkgs {
		if !isFlatpakInstalled(pkg) {
			return false, nil
		}
	}
	return true, nil
}

// flatpakScopeFlag returns '--user' when method is 'user', else "" (leave
// flatpak's own system-wide default in place).
func flatpakScopeFlag(item map[string]any) string {
	if strings.EqualFold(getString(item, "method"), "user") {
		return "--user"
	}
	return ""
}

func (flatpakHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	pkgs := flatpakPackageList(item)
	if len(pkgs) == 0 {
		return "flatpak (nothing to do)", nil
	}
	if action == engine.ActionUninstall {
		return "flatpak uninstall " + strings.Join(pkgs, " "), nil
	}
	if itemState(item) == "latest" {
		return "flatpak update " + strings.Join(pkgs, " ") + " (installing if missing)", nil
	}
	remote := getStringOr(item, "remote", "flathub")
	return "flatpak install " + remote + " " + strings.Join(pkgs, " "), nil
}

func (flatpakHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkgs := flatpakPackageList(item)
	if len(pkgs) == 0 {
		return engine.ExecResult{}, nil
	}
	base := []string{"-y", "--noninteractive"}
	if scope := flatpakScopeFlag(item); scope != "" {
		base = append(base, scope)
	}
	var result engine.ExecResult
	if itemState(item) == "latest" {
		result = runExternalCommand("flatpak", append(append([]string{"update"}, base...), pkgs...))
		if result.RC != 0 {
			remote := getStringOr(item, "remote", "flathub")
			args := append(append([]string{"install"}, base...), remote)
			args = append(args, pkgs...)
			result = runExternalCommand("flatpak", args)
		}
	} else {
		remote := getStringOr(item, "remote", "flathub")
		args := append(append([]string{"install"}, base...), remote)
		args = append(args, pkgs...)
		result = runExternalCommand("flatpak", args)
	}
	if result.RC != 0 {
		engine.Warn("flatpak install/update %s exited with code %d", strings.Join(pkgs, " "), result.RC)
	}
	return result, nil
}

func (flatpakHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkgs := flatpakPackageList(item)
	if len(pkgs) == 0 {
		return engine.ExecResult{}, nil
	}
	args := []string{"uninstall", "-y", "--noninteractive"}
	if scope := flatpakScopeFlag(item); scope != "" {
		args = append(args, scope)
	}
	args = append(args, pkgs...)
	result := runExternalCommand("flatpak", args)
	if result.RC != 0 {
		engine.Warn("flatpak uninstall %s exited with code %d", strings.Join(pkgs, " "), result.RC)
	}
	return result, nil
}

// ScanRole implements engine.ScanCapable - discovered packages seed
// roles/packages in a generated playbook (see internal/scan).
func (flatpakHandler) ScanRole() string { return "roles/packages" }

// Scan implements engine.ScanCapable: discovers installed applications via
// 'flatpak list --app' - the '--app' filter excludes runtimes pulled in
// only as a dependency, mirroring 'apt-mark showmanual'/'brew leaves'.
func (flatpakHandler) Scan(ctx engine.Context) ([]engine.ScanItem, error) {
	if runtime.GOOS == "windows" {
		return nil, nil
	}
	if _, err := exec.LookPath("flatpak"); err != nil {
		return nil, nil
	}
	result, err := runner.Run("flatpak", []string{"list", "--app", "--columns=application"})
	if err != nil || result.RC != 0 {
		return nil, nil //nolint:nilerr // flatpak invocation failure just means nothing to report
	}
	out := make([]engine.ScanItem, 0)
	for _, line := range strings.Split(result.Stdout, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		out = append(out, engine.ScanItem{
			Module: "flatpak",
			Name:   name,
			Config: map[string]any{"package": name, "state": "present"},
			Tags:   []string{"packages"},
		})
	}
	return out, nil
}
