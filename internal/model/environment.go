package model

import (
	"fmt"
	"reflect"
)

// NormalizeEnvironment combines the aliases at one document/task boundary.
// Provider objects are atomic values, not recursively merged declarations.
func NormalizeEnvironment(item map[string]any) error {
	values := map[string]any{}
	present := false
	for _, alias := range []string{"environment", "envs"} {
		raw, ok := item[alias]
		if !ok {
			continue
		}
		present = true
		mapping, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be a mapping of environment variable names to values", alias)
		}
		for key, value := range mapping {
			if previous, exists := values[key]; exists && !reflect.DeepEqual(previous, value) {
				return fmt.Errorf("environment and envs define conflicting values for %q", key)
			}
			values[key] = value
		}
	}
	if present {
		item["envs"] = values
		delete(item, "environment")
	}
	return nil
}

// NormalizeDocumentEnvironment visits only document/task boundaries, never
// module configuration (cron modules have their own environment property).
func NormalizeDocumentEnvironment(doc any) error {
	if root, ok := doc.(map[string]any); ok {
		if err := NormalizeEnvironment(root); err != nil {
			return err
		}
	}
	list, err := TaskList(doc)
	if err != nil {
		return err
	}
	return normalizeTaskEnvironments(list)
}

func normalizeTaskEnvironments(list []any) error {
	for _, raw := range list {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if err := NormalizeEnvironment(item); err != nil {
			return fmt.Errorf("task %q: %w", item["name"], err)
		}
		if err := normalizeTaskEnvironments(AsList(item["actions"])); err != nil {
			return err
		}
		if async, ok := item["async"].(map[string]any); ok {
			if err := normalizeTaskEnvironments(AsList(async["tasks"])); err != nil {
				return err
			}
		}
	}
	return nil
}
