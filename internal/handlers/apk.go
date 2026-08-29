package handlers

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/TacoContent/ironstate/internal/engine"
)

// apkHandler wraps Alpine Linux's apk, modeled on the core surface of
// community.general.apk (https://docs.ansible.com/projects/ansible/latest/collections/community/general/apk_module.html):
//
//   - package (aliases 'name'): a single package name, OR a list of
//     names - apk accepts multiple package arguments in one 'add'/'del'
//     invocation, same batching apt-get/pacman/yum support.
//   - state: present/installed (default), absent/removed, latest ('apk
//     add -u' both installs and upgrades to the newest available version
//     in one call, so unlike yum this needs no separate fallback step).
//   - update_cache (bool) + cache_valid_time (seconds): run 'apk update'
//     first, skipped when apk's own cache directory is already newer
//     than cache_valid_time.
//   - upgrade (bool): run 'apk upgrade' (every installed package) as
//     part of this leaf - the "task handler to fully upgrade the system"
//     case, same idea as apt's/pacman's own 'upgrade' flag.
//   - repository (string or list): '--repository=x', repeated per entry,
//     for pulling a package from a repo not enabled in /etc/apk/repositories.
//   - no_cache (bool): '--no-cache' (skip apk's local package cache
//     entirely).
//
// apk's add/del/update/upgrade all require root on a real system - this
// handler issues plain 'apk' commands and relies entirely on the engine's
// shared 'become'/sudo wrapping (see internal/exec/become.go) rather than
// handling elevation itself; set 'become: true' (or 'become: <user>') on
// the task.
type apkHandler struct{}

// apkPackageList reads 'package' (aliasing 'name') as either a single
// string or a list of strings.
func apkPackageList(item map[string]any) []string {
	v, ok := item["package"]
	if !ok {
		v = item["name"]
	}
	return stringSlice(v)
}

func isApkPackageInstalled(pkg string) bool {
	result, err := runner.Run("apk", []string{"info", "-e", pkg})
	return err == nil && result.RC == 0 && strings.TrimSpace(result.Stdout) != ""
}

func (apkHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	pkgs := apkPackageList(item)
	if len(pkgs) == 0 {
		// No package named - this leaf exists purely to run
		// update_cache/upgrade side effects, which have no "already
		// satisfied" check of their own.
		return itemState(item) == "absent", nil
	}
	if itemState(item) == "absent" {
		for _, pkg := range pkgs {
			if isApkPackageInstalled(pkg) {
				return true, nil
			}
		}
		return false, nil
	}
	for _, pkg := range pkgs {
		if !isApkPackageInstalled(pkg) {
			return false, nil
		}
	}
	return true, nil
}

// apkCacheValid reports whether apk's local cache is fresher than
// cacheValidSeconds, mirroring apt's cache_valid_time by checking the
// mtime of apk's own cache directory rather than tracking our own stamp.
func apkCacheValid(cacheValidSeconds int) bool {
	if cacheValidSeconds <= 0 {
		return false
	}
	info, err := os.Stat("/var/cache/apk")
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < time.Duration(cacheValidSeconds)*time.Second
}

// apkStep is one 'apk' invocation in a leaf's plan - a leaf can expand to
// several (update, then add/del, then a full upgrade, ...).
type apkStep struct {
	description string
	args        []string
}

// apkPlan builds the ordered list of apk invocations a leaf's config
// calls for, shared between Describe (joined for display) and Install/
// Uninstall (actually run in order, stopping at the first failure).
func apkPlan(item map[string]any, action engine.Action) []apkStep {
	var steps []apkStep
	pkgs := apkPackageList(item)
	noCache := getBool(item, "no_cache", false)

	if getBool(item, "update_cache", false) && !apkCacheValid(int(getFloat(item, "cache_valid_time", 0))) {
		steps = append(steps, apkStep{"update", []string{"update"}})
	}

	if action == engine.ActionUninstall {
		if len(pkgs) > 0 {
			args := []string{"del"}
			if noCache {
				args = append(args, "--no-cache")
			}
			args = append(args, pkgs...)
			steps = append(steps, apkStep{"del " + strings.Join(pkgs, " "), args})
		}
	} else {
		if len(pkgs) > 0 {
			args := []string{"add"}
			if itemState(item) == "latest" {
				args = append(args, "-u")
			}
			for _, repo := range stringSlice(item["repository"]) {
				args = append(args, "--repository", repo)
			}
			if noCache {
				args = append(args, "--no-cache")
			}
			args = append(args, pkgs...)
			steps = append(steps, apkStep{"add " + strings.Join(pkgs, " "), args})
		}
		if getBool(item, "upgrade", false) {
			steps = append(steps, apkStep{"upgrade", []string{"upgrade"}})
		}
	}

	return steps
}

func (apkHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	steps := apkPlan(item, action)
	if len(steps) == 0 {
		return "apk (nothing to do)", nil
	}
	descs := make([]string, len(steps))
	for i, s := range steps {
		descs[i] = "apk " + s.description
	}
	return strings.Join(descs, " && "), nil
}

func runApkPlan(item map[string]any, action engine.Action) engine.ExecResult {
	steps := apkPlan(item, action)
	var result engine.ExecResult
	for _, step := range steps {
		result = runExternalCommand("apk", step.args)
		if result.RC != 0 {
			engine.Warn("apk %s exited with code %d", step.description, result.RC)
			return result
		}
	}
	return result
}

func (apkHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return runApkPlan(item, engine.ActionInstall), nil
}

func (apkHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return runApkPlan(item, engine.ActionUninstall), nil
}

// ScanRole implements engine.ScanCapable - discovered packages seed
// roles/packages in a generated playbook (see internal/scan).
func (apkHandler) ScanRole() string { return "roles/packages" }

// apkWorldPackageName strips a /etc/apk/world entry's version-constraint
// operator (=, <, >, ~) and repository/provider tag (@, %) suffix, leaving
// just the bare package name.
func apkWorldPackageName(line string) string {
	if idx := strings.IndexAny(line, "=<>~@%"); idx >= 0 {
		line = line[:idx]
	}
	return strings.TrimSpace(line)
}

// Scan implements engine.ScanCapable: discovers packages explicitly
// requested by the user by reading /etc/apk/world directly - apk's own
// record of top-level dependencies (excludes anything pulled in only as
// another package's dependency), the Alpine equivalent of 'apt-mark
// showmanual'/'brew leaves'/'pacman -Qe'.
func (apkHandler) Scan(ctx engine.Context) ([]engine.ScanItem, error) {
	if runtime.GOOS == "windows" {
		return nil, nil
	}
	if _, err := exec.LookPath("apk"); err != nil {
		return nil, nil
	}
	data, err := os.ReadFile("/etc/apk/world")
	if err != nil {
		return nil, nil //nolint:nilerr // no world file just means nothing to report
	}
	out := make([]engine.ScanItem, 0)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		name := apkWorldPackageName(line)
		if name == "" {
			continue
		}
		out = append(out, engine.ScanItem{
			Module: "apk",
			Name:   name,
			Config: map[string]any{"package": name, "state": "present"},
			Tags:   []string{"packages"},
		})
	}
	return out, nil
}
