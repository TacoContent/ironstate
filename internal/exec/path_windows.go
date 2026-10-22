//go:build windows

package exec

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// RefreshPath picks up installer-added persistent PATH entries without
// replacing session-specific entries or changing their precedence.
func RefreshPath() (bool, error) {
	return refreshPath(readPersistentPaths)
}

func readPersistentPaths() ([]string, error) {
	var paths []string
	for _, scope := range []struct {
		key  registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`},
		{registry.CURRENT_USER, `Environment`},
	} {
		key, err := registry.OpenKey(scope.key, scope.path, registry.QUERY_VALUE)
		if errors.Is(err, registry.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", scope.path, err)
		}
		value, kind, err := key.GetStringValue("Path")
		_ = key.Close()
		if errors.Is(err, registry.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s\\Path: %w", scope.path, err)
		}
		if kind == registry.EXPAND_SZ {
			value, err = registry.ExpandString(value)
			if err != nil {
				return nil, fmt.Errorf("expand %s\\Path: %w", scope.path, err)
			}
		}
		paths = append(paths, value)
	}
	return paths, nil
}

func refreshPath(readPaths func() ([]string, error)) (bool, error) {
	paths, err := readPaths()
	if err != nil {
		return false, err
	}
	current := os.Getenv("PATH")
	updated := current
	seen := map[string]bool{}
	normalize := func(entry string) string {
		return strings.ToLower(strings.TrimRight(strings.Trim(strings.TrimSpace(entry), `"`), `\/`))
	}
	for _, entry := range strings.Split(current, ";") {
		seen[normalize(entry)] = true
	}
	for _, path := range paths {
		for _, entry := range strings.Split(path, ";") {
			key := normalize(entry)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			if updated != "" && !strings.HasSuffix(updated, ";") {
				updated += ";"
			}
			updated += entry
		}
	}
	if updated == current {
		return false, nil
	}
	if err := os.Setenv("PATH", updated); err != nil {
		return false, err
	}
	return true, nil
}
