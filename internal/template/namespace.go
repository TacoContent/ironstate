package template

import "github.com/TacoContent/ironstate/internal/expr"

// NamespaceKnown reports whether path's top-level segment belongs to a
// namespace this soft pass actually knows about (e.g. 'facts'/'vars'/
// 'package'/'inputs' at call sites that provide them) — as opposed to a
// bare id/fact name that may only exist in a *later* pass's context.
//
// 'facts' is a special case, ported exactly from
// Test-ExpressionNamespaceKnown: gathered host facts are fully known up
// front, but user-defined 'fact' values (sharing the same 'facts.<name>'
// surface) are populated progressively as tasks run — the namespace key
// existing doesn't mean *this specific* fact does yet, unlike 'vars'/
// 'package'/'inputs', which are always complete by the time any pass sees
// them. So for 'facts.*' specifically, this defers unless the full path
// already resolves to a value, rather than deferring only on an
// unrecognized top segment.
func NamespaceKnown(ctx map[string]any, path *expr.PathNode) bool {
	if len(path.Segments) == 0 || path.Segments[0].IsIndex {
		return false
	}
	top := path.Segments[0].Key
	if _, present := ctx[top]; !present {
		return false
	}
	if top == "facts" {
		_, ok := expr.ResolvePath(ctx, path)
		return ok
	}
	return true
}
