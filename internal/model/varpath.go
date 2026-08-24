package model

import (
	"fmt"
	"strings"
)

// ParseVarOverride splits a '--var key=value' argument into its dotted
// key path and raw string value — ports docs/plans/notes.md's
// "--var key=value" flag. The key may contain '.'-separated segments
// addressing a nested vars map (e.g. "ssh.port"); the value is always
// kept as a literal string (no type coercion), matching the flag's
// simplest, least-surprising contract.
func ParseVarOverride(raw string) (path, value string, err error) {
	key, val, ok := strings.Cut(raw, "=")
	key = strings.TrimSpace(key)
	if !ok || key == "" {
		return "", "", fmt.Errorf("invalid --var %q: expected key=value", raw)
	}
	return key, val, nil
}

// SetVarPath sets value at a '.'-separated dotted path inside vars,
// creating any missing intermediate map[string]any levels along the way.
// A path segment that already holds a non-map value is overwritten with
// a fresh nested map rather than erroring, since a CLI override is
// explicitly the final, highest-precedence word on a var's value.
func SetVarPath(vars map[string]any, path string, value any) {
	segments := strings.Split(path, ".")
	m := vars
	for _, seg := range segments[:len(segments)-1] {
		next, ok := m[seg].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[seg] = next
		}
		m = next
	}
	m[segments[len(segments)-1]] = value
}
