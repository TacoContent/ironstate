package handlers

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
	ironexec "github.com/TacoContent/ironstate/internal/exec"
	"github.com/TacoContent/ironstate/internal/pathutil"
)

// getString reads a string field, or "" if absent/wrong type.
func getString(item map[string]any, key string) string {
	s, _ := item[key].(string)
	return s
}

// getStringOr reads a string field, falling back to def if absent/empty.
func getStringOr(item map[string]any, key, def string) string {
	if s, ok := item[key].(string); ok && s != "" {
		return s
	}
	return def
}

// getBool reads a bool field, or def if absent/wrong type.
func getBool(item map[string]any, key string, def bool) bool {
	if b, ok := item[key].(bool); ok {
		return b
	}
	return def
}

// getMap reads a nested mapping field, or nil if absent/wrong type.
func getMap(item map[string]any, key string) map[string]any {
	m, _ := item[key].(map[string]any)
	return m
}

// getFloat reads a numeric field (YAML numbers decode as float64 or int),
// or def if absent/wrong type.
func getFloat(item map[string]any, key string, def float64) float64 {
	switch v := item[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	default:
		return def
	}
}

// stringSlice filters asList(v) down to its string entries, dropping
// anything else - used for id lists ('for', etc.) where a stray non-string
// entry should just be ignored rather than crashing a type assertion.
func stringSlice(v any) []string {
	var out []string
	for _, raw := range asList(v) {
		if s, ok := raw.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// asList wraps v into a []any the same way model.AsList does (a lone
// scalar becomes a single-element list, nil becomes an empty list) -
// duplicated here rather than importing internal/model, since these
// handlers only ever deal with already-flattened leaf Items, not raw YAML
// document trees.
func asList(v any) []any {
	switch val := v.(type) {
	case nil:
		return nil
	case []any:
		return val
	default:
		return []any{val}
	}
}

// itemState ports Common.psm1's Get-ItemState: the present/absent/latest
// state machine's input, defaulting to 'present'.
func itemState(item map[string]any) string {
	return getStringOr(item, "state", "present")
}

// resolvePath expands '~' then joins/normalizes - the common
// "read a path field, ready for os.* calls" idiom every file-shaped
// handler needs.
func resolvePath(path string) string {
	return pathutil.ResolveUserPath(path)
}

func ensureParentDir(path string) error {
	parent := filepath.Dir(path)
	if parent == "" || parent == "." {
		return nil
	}
	if _, err := os.Stat(parent); err == nil {
		return nil
	}
	return os.MkdirAll(parent, 0o755) //nolint:gosec // matches ironstate.ps1's own directories (not secrets), no tighter mode intended
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// firstNonEmptyLine mirrors Get-ItemLabel's '$command -split "\n" | Select
// -First 1' fallback for a shell-shaped item with no better label.
func firstNonEmptyLine(s string) string {
	line := strings.SplitN(s, "\n", 2)[0]
	return strings.TrimSpace(line)
}

// itemLabel ports Common.psm1's Get-ItemLabel: package, then name, then
// dest, then path, then script, then the first line of command, else
// '<unknown>'.
func itemLabel(item map[string]any) string {
	for _, key := range []string{"package", "name", "dest", "path", "script"} {
		if s, ok := item[key].(string); ok && s != "" {
			return s
		}
	}
	if cmd, ok := item["command"].(string); ok && cmd != "" {
		return firstNonEmptyLine(cmd)
	}
	return "<unknown>"
}

// describeLabel picks Name if the dispatch loop supplied one, else falls
// back to itemLabel(item) - the common Describe/Install pattern of
// "$Name ?? Get-ItemLabel".
func describeLabel(item map[string]any, name string) string {
	if name != "" {
		return name
	}
	return itemLabel(item)
}

// runner backs every package-manager handler's CLI invocations -
// overridable for tests, matching runPwshCommand's pattern.
var runner ironexec.Runner = ironexec.Default

// runExternalCommand ports Common.psm1's Invoke-ExternalCommand: runs exe,
// echoing captured stdout (Info)/stderr (Warn) after the command finishes,
// then returns the normalized engine.ExecResult. Every package-manager
// (and other CLI-backed) handler's mutating command goes through here, so
// this is also the one place a leaf's 'become' directive actually takes
// effect - see ironexec.CurrentBecome/WrapForBecome, set ambiently by
// internal/engine around each Install/Uninstall call. A leaf with no
// 'become' sees an empty ironexec.Become, which WrapForBecome passes
// straight through unchanged.
func runExternalCommand(exe string, args []string) engine.ExecResult {
	become := ironexec.CurrentBecome()
	if become.Enabled && runtime.GOOS == "windows" && become.User != "" && !strings.EqualFold(become.User, "root") {
		engine.Warn("become: Windows elevation only supports the default Administrator identity via sudo.exe/UAC; ignoring become user %q", become.User)
	}
	exe, args, err := ironexec.WrapForBecome(become, exe, args)
	if err != nil {
		engine.Warn("%s", err)
		return engine.ExecResult{RC: 1, Stderr: err.Error(), StderrLines: []string{err.Error()}}
	}
	result, err := runner.Run(exe, args)
	if err != nil {
		engine.Warn("%s: %v", exe, err)
		return engine.ExecResult{RC: 1, Stderr: err.Error(), StderrLines: []string{err.Error()}}
	}
	for _, line := range result.StdoutLines {
		engine.Info("%s", line)
	}
	for _, line := range result.StderrLines {
		engine.Warn("%s", line)
	}
	return engine.ExecResult{
		RC:          result.RC,
		Stdout:      result.Stdout,
		StdoutLines: result.StdoutLines,
		Stderr:      result.Stderr,
		StderrLines: result.StderrLines,
	}
}
