// Package filters implements the built-in '|' pipeline filters (a direct
// port of modules/Filters/*.ps1) plus the external script-filter adapter
// described in docs/plans/go-rewrite.md §4.5 (see script.go/pool.go). It
// implements internal/expr.Filters so internal/expr never imports it.
package filters

import "fmt"

// Func is one filter's implementation: value is the piped-in value (nil
// for a bare 'name(args)' call), args are the evaluated call arguments.
type Func func(value any, args []any) (any, error)

// Registry holds a set of named filters and implements expr.Filters.
type Registry struct {
	fns map[string]Func
}

// New returns a Registry with every built-in filter registered.
func New() *Registry {
	r := &Registry{fns: make(map[string]Func)}
	registerBuiltins(r)
	return r
}

// Register adds or replaces the filter named name.
func (r *Registry) Register(name string, fn Func) {
	r.fns[name] = fn
}

// Apply implements internal/expr.Filters.
func (r *Registry) Apply(name string, value any, args []any) (any, error) {
	fn, ok := r.fns[name]
	if !ok {
		return nil, fmt.Errorf("unknown filter %q", name)
	}
	return fn(value, args)
}

// Names returns every registered filter name, unsorted.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.fns))
	for name := range r.fns {
		names = append(names, name)
	}
	return names
}

// Has reports whether name is already registered - used by script-filter
// discovery to let an existing (built-in or previously registered) filter
// of the same name win, matching modules/Filters/*.ps1's "a Go built-in
// always wins" discovery rule.
func (r *Registry) Has(name string) bool {
	_, ok := r.fns[name]
	return ok
}

// defaultRegistry backs the package-level convenience functions used by
// internal/cli (e.g. `ironstate filters list`).
var defaultRegistry = New()

// Names returns every built-in filter name known to the default registry.
func Names() []string { return defaultRegistry.Names() }

// Apply evaluates a filter by name against the default registry.
func Apply(name string, value any, args []any) (any, error) {
	return defaultRegistry.Apply(name, value, args)
}
