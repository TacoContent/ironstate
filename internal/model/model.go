// Package model holds the generic, dynamically-typed document shape every
// other package works with — YAML decoded into map[string]any / []any /
// scalars, mirroring how the PowerShell implementation treats
// ConvertFrom-Yaml's [ordered] hashtables + IList duck-typing. A strict Go
// struct per module would fight the '${{ }}'/'when' substitution model
// (docs/plans/go-rewrite.md §4.2), so this stays intentionally loose.
package model

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Unmarshal parses raw YAML bytes into the generic Value shape: mappings
// become map[string]any, sequences become []any, scalars stay as
// string/float64/bool/nil. An empty (or comments/whitespace-only)
// document parses to an empty mapping rather than nil, matching
// Packages.psm1's Import-PackagesFile treating a null document the same
// as an empty task list.
func Unmarshal(data []byte) (any, error) {
	var out any
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return map[string]any{}, nil
	}
	return normalize(out), nil
}

// normalize defensively converts any map[any]any (which a YAML library
// could in principle still produce for a non-string-keyed mapping) into
// map[string]any, recursing through slices/maps — see
// docs/plans/go-rewrite.md §11's "YAML library behavioral differences" risk.
func normalize(v any) any {
	switch val := v.(type) {
	case map[string]any:
		for k, item := range val {
			val[k] = normalize(item)
		}
		return val
	case map[any]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			out[fmt.Sprintf("%v", k)] = normalize(item)
		}
		return out
	case []any:
		for i, item := range val {
			val[i] = normalize(item)
		}
		return val
	default:
		return val
	}
}

// DeepCopy recursively clones maps/slices produced by Unmarshal — needed
// wherever one source template is materialized multiple times (loops) so
// each copy can be mutated independently, mirroring Common.psm1's
// Copy-DeepData. Scalars (string/float64/bool/nil) are copied by value
// already and pass through unchanged.
func DeepCopy(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			out[k] = DeepCopy(item)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = DeepCopy(item)
		}
		return out
	default:
		return val
	}
}

// Prop safely reads key from item when item is a map[string]any,
// mirroring Common.psm1's Get-Prop — every optional field goes through
// this rather than a direct type assertion, since a leaf's module dict
// varies shape per module.
func Prop(item any, key string) (any, bool) {
	m, ok := item.(map[string]any)
	if !ok {
		return nil, false
	}
	v, ok := m[key]
	return v, ok
}

// PropOr is Prop with a fallback default when the key is absent.
func PropOr(item any, key string, def any) any {
	if v, ok := Prop(item, key); ok {
		return v
	}
	return def
}

// AsMap returns v as a map[string]any, or an empty map if it isn't one.
func AsMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

// AsList returns v as a []any, wrapping a lone non-list value as a
// single-element list (matching PowerShell's automatic '@(...)' array
// wrapping of a scalar), or an empty list for nil.
func AsList(v any) []any {
	switch val := v.(type) {
	case nil:
		return nil
	case []any:
		return val
	default:
		return []any{val}
	}
}

// AsStringSlice converts v (a []any of strings, a lone string, or nil)
// into a []string, skipping any non-string element — used for 'tags'.
func AsStringSlice(v any) []string {
	list := AsList(v)
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
