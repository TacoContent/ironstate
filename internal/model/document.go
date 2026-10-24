package model

import (
	"fmt"
	"maps"
)

// TaskList normalizes a loaded document's root into a plain task list:
// the explicit '{ tasks: [...] }' form, or the document itself when it's
// already a bare list (implicit form) — ports Tasks.psm1's Get-TaskList.
func TaskList(doc any) ([]any, error) {
	switch v := doc.(type) {
	case map[string]any:
		if tasks, ok := v["tasks"]; ok {
			return AsList(tasks), nil
		}
		return []any{}, nil
	case []any:
		return v, nil
	case nil:
		return []any{}, nil
	default:
		return nil, fmt.Errorf("document root must be a '{ tasks: [...] }' mapping or a bare list of tasks/actions")
	}
}

// Vars returns doc's top-level 'vars' mapping, or an empty map if doc
// isn't a mapping or has none.
func Vars(doc any) map[string]any {
	m, ok := doc.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return AsMap(m["vars"])
}

// Tags returns doc's top-level file 'tags', or nil if doc isn't a mapping
// or has none.
func Tags(doc any) []string {
	m, ok := doc.(map[string]any)
	if !ok {
		return nil
	}
	return AsStringSlice(m["tags"])
}

// Envs returns doc's top-level 'envs' mapping, or an empty map if doc
// isn't a mapping or has none.
func Envs(doc any) map[string]any {
	m, ok := doc.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return AsMap(m["envs"])
}

// ScopedVars exposes package and site variables under the same namespace as
// their bare names. Site values replace same-named package values wholesale.
func ScopedVars(packageVars, siteVars map[string]any) map[string]any {
	result := make(map[string]any, len(packageVars)+len(siteVars))
	maps.Copy(result, packageVars)
	maps.Copy(result, siteVars)
	return result
}

// MergeMaps recursively overlays mappings; other values are replaced.
// Inputs are not modified, but unchanged nested values may be shared.
func MergeMaps(base, overlay map[string]any) map[string]any {
	result := make(map[string]any, len(base)+len(overlay))
	maps.Copy(result, base)
	for k, v := range overlay {
		if baseVal, ok := result[k].(map[string]any); ok {
			if overlayVal, ok := v.(map[string]any); ok {
				result[k] = MergeMaps(baseVal, overlayVal)
				continue
			}
		}
		result[k] = v
	}
	return result
}
