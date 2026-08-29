package handlers

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/TacoContent/ironstate/internal/engine"
)

// yumHandler wraps RHEL/CentOS/Fedora's yum, modeled on the core surface of
// ansible.builtin.yum (https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/yum_module.html):
//
//   - package (aliases 'name'): a single package name, OR a list of
//     names - yum accepts multiple package arguments in one 'install'/
//     'remove' invocation, same batching apt-get/pacman support.
//   - state: present/installed (default), absent/removed, latest ('yum
//     update' only upgrades what's already installed, so 'latest' tries
//     'update' first and falls back to 'install' for a package that
//     isn't present yet - same fallback shape as homebrew/pipx).
//   - update_cache (bool) + cache_valid_time (seconds): run 'yum
//     makecache' first, skipped when yum's own cache directory is
//     already newer than cache_valid_time.
//   - enablerepo / disablerepo (string or list): '--enablerepo=x'/
//     '--disablerepo=x', repeated per entry.
//   - exclude (string or list): '--exclude=x', repeated per entry.
//   - disable_gpg_check (bool): '--nogpgcheck'.
//
// yum's install/remove/makecache all require root on a real system - this
// handler issues plain 'yum' commands and relies entirely on the engine's
// shared 'become'/sudo wrapping (see internal/exec/become.go) rather than
// handling elevation itself; set 'become: true' (or 'become: <user>') on
// the task.
type yumHandler struct{}

// yumPackageList reads 'package' (aliasing 'name') as either a single
// string or a list of strings.
func yumPackageList(item map[string]any) []string {
	v, ok := item["package"]
	if !ok {
		v = item["name"]
	}
	return stringSlice(v)
}

func isYumPackageInstalled(pkg string) bool {
	result, err := runner.Run("rpm", []string{"-q", pkg})
	return err == nil && result.RC == 0
}

func (yumHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	pkgs := yumPackageList(item)
	if len(pkgs) == 0 {
		// No package named - this leaf exists purely to run a
		// update_cache side effect, which has no "already satisfied"
		// check of its own.
		return itemState(item) == "absent", nil
	}
	if itemState(item) == "absent" {
		for _, pkg := range pkgs {
			if isYumPackageInstalled(pkg) {
				return true, nil
			}
		}
		return false, nil
	}
	for _, pkg := range pkgs {
		if !isYumPackageInstalled(pkg) {
			return false, nil
		}
	}
	return true, nil
}

// yumCacheValid reports whether yum's local cache is fresher than
// cacheValidSeconds, mirroring apt's cache_valid_time by checking the
// mtime of yum's own cache directory rather than tracking our own stamp.
func yumCacheValid(cacheValidSeconds int) bool {
	if cacheValidSeconds <= 0 {
		return false
	}
	info, err := os.Stat("/var/cache/yum")
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < time.Duration(cacheValidSeconds)*time.Second
}

// yumRepoFlags builds the '--enablerepo=x'/'--disablerepo=x'/
// '--exclude=x'/'--nogpgcheck' flags common to yum's install/update.
func yumRepoFlags(item map[string]any) []string {
	var flags []string
	for _, repo := range stringSlice(item["enablerepo"]) {
		flags = append(flags, "--enablerepo="+repo)
	}
	for _, repo := range stringSlice(item["disablerepo"]) {
		flags = append(flags, "--disablerepo="+repo)
	}
	for _, pkg := range stringSlice(item["exclude"]) {
		flags = append(flags, "--exclude="+pkg)
	}
	if getBool(item, "disable_gpg_check", false) {
		flags = append(flags, "--nogpgcheck")
	}
	return flags
}

func (yumHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	var parts []string
	if getBool(item, "update_cache", false) && !yumCacheValid(int(getFloat(item, "cache_valid_time", 0))) {
		parts = append(parts, "yum makecache")
	}
	pkgs := yumPackageList(item)
	switch {
	case len(pkgs) == 0:
		if len(parts) == 0 {
			return "yum (nothing to do)", nil
		}
	case action == engine.ActionUninstall:
		parts = append(parts, "yum remove -y "+strings.Join(pkgs, " "))
	case itemState(item) == "latest":
		parts = append(parts, "yum update -y "+strings.Join(pkgs, " ")+" (installing if missing)")
	default:
		parts = append(parts, "yum install -y "+strings.Join(pkgs, " "))
	}
	return strings.Join(parts, " && "), nil
}

func (yumHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	var result engine.ExecResult
	if getBool(item, "update_cache", false) && !yumCacheValid(int(getFloat(item, "cache_valid_time", 0))) {
		result = runExternalCommand("yum", []string{"makecache"})
		if result.RC != 0 {
			engine.Warn("yum makecache exited with code %d", result.RC)
			return result, nil
		}
	}
	pkgs := yumPackageList(item)
	if len(pkgs) == 0 {
		return result, nil
	}
	flags := yumRepoFlags(item)
	if itemState(item) == "latest" {
		args := append(append([]string{"update", "-y"}, flags...), pkgs...)
		result = runExternalCommand("yum", args)
		if result.RC != 0 {
			args = append(append([]string{"install", "-y"}, flags...), pkgs...)
			result = runExternalCommand("yum", args)
		}
	} else {
		args := append(append([]string{"install", "-y"}, flags...), pkgs...)
		result = runExternalCommand("yum", args)
	}
	if result.RC != 0 {
		engine.Warn("yum install/update %s exited with code %d", strings.Join(pkgs, " "), result.RC)
	}
	return result, nil
}

func (yumHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkgs := yumPackageList(item)
	if len(pkgs) == 0 {
		return engine.ExecResult{}, nil
	}
	result := runExternalCommand("yum", append([]string{"remove", "-y"}, pkgs...))
	if result.RC != 0 {
		engine.Warn("yum remove %s exited with code %d", strings.Join(pkgs, " "), result.RC)
	}
	return result, nil
}

// ScanRole implements engine.ScanCapable - discovered packages seed
// roles/packages in a generated playbook (see internal/scan).
func (yumHandler) ScanRole() string { return "roles/packages" }

// Scan implements engine.ScanCapable: discovers packages explicitly
// requested by the user via 'yum repoquery --userinstalled' - the closest
// yum-family equivalent to 'apt-mark showmanual'/'brew leaves' (excludes
// packages pulled in only as a dependency). Unlike apt/homebrew, this
// relies on the repoquery plugin (bundled with dnf-backed yum on
// RHEL8+/Fedora, or available separately via yum-utils on older systems)
// - if it's missing, this reports nothing rather than falling back to
// listing every installed package (which would include dependencies).
func (yumHandler) Scan(ctx engine.Context) ([]engine.ScanItem, error) {
	if runtime.GOOS == "windows" {
		return nil, nil
	}
	if _, err := exec.LookPath("yum"); err != nil {
		return nil, nil
	}
	result, err := runner.Run("yum", []string{"repoquery", "--userinstalled", "--qf", "%{name}"})
	if err != nil || result.RC != 0 {
		return nil, nil //nolint:nilerr // repoquery unavailable/failing just means nothing to report
	}
	out := make([]engine.ScanItem, 0)
	for _, line := range strings.Split(result.Stdout, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		out = append(out, engine.ScanItem{
			Module: "yum",
			Name:   name,
			Config: map[string]any{"package": name, "state": "present"},
			Tags:   []string{"packages"},
		})
	}
	return out, nil
}
