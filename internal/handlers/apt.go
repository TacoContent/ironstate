package handlers

import (
	"os"
	"strings"
	"time"

	"github.com/TacoContent/ironstate/internal/engine"
)

// aptHandler wraps Debian/Ubuntu's apt-get, modeled on the core surface of
// ansible.builtin.apt (https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/apt_module.html):
//
//   - package (aliases 'name'): a single package name, OR a list of
//     names - the one package-manager handler in this codebase where a
//     leaf can name more than one package at once, since apt-get itself
//     accepts multiple package arguments in one invocation. Handlers that
//     can't batch (winget, chocolatey, ...) rely on the shared
//     'items'/'with' loop instead for the "install several" case.
//   - state: present/installed (default), absent/removed, latest,
//     build-dep, fixed (fix a broken dependency state via 'apt-get
//     install -f'). See engine.resolvePackageAction for how
//     'installed'/'removed'/'build-dep'/'fixed' are recognized alongside
//     this port's own present/absent/latest.
//   - update_cache (bool) + cache_valid_time (seconds): run 'apt-get
//     update' first, skipped when the apt cache is already newer than
//     cache_valid_time.
//   - upgrade: no (default) / yes / safe / full / dist.
//   - purge (bool): 'apt-get purge' instead of 'remove' when uninstalling
//     (or combined with autoremove).
//   - autoremove / autoclean (bool): run 'apt-get autoremove'/'autoclean'
//     as part of this leaf - the "task handler to clean up unused
//     packages" case; a leaf with no 'package' at all and one of these
//     (or update_cache/upgrade) set is a pure side-effect leaf that
//     always runs (see Test).
//   - install_recommends, only_upgrade, allow_unauthenticated, force
//     (bool): map to apt-get's own --no-install-recommends/
//     --install-recommends, --only-upgrade, --allow-unauthenticated,
//     --force-yes.
//
// apt-get install/remove/purge/autoremove/update all require root on a
// real system - this handler issues plain 'apt-get' commands and relies
// entirely on the engine's shared 'become'/sudo wrapping (see
// internal/exec/become.go) rather than handling elevation itself; set
// 'become: true' (or 'become: <user>') on the task.
type aptHandler struct{}

// aptPackageList reads 'package' (aliasing 'name', matching ansible) as
// either a single string or a list of strings.
func aptPackageList(item map[string]any) []string {
	v, ok := item["package"]
	if !ok {
		v = item["name"]
	}
	return stringSlice(v)
}

func isAptPackageInstalled(pkg string) bool {
	result, err := runner.Run("dpkg-query", []string{"-W", "-f=${Status}", pkg})
	if err != nil || result.RC != 0 {
		return false
	}
	return strings.Contains(result.Stdout, "install ok installed")
}

func (aptHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	pkgs := aptPackageList(item)
	if len(pkgs) == 0 {
		// No package named - this leaf exists purely to run
		// update_cache/upgrade/autoremove/autoclean side effects, which
		// have no "already satisfied" check of their own. Report "not
		// installed" so the default 'present' state's action resolves to
		// Install and this runs every dispatch (same idea as 'latest');
		// 'absent' has nothing to do either way.
		return itemState(item) == "absent", nil
	}
	if itemState(item) == "absent" {
		for _, pkg := range pkgs {
			if isAptPackageInstalled(pkg) {
				return true, nil
			}
		}
		return false, nil
	}
	for _, pkg := range pkgs {
		if !isAptPackageInstalled(pkg) {
			return false, nil
		}
	}
	return true, nil
}

// aptCacheValid reports whether the apt package index is fresher than
// cacheValidSeconds, mirroring ansible's cache_valid_time by checking the
// mtime of apt's own cache file rather than tracking our own stamp.
func aptCacheValid(cacheValidSeconds int) bool {
	if cacheValidSeconds <= 0 {
		return false
	}
	info, err := os.Stat("/var/cache/apt/pkgcache.bin")
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < time.Duration(cacheValidSeconds)*time.Second
}

