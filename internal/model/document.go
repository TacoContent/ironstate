package model

import "fmt"

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
