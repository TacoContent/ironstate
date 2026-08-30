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

// CoerceVarValue turns a --var override's raw string value into a real
// bool when it's exactly "true" or "false", leaving every other value as
// the literal string. This lets --var satisfy filters like 'enabled' that
// gate a leaf on a genuine bool (a package-id string such as
// "Eclipse.Temurin.21" must still read as an "off" scalar, so only the
// unambiguous true/false spellings are coerced - nothing else is guessed).
func CoerceVarValue(raw string) any {
	switch raw {
	case "true":
		return true
	case "false":
		return false
	default:
		return raw
	}
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
