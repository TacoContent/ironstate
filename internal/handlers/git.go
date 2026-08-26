package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// gitHandler manages a checkout at dest using the local git CLI,
// modeled on Ansible's git module with a focused option set.
type gitHandler struct{}

func gitRepo(item map[string]any) string {
	return strings.TrimSpace(getString(item, "repo"))
}

func gitDest(item map[string]any) string {
	return resolvePath(getString(item, "dest"))
}

func gitRef(item map[string]any) string {
	v := strings.TrimSpace(getStringOr(item, "ref", "HEAD"))
	if v == "" {
		return "HEAD"
	}
	return v
}

func gitUpdate(item map[string]any) bool {
	return getBool(item, "update", true)
}

func gitClone(item map[string]any) bool {
	return getBool(item, "clone", true)
}

func gitRecursive(item map[string]any) bool {
	return getBool(item, "recursive", true)
}

func gitForce(item map[string]any) bool {
	return getBool(item, "force", false)
}

func gitSingleBranch(item map[string]any) bool {
	return getBool(item, "single_branch", false)
}

func gitDepth(item map[string]any) int {
	if v, ok := item["depth"]; ok {
		switch t := v.(type) {
		case int:
			if t > 0 {
				return t
			}
		case float64:
			if int(t) > 0 {
				return int(t)
			}
		case string:
			parsed, err := strconv.Atoi(strings.TrimSpace(t))
			if err == nil && parsed > 0 {
				return parsed
			}
		}
	}
	return 0
}

func gitWorktreePresent(dest string) bool {
	if dest == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(dest, ".git"))
	return err == nil
}

func gitResolveRef(dest, ref string) (string, error) {
	res, err := runner.Run("git", []string{"-C", dest, "rev-parse", "--verify", ref + "^{commit}"})
	if err != nil {
		return "", err
	}
	if res.RC != 0 {
		return "", fmt.Errorf("git rev-parse --verify %s failed with code %d", ref, res.RC)
	}
	return strings.TrimSpace(res.Stdout), nil
}

func gitOriginURL(dest string) string {
	res, err := runner.Run("git", []string{"-C", dest, "config", "--get", "remote.origin.url"})
	if err != nil || res.RC != 0 {
		return ""
	}
	return strings.TrimSpace(res.Stdout)
}

func gitMatchesRepo(dest, repo string) bool {
	if repo == "" {
		return true
	}
	origin := gitOriginURL(dest)
	if origin == "" {
		return false
	}
	return origin == repo
}

func gitSubmoduleUpdate(item map[string]any, dest string) engine.ExecResult {
	if !gitRecursive(item) {
		return engine.ExecResult{}
	}
	args := []string{"-C", dest, "submodule", "update", "--init", "--recursive"}
	if d := gitDepth(item); d > 0 {
		args = append(args, "--depth", strconv.Itoa(d))
	}
	return runExternalCommand("git", args)
}

func (gitHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	dest := gitDest(item)
	if dest == "" {
		return false, nil
	}

	if itemState(item) == "absent" {
		return fileExists(dest), nil
	}

	if !gitWorktreePresent(dest) {
		return false, nil
	}

	if !gitMatchesRepo(dest, gitRepo(item)) {
		return false, nil
	}

	if itemState(item) == "latest" && gitUpdate(item) {
		return false, nil
	}

	v := gitRef(item)
	if strings.EqualFold(v, "HEAD") {
		return true, nil
	}

	head, err := gitResolveRef(dest, "HEAD")
	if err != nil {
		return false, nil //nolint:nilerr // probe failure => treat as not-converged
	}
	target, err := gitResolveRef(dest, v)
	if err != nil {
		return false, nil //nolint:nilerr // probe failure => treat as not-converged
	}

	return head == target, nil
}

func (gitHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	dest := gitDest(item)
	repo := gitRepo(item)
	if action == engine.ActionUninstall {
		return "remove git checkout at " + dest, nil
	}
	return fmt.Sprintf("git checkout %s -> %s (%s)", repo, dest, gitRef(item)), nil
}

