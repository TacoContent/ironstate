package packages

import "github.com/TacoContent/ironstate/internal/model"

// MergeVars deep-merges overlay into base key-by-key: a nested mapping on
// both sides merges recursively; anything else, overlay wins — ports
// Packages.psm1's Merge-VarsData. This lets a host/user overlay add or
// override individual vars without wiping out the base set.
func MergeVars(base, overlay map[string]any) map[string]any {
	return model.MergeMaps(base, overlay)
}

// MergeDocuments merges overlay into base — ports Packages.psm1's
// Merge-PackagesData: 'vars' deep-merges and 'envs' merges by variable name;
// environment source objects are replaced atomically. Any other
// key whose value is a list on both sides is appended; everything else,
// overlay wins wholesale. The overlay's file-level 'tags' apply only to
// the overlay's own tasks (see scopeFileTags) - base's 'tags' are left
// as the merged document's file-level tags, covering every task in it.
func MergeDocuments(base, overlay map[string]any) map[string]any {
	overlay = scopeFileTags(overlay)
	result := make(map[string]any, len(base))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range overlay {
		if k == "envs" {
			merged := make(map[string]any)
			for name, value := range model.AsMap(result[k]) {
				merged[name] = value
			}
			for name, value := range model.AsMap(v) {
				merged[name] = value
			}
			result[k] = merged
			continue
		}
		if k == "vars" {
			if baseVars, ok := result[k].(map[string]any); ok {
				if overlayVars, ok := v.(map[string]any); ok {
					result[k] = MergeVars(baseVars, overlayVars)
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

// scopeFileTags returns doc without its file-level 'tags', its tasks
// wrapped in one grouping task carrying those tags so they cascade to
// that file's tasks only.
func scopeFileTags(doc map[string]any) map[string]any {
	tags, ok := doc["tags"]
	if !ok {
		return doc
	}
	result := make(map[string]any, len(doc))
	for k, v := range doc {
		if k != "tags" {
			result[k] = v
		}
	}
	if tags == nil {
		return result
	}
	if tasks := model.AsList(doc["tasks"]); len(tasks) > 0 {
		result["tasks"] = []any{map[string]any{"tags": tags, "actions": tasks}}
	}
	return result
}
