package filters

import (
	"crypto/sha1" //nolint:gosec // ports the 'sha1' filter's existing use as a deterministic content-hash (e.g. blockinfile marker names), not for security
	"encoding/hex"
	"fmt"
	"os"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/TacoContent/ironstate/internal/expr"
	"github.com/TacoContent/ironstate/internal/pathutil"
)

func registerBuiltins(r *Registry) {
	r.Register("default", filterDefault)
	r.Register("toggle", filterToggle)
	r.Register("ternary", filterTernary)
	r.Register("enabled", filterEnabled)
	r.Register("upper", filterUpper)
	r.Register("lower", filterLower)
	r.Register("trim", filterTrim)
	r.Register("quote", filterQuote)
	r.Register("length", filterLength)
	r.Register("concat", filterConcat)
	r.Register("join", filterJoin)
	r.Register("split", filterSplit)
	r.Register("prefix", filterPrefix)
	r.Register("dirname", filterDirname)
	r.Register("basename", filterBasename)
	r.Register("resolve", filterResolve)
	r.Register("exists", filterExists)
	r.Register("sha1", filterSHA1)
	r.Register("extension", filterExtension)
	r.Register("env", filterEnv)
	registerJSONFilters(r)
	registerLookupFilter(r)
}

// asStringSlice converts a value that may be a single string or a []any
// of scalars into a flat []string, mirroring how 'concat'/'prefix' accept
// either shape.
func asItems(value any) []any {
	if value == nil {
		return nil
	}
	if list, ok := value.([]any); ok {
		return list
	}
	return []any{value}
}

func toStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func filterDefault(value any, args []any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("'default' filter expects exactly 1 argument")
	}
	if value == nil {
		return args[0], nil
	}
	return value, nil
}

func filterEnv(value any, args []any) (any, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("'env' filter expects at least 1 argument. env(<key>, [default])")
	}
	if len(args) > 2 {
		return nil, fmt.Errorf("'env' filter expects at most 2 arguments. env(<key>, [default])")
	}
	key := toStr(args[0])
	if key == "" {
		return nil, fmt.Errorf("'env' filter requires a non-empty environment variable name")
	}
	if len(args) == 2 {
		def := args[1]
		val := os.Getenv(key)
		if val == "" {
			return def, nil
		}
		return val, nil
	}
	return os.Getenv(key), nil
}

func filterToggle(value any, args []any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("'toggle' filter expects exactly 1 argument")
	}
	if s, ok := value.(string); ok {
		return s, nil
	}
	return args[0], nil
}

func filterTernary(value any, args []any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("'ternary' filter expects exactly 2 arguments")
	}
	if expr.Truthy(value) {
		return args[0], nil
	}
	return args[1], nil
}

func filterEnabled(value any, args []any) (any, error) {
	current := value
	for _, keyArg := range args {
		if b, ok := current.(bool); ok {
			return b, nil
		}
		m, ok := current.(map[string]any)
		if !ok {
			return false, nil
		}
		v, present := m[toStr(keyArg)]
		if !present {
			return false, nil
		}
		current = v
	}
	if b, ok := current.(bool); ok {
		return b, nil
	}
	if _, ok := current.(map[string]any); ok {
		return true, nil
	}
	return false, nil
}

func filterUpper(value any, _ []any) (any, error) {
	if value == nil {
		return nil, nil
	}
	return strings.ToUpper(toStr(value)), nil
}

func filterLower(value any, _ []any) (any, error) {
	if value == nil {
		return nil, nil
	}
	return strings.ToLower(toStr(value)), nil
}

func filterTrim(value any, _ []any) (any, error) {
	if value == nil {
		return nil, nil
	}
	return strings.TrimSpace(toStr(value)), nil
}

func filterQuote(value any, args []any) (any, error) {
	if value == nil {
		return nil, nil
	}
	s := toStr(value)
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	q := `"`
	if len(args) == 1 {
		q = toStr(args[0])
	}
	return q + s + q, nil
}

func filterLength(value any, _ []any) (any, error) {
	if value == nil {
		return float64(0), nil
	}
	switch v := value.(type) {
	case string:
		return float64(utf8.RuneCountInString(v)), nil
	case []any:
		return float64(len(v)), nil
	default:
		return float64(0), nil
	}
}

func filterConcat(value any, args []any) (any, error) {
	if value == nil {
		return nil, nil
	}
	if len(args) < 1 {
		return nil, fmt.Errorf("'concat' filter expects at least 1 argument")
	}
	delimiter := toStr(args[0])
	extra := args[1:]

	var items []string
	for _, item := range asItems(value) {
		items = append(items, toStr(item))
	}
	for _, item := range extra {
		items = append(items, toStr(item))
	}
	return strings.Join(items, delimiter), nil
}

