package handlers

import (
	"os/exec"
	"runtime"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// snapHandler wraps snapd's snap CLI, modeled on the core surface of
// ansible.builtin.snap (https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/snap_module.html),
// plus this codebase's own 'state: latest' convention (see apt/homebrew):
//
//   - package (aliases 'name'): a single snap name, OR a list of names -
//     'snap install'/'snap remove' both accept multiple names in one
//     invocation, same batching apt-get/pacman/yum/apk support.
//   - state: present/installed (default), absent/removed, latest ('snap
//     refresh' only affects snaps already installed, so 'latest' tries
//     refresh first and falls back to install for one that isn't present
//     yet - same fallback shape as homebrew/pipx/yum).
//   - classic (bool): '--classic' (classic confinement, required by
//     snaps that need broader system access than strict confinement
//     allows).
//   - channel (string): '--channel=x' (defaults to snapd's own 'stable'
//     when unset).
//
// snap install/remove/refresh all require root on a real system - this
// handler issues plain 'snap' commands and relies entirely on the
// engine's shared 'become'/sudo wrapping (see internal/exec/become.go)
// rather than handling elevation itself; set 'become: true' (or
// 'become: <user>') on the task.
type snapHandler struct{}

// snapPackageList reads 'package' (aliasing 'name') as either a single
// string or a list of strings.
func snapPackageList(item map[string]any) []string {
	v, ok := item["package"]
	if !ok {
		v = item["name"]
	}
	return stringSlice(v)
}

func isSnapInstalled(pkg string) bool {
	result, err := runner.Run("snap", []string{"list", pkg})
	return err == nil && result.RC == 0
}

func (snapHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	pkgs := snapPackageList(item)
	if len(pkgs) == 0 {
		return itemState(item) == "absent", nil
	}
	if itemState(item) == "absent" {
		for _, pkg := range pkgs {
			if isSnapInstalled(pkg) {
				return true, nil
			}
		}
		return false, nil
	}
	for _, pkg := range pkgs {
		if !isSnapInstalled(pkg) {
			return false, nil
		}
	}
	return true, nil
}

// snapFlags builds the '--classic'/'--channel=x' flags shared by
// install/refresh.
func snapFlags(item map[string]any) []string {
	var flags []string
	if getBool(item, "classic", false) {
		flags = append(flags, "--classic")
	}
	if channel := getString(item, "channel"); channel != "" {
		flags = append(flags, "--channel="+channel)
	}
	return flags
}

func (snapHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	pkgs := snapPackageList(item)
	if len(pkgs) == 0 {
		return "snap (nothing to do)", nil
	}
	if action == engine.ActionUninstall {
		return "snap remove " + strings.Join(pkgs, " "), nil
	}
	if itemState(item) == "latest" {
		return "snap refresh " + strings.Join(pkgs, " ") + " (installing if missing)", nil
	}
	return "snap install " + strings.Join(pkgs, " "), nil
}

func (snapHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkgs := snapPackageList(item)
	if len(pkgs) == 0 {
		return engine.ExecResult{}, nil
	}
	flags := snapFlags(item)
	var result engine.ExecResult
	if itemState(item) == "latest" {
		result = runExternalCommand("snap", append(append([]string{"refresh"}, flags...), pkgs...))
		if result.RC != 0 {
			result = runExternalCommand("snap", append(append([]string{"install"}, flags...), pkgs...))
		}
	} else {
		result = runExternalCommand("snap", append(append([]string{"install"}, flags...), pkgs...))
	}
	if result.RC != 0 {
		engine.Warn("snap install/refresh %s exited with code %d", strings.Join(pkgs, " "), result.RC)
	}
	return result, nil
}

func (snapHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkgs := snapPackageList(item)
	if len(pkgs) == 0 {
		return engine.ExecResult{}, nil
	}
	result := runExternalCommand("snap", append([]string{"remove"}, pkgs...))
	if result.RC != 0 {
		engine.Warn("snap remove %s exited with code %d", strings.Join(pkgs, " "), result.RC)
	}
	return result, nil
}

// ScanRole implements engine.ScanCapable - discovered packages seed
// roles/packages in a generated playbook (see internal/scan).
func (snapHandler) ScanRole() string { return "roles/packages" }

// Scan implements engine.ScanCapable: discovers installed snaps via
// 'snap list'. Unlike apt/homebrew/pacman/apk, snapd doesn't distinguish
// explicitly-requested snaps from ones pulled in as a dependency (e.g. a
// 'core'/'base'/'gnome-*' content snap), so - like yum - this reports
// every installed snap rather than a curated "leaves" subset.
func (snapHandler) Scan(ctx engine.Context) ([]engine.ScanItem, error) {
	if runtime.GOOS == "windows" {
		return nil, nil
	}
	if _, err := exec.LookPath("snap"); err != nil {
		return nil, nil
	}
	result, err := runner.Run("snap", []string{"list"})
	if err != nil || result.RC != 0 {
		return nil, nil //nolint:nilerr // snap invocation failure just means nothing to report
	}
	out := make([]engine.ScanItem, 0)
	lines := strings.Split(result.Stdout, "\n")
	for i, line := range lines {
		if i == 0 {
			continue // header row ("Name Version Rev Tracking Publisher Notes")
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		out = append(out, engine.ScanItem{
			Module: "snap",
			Name:   name,
			Config: map[string]any{"package": name, "state": "present"},
			Tags:   []string{"packages"},
		})
	}
	return out, nil
}
