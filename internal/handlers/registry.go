package handlers

import (
	"fmt"
	"sort"

	"github.com/TacoContent/ironstate/internal/engine"
)

// Registry holds the handlers and recognized module names for one run.
type Registry struct {
	handlers map[string]engine.Handler
}

// NewRegistry returns a registry initialized with every builtin handler.
func NewRegistry() *Registry {
	return &Registry{handlers: All()}
}

// Merge adds externally supplied, fully-qualified handlers. It rejects a
// collision so a plugin can never replace a builtin or another plugin.
func (r *Registry) Merge(external map[string]engine.Handler) error {
	if r == nil {
		return fmt.Errorf("handler registry is nil")
	}
	for name, handler := range external {
		if name == "" || handler == nil {
			return fmt.Errorf("plugin handler name and implementation are required")
		}
		if _, exists := r.handlers[name]; exists {
			return fmt.Errorf("handler %q is already registered", name)
		}
	}
	for name, handler := range external {
		r.handlers[name] = handler
	}
	return nil
}

// Handlers returns a defensive copy suitable for engine.Options.
func (r *Registry) Handlers() map[string]engine.Handler {
	result := make(map[string]engine.Handler, len(r.handlers))
	for name, handler := range r.handlers {
		result[name] = handler
	}
	return result
}

// ModuleNames returns sorted names for tasks.Options.ModuleNames.
func (r *Registry) ModuleNames() []string {
	names := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
