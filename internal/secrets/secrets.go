package secrets

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

const minSecretLength = 6

var (
	mu     sync.RWMutex
	values []string
)

var secretVarPaths = map[string]bool{}

func Reset() {
	mu.Lock()
	defer mu.Unlock()
	values = nil
	secretVarPaths = map[string]bool{}
}

func MarkSecretVarPaths(value any) {
	var walk func(prefix string, current any)
	walk = func(prefix string, current any) {
		switch v := current.(type) {
		case map[string]any:
			for key, child := range v {
				path := key
				if prefix != "" {
					path = prefix + "." + key
				}
				if strings.HasPrefix(key, "$") {
					bare := strings.TrimPrefix(key, "$")
					if prefix == "" {
						RegisterVarPath(bare)
						RegisterVarPath("vars." + bare)
					} else {
						RegisterVarPath(prefix + "." + bare)
						RegisterVarPath("vars." + prefix + "." + bare)
					}
					delete(v, key)
					v[bare] = child
				}
				if prefix != "" || !strings.HasPrefix(key, "$") {
					walk(path, child)
				}
			}
		case []any:
			for i, child := range v {
				walk(fmt.Sprintf("%s[%d]", prefix, i), child)
			}
		}
	}
	walk("", value)
}

func RegisterVarPath(path string) {
	if path == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	secretVarPaths[path] = true
}

func IsSecretVarPath(path string) bool {
	if path == "" {
		return false
	}
	path = strings.TrimPrefix(path, ".")
	mu.RLock()
	defer mu.RUnlock()
	if _, ok := secretVarPaths[path]; ok {
		return true
	}
	if strings.HasPrefix(path, "vars.") {
		_, ok := secretVarPaths[strings.TrimPrefix(path, "vars.")]
		return ok
	}
	return false
}

// Register records a sensitive value for later redaction. Values shorter than
// minSecretLength are ignored to avoid masking common terms such as "true" or
// "admin".
func Register(value string) {
	if value == "" || len(value) < minSecretLength {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	for _, existing := range values {
		if existing == value {
			return
		}
	}
	values = append(values, value)
}

// Redact replaces every registered secret value with *** in the provided text.
func Redact(text string) string {
	if text == "" {
		return text
	}
	mu.RLock()
	registered := append([]string(nil), values...)
	mu.RUnlock()
	if len(registered) == 0 {
		return text
	}
	sort.Slice(registered, func(i, j int) bool {
		return len(registered[i]) > len(registered[j])
	})
	for _, value := range registered {
		if value == "" {
			continue
		}
		text = strings.ReplaceAll(text, value, "***")
	}
	return text
}
