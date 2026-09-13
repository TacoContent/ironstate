package model

import (
	"fmt"
	"strings"
)

// Plugin is one playbook-declared external handler plugin.
type Plugin struct {
	Namespace string
	Version   string
}

// Plugins parses a document's optional plugins list.
func Plugins(doc map[string]any) ([]Plugin, error) {
	raw, ok := doc["plugins"]
	if !ok || raw == nil {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("plugins must be a list")
	}
	plugins := make([]Plugin, 0, len(items))
	for _, rawItem := range items {
		use, ok := AsMap(rawItem)["use"].(string)
		if !ok {
			return nil, fmt.Errorf("plugin entry must contain a string use value")
		}
		plugin, err := ParsePlugin(use)
		if err != nil {
			return nil, err
		}
		plugins = append(plugins, plugin)
	}
	return plugins, nil
}

// ParsePlugin parses organization.plugin@version.
func ParsePlugin(value string) (Plugin, error) {
	parts := strings.Split(strings.TrimSpace(value), "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.Count(parts[0], ".") != 1 {
		return Plugin{}, fmt.Errorf("plugin %q must use organization.plugin@version", value)
	}
	return Plugin{Namespace: parts[0], Version: parts[1]}, nil
}
