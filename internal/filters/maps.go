package filters

import (
	"encoding/json"
	"fmt"

	"github.com/TacoContent/ironstate/internal/model"
)

func filterMap(value any, args []any) (any, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("'map' filter expects no arguments; pipe in a map, JSON object, or key/value pairs")
	}
	switch v := value.(type) {
	case nil:
		return map[string]any{}, nil
	case map[string]any:
		return model.DeepCopy(v), nil
	case string:
		var decoded any
		if err := json.Unmarshal([]byte(v), &decoded); err != nil {
			return nil, fmt.Errorf("'map' filter: invalid JSON object: %w", err)
		}
		m, ok := decoded.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("'map' filter requires a JSON object")
		}
		return m, nil
	case []any:
		result := make(map[string]any, len(v))
		for i, item := range v {
			pair, ok := item.([]any)
			if !ok || len(pair) != 2 {
				return nil, fmt.Errorf("'map' filter: item %d must be a [key, value] pair", i)
			}
			key, ok := pair[0].(string)
			if !ok {
				return nil, fmt.Errorf("'map' filter: item %d key must be a string", i)
			}
			result[key] = model.DeepCopy(pair[1])
		}
		return result, nil
	default:
		return nil, fmt.Errorf("'map' filter requires a map, JSON object, or key/value pairs, got %T", value)
	}
}

func filterMerge(value any, args []any) (any, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("'merge' filter expects at least 1 map or list argument")
	}
	kind := value
	if kind == nil {
		for _, arg := range args {
			if arg != nil {
				kind = arg
				break
			}
		}
	}
	if _, list := kind.([]any); list {
		result := []any{}
		if value != nil {
			result = append(result, value.([]any)...)
		}
		for index, arg := range args {
			if arg == nil {
				continue
			}
			entries, ok := arg.([]any)
			if !ok {
				return nil, fmt.Errorf("'merge' filter argument %d must be a list or null, got %T", index+1, arg)
			}
			result = append(result, entries...)
		}
		return model.DeepCopy(result), nil
	}
	var base map[string]any
	if value != nil {
		var ok bool
		base, ok = value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("'merge' filter requires a map or null input, got %T", value)
		}
	}
	result := base
	for i, arg := range args {
		if arg == nil {
			continue
		}
		overlay, ok := arg.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("'merge' filter argument %d must be a map or null, got %T", i+1, arg)
		}
		result = model.MergeMaps(result, overlay)
	}
	return model.DeepCopy(result), nil
}
