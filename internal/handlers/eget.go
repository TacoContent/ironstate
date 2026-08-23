package handlers

import (
	"os"
	"regexp"

	"github.com/TacoContent/ironstate/internal/engine"
)

// egetHandler ports Handlers/Eget.psm1 (GitHub release binaries via eget).
type egetHandler struct{}

var egetToArgPattern = regexp.MustCompile(`^--to=(.+)$`)

func egetExpandedArgs(item map[string]any) []string {
	var out []string
	for _, raw := range asList(item["args"]) {
		s, ok := raw.(string)
		if !ok {
			continue
		}
		if m := egetToArgPattern.FindStringSubmatch(s); m != nil {
			out = append(out, "--to="+resolvePath(m[1]))
		} else {
			out = append(out, s)
		}
	}
	return out
}

func egetTargetPath(item map[string]any) string {
	for _, arg := range egetExpandedArgs(item) {
		if m := egetToArgPattern.FindStringSubmatch(arg); m != nil {
			return m[1]
		}
	}
	return ""
}

func (egetHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	target := egetTargetPath(item)
	if target == "" {
		return false, nil
	}
	return fileExists(target), nil
}

func (egetHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	pkg := getString(item, "package")
	if action == engine.ActionUninstall {
		return "remove " + egetTargetPath(item), nil
	}
	desc := "eget " + pkg
	for _, a := range egetExpandedArgs(item) {
		desc += " " + a
	}
	return desc, nil
}

func (egetHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkg := getString(item, "package")
	args := append([]string{pkg}, egetExpandedArgs(item)...)
	result := runExternalCommand("eget", args)
	if result.RC != 0 {
		engine.Warn("eget %s exited with code %d", pkg, result.RC)
	}
	return result, nil
}

func (egetHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	target := egetTargetPath(item)
	if target != "" && fileExists(target) {
		_ = os.Remove(target)
	}
	return engine.ExecResult{}, nil
}