func aptInstallFlags(item map[string]any) []string {
	var flags []string
	if v, ok := item["install_recommends"].(bool); ok {
		if v {
			flags = append(flags, "--install-recommends")
		} else {
			flags = append(flags, "--no-install-recommends")
		}
	}
	if getBool(item, "only_upgrade", false) {
		flags = append(flags, "--only-upgrade")
	}
	if getBool(item, "allow_unauthenticated", false) {
		flags = append(flags, "--allow-unauthenticated")
	}
	if getBool(item, "force", false) {
		flags = append(flags, "--force-yes")
	}
	return flags
}

// aptStep is one 'apt-get' invocation in a leaf's plan - a leaf can
// expand to several (update, then install, then autoremove, ...).
type aptStep struct {
	description string
	args        []string
}

// aptPlan builds the ordered list of apt-get invocations a leaf's config
// calls for, shared between Describe (joined for display) and Install/
// Uninstall (actually run in order, stopping at the first failure).
func aptPlan(item map[string]any, action engine.Action) []aptStep {
	var steps []aptStep
	pkgs := aptPackageList(item)

	if getBool(item, "update_cache", false) && !aptCacheValid(int(getFloat(item, "cache_valid_time", 0))) {
		steps = append(steps, aptStep{"update", []string{"update"}})
	}

	if action == engine.ActionUninstall {
		verb := "remove"
		if getBool(item, "purge", false) {
			verb = "purge"
		}
		if len(pkgs) > 0 {
			args := append([]string{verb, "-y"}, pkgs...)
			steps = append(steps, aptStep{verb + " " + strings.Join(pkgs, " "), args})
		}
	} else {
		state := itemState(item)
		switch {
		case len(pkgs) > 0 && state == "build-dep":
			args := append([]string{"build-dep", "-y"}, pkgs...)
			steps = append(steps, aptStep{"build-dep " + strings.Join(pkgs, " "), args})
		case len(pkgs) > 0:
			args := append([]string{"install", "-y"}, aptInstallFlags(item)...)
			if state == "fixed" {
				args = append(args, "-f")
			}
			args = append(args, pkgs...)
			steps = append(steps, aptStep{"install " + strings.Join(pkgs, " "), args})
		case state == "fixed":
			steps = append(steps, aptStep{"fix broken dependencies", []string{"install", "-y", "-f"}})
		}

		if upgrade := strings.ToLower(getStringOr(item, "upgrade", "no")); upgrade != "" && upgrade != "no" && upgrade != "false" {
			verb := "upgrade"
			if upgrade == "full" || upgrade == "dist" {
				verb = "dist-upgrade"
			}
			steps = append(steps, aptStep{verb, []string{verb, "-y"}})
		}
	}

	if getBool(item, "autoremove", false) {
		args := []string{"autoremove", "-y"}
		if getBool(item, "purge", false) {
			args = append(args, "--purge")
		}
		steps = append(steps, aptStep{"autoremove", args})
	}
	if getBool(item, "autoclean", false) {
		steps = append(steps, aptStep{"autoclean", []string{"autoclean"}})
	}

	return steps
}

func (aptHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	steps := aptPlan(item, action)
	if len(steps) == 0 {
		return "apt-get (nothing to do)", nil
	}
	descs := make([]string, len(steps))
	for i, s := range steps {
		descs[i] = "apt-get " + s.description
	}
	return strings.Join(descs, " && "), nil
}

func runAptPlan(item map[string]any, action engine.Action) engine.ExecResult {
	steps := aptPlan(item, action)
	var result engine.ExecResult
	for _, step := range steps {
		result = runExternalCommand("apt-get", step.args)
		if result.RC != 0 {
			engine.Warn("apt-get %s exited with code %d", step.description, result.RC)
			return result
		}
	}
	return result
}

func (aptHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return runAptPlan(item, engine.ActionInstall), nil
}

func (aptHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return runAptPlan(item, engine.ActionUninstall), nil
}
