package handlers

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/TacoContent/ironstate/internal/engine"
)

// pacmanHandler wraps Arch Linux's pacman, modeled on the core surface of
// ansible.builtin.pacman (https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/pacman_module.html):
//
//   - package (aliases 'name'): a single package name, OR a list of
//     names - pacman accepts multiple package arguments in one '-S'/'-R'
//     invocation, same batching apt-get supports.
//   - state: present/installed (default), absent/removed, latest ('-S'
//     always resolves to the current repo version, so 'latest' just
//     forces the leaf to always run - see Test).
//   - update_cache (bool) + cache_valid_time (seconds): run 'pacman -Sy'
//     first, skipped when the local sync database is already newer than
//     cache_valid_time.
//   - upgrade (bool): run 'pacman -Syu' as part of this leaf - the "task
//     handler to fully upgrade the system" case.
//   - force (bool): add '--overwrite=*' to the install, for reinstalling
//     over files pacman thinks another package owns.
//   - recurse (bool): use '-Rs' instead of '-R' when uninstalling, so
//     dependencies no longer needed by anything else are removed too.
//   - nosave (bool): add the 'n' modifier ('-Rn'/'-Rsn') so pacman deletes
//     configuration files instead of saving them as '.pacsave'.
//
// pacman's sync/remove operations all require root on a real system -
// this handler issues plain 'pacman' commands and relies entirely on the
// engine's shared 'become'/sudo wrapping (see internal/exec/become.go)
// rather than handling elevation itself; set 'become: true' (or
// 'become: <user>') on the task.
type pacmanHandler struct{}

// pacmanPackageList reads 'package' (aliasing 'name') as either a single
// string or a list of strings.
func pacmanPackageList(item map[string]any) []string {
	v, ok := item["package"]
	if !ok {
		v = item["name"]
	}
	return stringSlice(v)
}

func isPacmanPackageInstalled(pkg string) bool {
	result, err := runner.Run("pacman", []string{"-Q", pkg})
	return err == nil && result.RC == 0
}

func (pacmanHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	pkgs := pacmanPackageList(item)
	if len(pkgs) == 0 {
		// No package named - this leaf exists purely to run
		// update_cache/upgrade side effects, which have no "already
		// satisfied" check of their own.
		return itemState(item) == "absent", nil
	}
	if itemState(item) == "absent" {
		for _, pkg := range pkgs {
			if isPacmanPackageInstalled(pkg) {
				return true, nil
			}
		}
		return false, nil
	}
	for _, pkg := range pkgs {
		if !isPacmanPackageInstalled(pkg) {
			return false, nil
		}
	}
	return true, nil
}

// pacmanCacheValid reports whether pacman's local sync database is fresher
// than cacheValidSeconds, mirroring apt's cache_valid_time by checking the
// mtime of pacman's own sync directory rather than tracking our own stamp.
func pacmanCacheValid(cacheValidSeconds int) bool {
	if cacheValidSeconds <= 0 {
		return false
	}
	info, err := os.Stat("/var/lib/pacman/sync")
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < time.Duration(cacheValidSeconds)*time.Second
}

// pacmanStep is one 'pacman' invocation in a leaf's plan - a leaf can
// expand to several (sync, then install, then system upgrade, ...).
type pacmanStep struct {
	description string
	args        []string
}

// pacmanPlan builds the ordered list of pacman invocations a leaf's config
// calls for, shared between Describe (joined for display) and Install/
// Uninstall (actually run in order, stopping at the first failure).
func pacmanPlan(item map[string]any, action engine.Action) []pacmanStep {
	var steps []pacmanStep
	pkgs := pacmanPackageList(item)

	if getBool(item, "update_cache", false) && !pacmanCacheValid(int(getFloat(item, "cache_valid_time", 0))) {
		steps = append(steps, pacmanStep{"sync", []string{"-Sy", "--noconfirm"}})
	}

	if action == engine.ActionUninstall {
		verb := "-R"
		if getBool(item, "recurse", false) {
			verb += "s"
		}
		if getBool(item, "nosave", false) {
			verb += "n"
		}
		if len(pkgs) > 0 {
			args := append([]string{verb, "--noconfirm"}, pkgs...)
			steps = append(steps, pacmanStep{verb + " " + strings.Join(pkgs, " "), args})
		}
	} else {
		if len(pkgs) > 0 {
			args := []string{"-S", "--noconfirm"}
			if getBool(item, "force", false) {
				args = append(args, "--overwrite=*")
			}
			args = append(args, pkgs...)
			steps = append(steps, pacmanStep{"-S " + strings.Join(pkgs, " "), args})
		}
		if getBool(item, "upgrade", false) {
			steps = append(steps, pacmanStep{"-Syu", []string{"-Syu", "--noconfirm"}})
		}
	}

	return steps
}

func (pacmanHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	steps := pacmanPlan(item, action)
	if len(steps) == 0 {
		return "pacman (nothing to do)", nil
	}
	descs := make([]string, len(steps))
	for i, s := range steps {
		descs[i] = "pacman " + s.description
	}
	return strings.Join(descs, " && "), nil
}

func runPacmanPlan(item map[string]any, action engine.Action) engine.ExecResult {
	steps := pacmanPlan(item, action)
	var result engine.ExecResult
	for _, step := range steps {
		result = runExternalCommand("pacman", step.args)
		if result.RC != 0 {
			engine.Warn("pacman %s exited with code %d", step.description, result.RC)
			return result
		}
	}
	return result
}

func (pacmanHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return runPacmanPlan(item, engine.ActionInstall), nil
}

func (pacmanHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return runPacmanPlan(item, engine.ActionUninstall), nil
}

// ScanRole implements engine.ScanCapable - discovered packages seed
// roles/packages in a generated playbook (see internal/scan).
func (pacmanHandler) ScanRole() string { return "roles/packages" }

// Scan implements engine.ScanCapable: discovers packages pacman has
// explicitly installed via 'pacman -Qe' - not 'pacman -Q', so packages
// pulled in only as another package's dependency are excluded (mirrors
// 'apt-mark showmanual'/'brew leaves').
func (pacmanHandler) Scan(ctx engine.Context) ([]engine.ScanItem, error) {
	if runtime.GOOS == "windows" {
		return nil, nil
	}
	if _, err := exec.LookPath("pacman"); err != nil {
		return nil, nil
	}
	result, err := runner.Run("pacman", []string{"-Qe"})
	if err != nil {
		return nil, nil //nolint:nilerr // pacman invocation failure just means nothing to report
	}
	out := make([]engine.ScanItem, 0)
	for _, line := range strings.Split(result.Stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		out = append(out, engine.ScanItem{
			Module: "pacman",
			Name:   name,
			Config: map[string]any{"package": name, "state": "present"},
			Tags:   []string{"packages"},
		})
	}
	return out, nil
}
