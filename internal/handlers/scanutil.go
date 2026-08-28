package handlers

import (
	"encoding/json"
	"fmt"
)

// parseJSONList parses PowerShell's ConvertTo-Json output, which prints a
// bare object (not a one-element array) when exactly one record matches -
// shared by every Windows-side Scan() that shells out to a
// Select-Object|ConvertTo-Json pipeline (user, group).
func parseJSONList[T any](out string) ([]T, error) {
	var items []T
	if err := json.Unmarshal([]byte(out), &items); err == nil {
		return items, nil
	}
	var item T
	if err := json.Unmarshal([]byte(out), &item); err == nil {
		return []T{item}, nil
	}
	return nil, fmt.Errorf("unable to parse JSON list")
}
