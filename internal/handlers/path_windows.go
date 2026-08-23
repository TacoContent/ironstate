//go:build windows

package handlers

import (
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"

	"github.com/TacoContent/ironstate/internal/engine"
)

// pathHandler ports Handlers/Path.psm1: ensures directories are present
// on (or absent from) the current user's persistent PATH environment
// variable. Reads/writes the User-scope PATH via the registry directly
// (HKCU\Environment) - no admin required, matching every other handler in
// this codebase installing under '~/.local/bin' etc. Also patches the
// *current* process's PATH for entries it actually adds/removes, so later
// steps in the same run see them immediately.
//
// Deviation from Handlers/Path.psm1: 'scope' is accepted but only 'User'
// is actually implemented (Machine-scope would require admin and isn't
// exercised anywhere in this repo today - same treatment as other
// documented, deliberate v1 gaps in docs/plans/go-rewrite.md §5/§11).
type pathHandler struct{}

func getUserPathEntries() ([]string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE)
	if err != nil {
		return nil, err
	}
	defer func() { _ = k.Close() }()
	current, _, err := k.GetStringValue("PATH")
	if err != nil && err != registry.ErrNotExist {
		return nil, err
	}
	return splitPathEntries(current), nil
}

func setUserPathEntries(entries []string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()
	return k.SetExpandStringValue("PATH", strings.Join(entries, ";"))
}

func splitPathEntries(v string) []string {
	if v == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(v, ";") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func getWantedPaths(item map[string]any) []string {
	var out []string
	for _, raw := range asList(item["paths"]) {
		if s, ok := raw.(string); ok {
			out = append(out, resolvePath(s))
		}
	}
	return out
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (pathHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	wanted := getWantedPaths(item)
	if len(wanted) == 0 {
		return true, nil
	}
	current, err := getUserPathEntries()
	if err != nil {
		return false, err
	}
	for _, p := range wanted {
		if !containsStr(current, p) {
			return false, nil
		}
	}
	return true, nil
}

func (pathHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	paths := strings.Join(getWantedPaths(item), ", ")
	scope := getStringOr(item, "scope", "User")
	if action == engine.ActionUninstall {
		return "remove from PATH: " + paths + " (scope: " + scope + ")", nil
	}
	return "add to PATH: " + paths + " (scope: " + scope + ")", nil
}

func (pathHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	wanted := getWantedPaths(item)
	current, err := getUserPathEntries()
	if err != nil {
		return engine.ExecResult{}, err
	}

	updated := append([]string{}, current...)
	var added []string
	for _, p := range wanted {
		if !containsStr(updated, p) {
			updated = append(updated, p)
			added = append(added, p)
		}
	}

	if err := setUserPathEntries(updated); err != nil {
		return engine.ExecResult{}, err
	}
	for _, p := range added {
		_ = os.Setenv("PATH", os.Getenv("PATH")+";"+p)
	}
	return engine.ExecResult{}, nil
}

func (pathHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	unwanted := getWantedPaths(item)
	current, err := getUserPathEntries()
	if err != nil {
		return engine.ExecResult{}, err
	}

	var updated []string
	for _, p := range current {
		if !containsStr(unwanted, p) {
			updated = append(updated, p)
		}
	}
	if err := setUserPathEntries(updated); err != nil {
		return engine.ExecResult{}, err
	}

	var liveUpdated []string
	for _, p := range splitPathEntries(os.Getenv("PATH")) {
		if !containsStr(unwanted, p) {
			liveUpdated = append(liveUpdated, p)
		}
	}
	_ = os.Setenv("PATH", strings.Join(liveUpdated, ";"))
	return engine.ExecResult{}, nil
}