func filterJoin(value any, args []any) (any, error) {
	if value == nil {
		return nil, nil
	}
	if len(args) < 1 {
		return nil, fmt.Errorf("'join' filter expects at least 1 argument")
	}
	parts := []string{toStr(value)}
	for _, a := range args {
		parts = append(parts, toStr(a))
	}
	return pathutil.CombinePaths(parts), nil
}

func filterSplit(value any, args []any) (any, error) {
	if value == nil {
		return nil, nil
	}
	if len(args) != 1 {
		return nil, fmt.Errorf("'split' filter expects exactly 1 argument")
	}
	delimiter := toStr(args[0])
	parts := strings.Split(toStr(value), delimiter)
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	result := make([]any, len(parts))
	for i, p := range parts {
		result[i] = p
	}
	return result, nil
}

func filterPrefix(value any, args []any) (any, error) {
	if value == nil {
		return nil, nil
	}
	if len(args) != 1 {
		return nil, fmt.Errorf("'prefix' filter expects exactly 1 argument")
	}
	prefix := toStr(args[0])
	if s, ok := value.(string); ok {
		return prefix + " " + s, nil
	}
	items := asItems(value)
	result := make([]any, len(items))
	for i, item := range items {
		result[i] = prefix + " " + toStr(item)
	}
	return result, nil
}

// dotnetDirName/dotnetFileName mirror [System.IO.Path]::GetDirectoryName/
// GetFileName: split on the last path separator; GetDirectoryName returns
// "" (not ".") when there is none.
func dotnetDirName(p string) string {
	idx := strings.LastIndexAny(p, `/\`)
	if idx < 0 {
		return ""
	}
	return p[:idx]
}

func dotnetFileName(p string) string {
	idx := strings.LastIndexAny(p, `/\`)
	if idx < 0 {
		return p
	}
	return p[idx+1:]
}

func filterDirname(value any, _ []any) (any, error) {
	if value == nil {
		return nil, nil
	}
	return dotnetDirName(toStr(value)), nil
}

func filterBasename(value any, _ []any) (any, error) {
	if value == nil {
		return nil, nil
	}
	return dotnetFileName(toStr(value)), nil
}

func filterResolve(value any, args []any) (any, error) {
	if value == nil {
		return nil, nil
	}
	if len(args) > 0 {
		return nil, fmt.Errorf("'resolve' filter does not accept argument values")
	}
	return pathutil.ResolveUserPath(toStr(value)), nil
}

// dotnetExtension mirrors [System.IO.Path]::GetExtension: the substring
// from the last '.' in the final path segment (dot included), "" if
// there's no dot, or "" if the last character is a trailing dot.
func dotnetExtension(p string) string {
	base := dotnetFileName(p)
	idx := strings.LastIndex(base, ".")
	if idx < 0 || idx == len(base)-1 {
		return ""
	}
	return base[idx:]
}

func normalizeExtension(ext string) string {
	if strings.HasPrefix(ext, ".") {
		return ext
	}
	return "." + ext
}

// filterExtension ports the 'extension' PowerShell filter (Windows-only:
// non-Windows platforms have no ".exe"-style suffix convention, so the
// value passes through unchanged there). args[0] is one extension or a
// list of acceptable extensions (with or without the leading dot); if the
// value doesn't already end in one of them, args[1] - or, if omitted,
// args[0]'s first entry - is appended.
func filterExtension(value any, args []any) (any, error) {
	if value == nil {
		return nil, nil
	}
	if runtime.GOOS != "windows" {
		return value, nil
	}
	if len(args) < 1 {
		return value, nil
	}

	var rawExtensions []string
	for _, item := range asItems(args[0]) {
		rawExtensions = append(rawExtensions, toStr(item))
	}
	if len(rawExtensions) == 0 {
		return value, nil
	}

	rawExtToAdd := rawExtensions[0]
	if len(args) >= 2 {
		rawExtToAdd = toStr(args[1])
	}

	s := toStr(value)
	current := dotnetExtension(s)
	for _, ext := range rawExtensions {
		if strings.EqualFold(normalizeExtension(ext), current) {
			return s, nil
		}
	}
	return s + normalizeExtension(rawExtToAdd), nil
}

func filterExists(value any, args []any) (any, error) {
	expected := true
	for _, a := range args {
		if b, ok := a.(bool); ok {
			expected = b
		}
	}
	if value == nil {
		return !expected, nil
	}
	s, ok := value.(string)
	if !ok {
		return !expected, nil
	}
	if strings.TrimSpace(s) == "" {
		return !expected, nil
	}
	_, err := os.Stat(pathutil.ResolveUserPath(s))
	return expected == (err == nil), nil
}

func filterSHA1(value any, _ []any) (any, error) {
	if value == nil {
		return nil, nil
	}
	s, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("sha1 filter requires a string value")
	}
	sum := sha1.Sum([]byte(s)) //nolint:gosec // deterministic content-hash, not a security use
	return hex.EncodeToString(sum[:]), nil
}
