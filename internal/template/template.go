// Package template implements '${{ ... }}' expansion — span scanning,
// whole-value vs. embedded substitution, and the soft/strict two-pass
// resolution model — a port of modules/Templates.psm1 built on top of
// internal/expr. See docs/plans/go-rewrite.md §4.4 for the three-pass
// model this package is one part of (the third, self-referential pass
// lives in the future internal/handlers/template package).
package template

import (
	"fmt"
	"os"
	"strings"

	"github.com/TacoContent/ironstate/internal/expr"
)

type omitMarker struct{}

// Omit is returned by ExpandValue/ExpandNode when a field's entire value
// was a single unresolved '${{ }}' reference — the caller must remove the
// key/element entirely rather than substitute a wrongly-typed empty
// value, mirroring Templates.psm1's TemplateOmitMarker.
var Omit = omitMarker{}

// IsOmit reports whether v is the Omit marker.
func IsOmit(v any) bool {
	_, ok := v.(omitMarker)
	return ok
}

// Warn reports an unresolved-reference warning, matching ironstate.ps1's
// Write-Warning usage. Overridable for tests/CLI output redirection.
var Warn = func(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "warning: "+format+"\n", args...)
}

// ExpandValue expands '${{ ... }}' occurrences in value. Non-string
// values pass through unchanged. label names the owning
// document/package, used only in warning text. When soft is true, an
// expression referencing a not-yet-known namespace (see NamespaceKnown)
// is left completely untouched for a later strict pass instead of being
// evaluated (and possibly warning on a still-missing reference).
func ExpandValue(value any, ctx map[string]any, filters expr.Filters, label string, soft bool) (any, error) {
	s, ok := value.(string)
	if !ok {
		return value, nil
	}
	if !strings.Contains(s, "${{") {
		return value, nil
	}

	// Whole-value case: the entire (trimmed) field content is exactly one
	// '${{ ... }}' span — replace with the result's native type.
	trimmed := strings.TrimSpace(s)
	wholeSpans := expr.ScanSpans(trimmed)
	if len(wholeSpans) == 1 && wholeSpans[0].Start == 0 && wholeSpans[0].End == len(trimmed) {
		return expandWholeValue(wholeSpans[0].Expression, ctx, filters, label, soft, value)
	}

	// Embedded case: one or more '${{ ... }}' spans inside a larger string.
	spans := expr.ScanSpans(s)
	if len(spans) == 0 {
		return value, nil
	}
	return expandEmbedded(s, spans, ctx, filters, label, soft)
}

func resolveSoft(exprText string, ctx map[string]any, filters expr.Filters, soft bool) (value any, deferred bool, err error) {
	node, err := expr.Parse(exprText)
	if err != nil {
		return nil, false, err
	}
	if soft {
		for _, p := range expr.VarPaths(node) {
			if !NamespaceKnown(ctx, p) {
				return nil, true, nil
			}
		}
	}
	v, err := expr.Eval(node, ctx, filters)
	if err != nil {
		return nil, false, err
	}
	return v, false, nil
}

func expandWholeValue(exprText string, ctx map[string]any, filters expr.Filters, label string, soft bool, original any) (any, error) {
	v, deferred, err := resolveSoft(exprText, ctx, filters, soft)
	if err != nil {
		return nil, err
	}
	if deferred {
		return original, nil
	}
	if v == nil {
		Warn("%s: unresolved template reference '%s' - field omitted", label, exprText)
		return Omit, nil
	}
	return v, nil
}

func expandEmbedded(text string, spans []expr.Span, ctx map[string]any, filters expr.Filters, label string, soft bool) (any, error) {
	var sb strings.Builder
	cursor := 0
	for _, span := range spans {
		sb.WriteString(text[cursor:span.Start])
		v, deferred, err := resolveSoft(span.Expression, ctx, filters, soft)
		if err != nil {
			return nil, err
		}
		switch {
		case deferred:
			sb.WriteString(text[span.Start:span.End])
		case v == nil:
			Warn("%s: unresolved template reference '%s'", label, span.Expression)
		default:
			sb.WriteString(expr.DisplayString(v))
		}
		cursor = span.End
	}
	sb.WriteString(text[cursor:])
	return sb.String(), nil
}

// ExpandNode recurses through map[string]any/[]any/scalar values (the
// shape produced by YAML decoding into `any`), expanding every string leaf
// in place. boundaryKeys, when non-empty, stops the walk from crossing
// into a nested map containing one of those keys — only the boundary
// key(s) themselves are expanded in that map; every sibling key is left
// completely untouched for a caller's own later, separate pass (see
// Templates.psm1's Expand-TemplateNode '-BoundaryKeys', used by loop
// materialization to stop an outer loop from resolving a nested loop's own
// fields against the wrong 'item' binding).
func ExpandNode(node any, ctx map[string]any, filters expr.Filters, label string, soft bool, boundaryKeys []string) (any, error) {
	switch v := node.(type) {
	case string:
		return ExpandValue(v, ctx, filters, label, soft)

	case map[string]any:
		isBoundary := false
		if len(boundaryKeys) > 0 {
			for _, k := range boundaryKeys {
				if _, ok := v[k]; ok {
					isBoundary = true
					break
				}
			}
		}
		for key, val := range v {
			if isBoundary && !containsStr(boundaryKeys, key) {
				continue
			}
			resolved, err := ExpandNode(val, ctx, filters, label, soft, boundaryKeys)
			if err != nil {
				return nil, err
			}
			if IsOmit(resolved) {
				delete(v, key)
			} else {
				v[key] = resolved
			}
		}
		return v, nil

	case []any:
		for i, item := range v {
			resolved, err := ExpandNode(item, ctx, filters, label, soft, boundaryKeys)
			if err != nil {
				return nil, err
			}
			// No clean way to remove a list *element* mid-walk without
			// reindexing — an omitted array entry becomes nil rather
			// than vanishing, matching Templates.psm1.
			if IsOmit(resolved) {
				v[i] = nil
			} else {
				v[i] = resolved
			}
		}
		return v, nil

	default:
		return v, nil
	}
}

func containsStr(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

// ResolveInPlace expands every top-level group/item/field of data in
// place — the document-level entry point (Templates.psm1's
// Resolve-TemplatesInPlace).
func ResolveInPlace(data map[string]any, ctx map[string]any, filters expr.Filters, label string, soft bool, boundaryKeys ...string) error {
	for key, val := range data {
		resolved, err := ExpandNode(val, ctx, filters, label, soft, boundaryKeys)
		if err != nil {
			return err
		}
		if IsOmit(resolved) {
			delete(data, key)
		} else {
			data[key] = resolved
		}
	}
	return nil
}
