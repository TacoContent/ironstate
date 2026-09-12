package handlers

import (
	"os"
	"regexp"

	"github.com/TacoContent/ironstate/internal/engine"
)

// GitHub release binaries via xget.
type xgetHandler struct{}

var xgetToArgPattern = regexp.MustCompile(`^--to=(.+)$`)

func xgetExpandedArgs(item map[string]any) []string {
	var out []string
	for _, raw := range asList(item["args"]) {
		s, ok := raw.(string)
		if !ok {
			continue
		}
		if m := xgetToArgPattern.FindStringSubmatch(s); m != nil {
			out = append(out, "--to="+resolvePath(m[1]))
		} else {
			out = append(out, s)
		}
	}
	return out
}

func xgetTargetPath(item map[string]any) string {
	for _, arg := range xgetExpandedArgs(item) {
		if m := xgetToArgPattern.FindStringSubmatch(arg); m != nil {
			return m[1]
		}
	}
	return ""
}

func (xgetHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	target := xgetTargetPath(item)
	if target == "" {
		return false, nil
	}
	return fileExists(target), nil
}

func (xgetHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	pkg := getString(item, "package")
	if action == engine.ActionUninstall {
		return "remove " + xgetTargetPath(item), nil
	}
	desc := "eget " + pkg
	for _, a := range xgetExpandedArgs(item) {
		desc += " " + a
	}
	return desc, nil
}

func (xgetHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkg := getString(item, "package")
	args := append([]string{pkg}, xgetExpandedArgs(item)...)
	result := runExternalCommand("eget", args)
	if result.RC != 0 {
		engine.Warn("eget %s exited with code %d", pkg, result.RC)
	}
	return result, nil
}

func (xgetHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	target := xgetTargetPath(item)
	if target != "" && fileExists(target) {
		_ = os.Remove(target)
	}
	return engine.ExecResult{}, nil
}
