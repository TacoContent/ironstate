package packages

// MergeVars deep-merges overlay into base key-by-key: a nested mapping on
// both sides merges recursively; anything else, overlay wins — ports
// Packages.psm1's Merge-VarsData. This lets a host/user overlay add or
// override individual vars without wiping out the base set.
func MergeVars(base, overlay map[string]any) map[string]any {
	result := make(map[string]any, len(base))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range overlay {
		if baseVal, ok := result[k].(map[string]any); ok {
			if overlayVal, ok := v.(map[string]any); ok {
				result[k] = MergeVars(baseVal, overlayVal)
				continue
			}
		}
		result[k] = v
	}
	return result
}

// MergeDocuments merges overlay into base — ports Packages.psm1's
// Merge-PackagesData: 'vars' deep-merges (via MergeVars); any other key
// whose value is a list on both sides is appended; everything else,
// overlay wins wholesale.
func MergeDocuments(base, overlay map[string]any) map[string]any {
	result := make(map[string]any, len(base))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range overlay {
		if k == "vars" {
			if baseVars, ok := result["vars"].(map[string]any); ok {
				if overlayVars, ok := v.(map[string]any); ok {
					result["vars"] = MergeVars(baseVars, overlayVars)
					continue
				}
			}
			result[k] = v
			continue
		}
		if baseList, ok := result[k].([]any); ok {
			if overlayList, ok := v.([]any); ok {
				merged := make([]any, 0, len(baseList)+len(overlayList))
				merged = append(merged, baseList...)
				merged = append(merged, overlayList...)
				result[k] = merged
				continue
			}
		}
		result[k] = v
	}
	return result
}