func (gitHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	repo := gitRepo(item)
	dest := gitDest(item)
	if repo == "" {
		return engine.ExecResult{}, fmt.Errorf("git handler requires 'repo'")
	}
	if dest == "" {
		return engine.ExecResult{}, fmt.Errorf("git handler requires 'dest'")
	}

	cloneNeeded := !gitWorktreePresent(dest)
	if fileExists(dest) && !gitWorktreePresent(dest) {
		if !gitForce(item) {
			return engine.ExecResult{}, fmt.Errorf("git dest exists but is not a git worktree: %s (set force: true to replace)", dest)
		}
		if err := os.RemoveAll(dest); err != nil { //nolint:gosec // path is authored YAML content, same trust boundary as this handler's other file operations
			return engine.ExecResult{}, err
		}
		cloneNeeded = true
	}

	if !cloneNeeded && !gitMatchesRepo(dest, repo) {
		if !gitForce(item) {
			return engine.ExecResult{}, fmt.Errorf("git remote mismatch at %s (set force: true to reclone)", dest)
		}
		if err := os.RemoveAll(dest); err != nil { //nolint:gosec // path is authored YAML content, same trust boundary as this handler's other file operations
			return engine.ExecResult{}, err
		}
		cloneNeeded = true
	}

	last := engine.ExecResult{}
	v := gitRef(item)

	if cloneNeeded {
		if !gitClone(item) {
			return engine.ExecResult{}, fmt.Errorf("git dest is missing and 'clone' is false: %s", dest)
		}
		if err := ensureParentDir(dest); err != nil {
			return engine.ExecResult{}, err
		}
		args := []string{"clone"}
		if d := gitDepth(item); d > 0 {
			args = append(args, "--depth", strconv.Itoa(d))
		}
		if gitSingleBranch(item) {
			args = append(args, "--single-branch")
		}
		if !strings.EqualFold(v, "HEAD") {
			args = append(args, "--branch", v)
		}
		args = append(args, repo, dest)
		last = runExternalCommand("git", args)
		if last.RC != 0 {
			engine.Warn("git clone %s -> %s exited with code %d", repo, dest, last.RC)
			return last, nil
		}
	} else if gitUpdate(item) {
		args := []string{"-C", dest, "fetch", "--tags", "--prune"}
		if d := gitDepth(item); d > 0 {
			args = append(args, "--depth", strconv.Itoa(d))
		}
		last = runExternalCommand("git", args)
		if last.RC != 0 {
			engine.Warn("git fetch at %s exited with code %d", dest, last.RC)
			return last, nil
		}
	}

	if !strings.EqualFold(v, "HEAD") {
		args := []string{"-C", dest, "checkout"}
		if gitForce(item) {
			args = append(args, "--force")
		}
		args = append(args, v)
		last = runExternalCommand("git", args)
		if last.RC != 0 {
			engine.Warn("git checkout %s at %s exited with code %d", v, dest, last.RC)
			return last, nil
		}
	} else if gitUpdate(item) {
		last = runExternalCommand("git", []string{"-C", dest, "pull", "--ff-only"})
		if last.RC != 0 {
			engine.Warn("git pull at %s exited with code %d", dest, last.RC)
			return last, nil
		}
	}

	subResult := gitSubmoduleUpdate(item, dest)
	if subResult.RC != 0 {
		engine.Warn("git submodule update at %s exited with code %d", dest, subResult.RC)
		return subResult, nil
	}

	if last.RC != 0 || last.Stdout != "" || last.Stderr != "" || len(last.StdoutLines) > 0 || len(last.StderrLines) > 0 {
		return last, nil
	}
	return subResult, nil
}

func (gitHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	dest := gitDest(item)
	if dest == "" || !fileExists(dest) {
		return engine.ExecResult{}, nil
	}
	if err := os.RemoveAll(dest); err != nil { //nolint:gosec // path is authored YAML content, same trust boundary as this handler's other file operations
		return engine.ExecResult{}, err
	}
	return engine.ExecResult{}, nil
}
