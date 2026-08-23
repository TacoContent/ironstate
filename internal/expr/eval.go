package expr

import (
	"fmt"
	"strings"
)

// Filters is the pipeline's filter registry, injected rather than
// hardcoded — internal/filters implements this so internal/expr never
// depends on it (docs/plans/go-rewrite.md §4.3).
type Filters interface {
	Apply(name string, value any, args []any) (any, error)
}

// Eval resolves node to its runtime value against ctx (a flat map whose
// values may nest map[string]any / []any). Boolean-shaped nodes (And/Or/
// Not/Compare/In/Is) always return a real bool; everything else keeps its
// native type — this one function backs both 'when' truthiness (caller
// casts via Truthy) and '${{ }}' value substitution (native type kept).
func Eval(node Node, ctx map[string]any, filters Filters) (any, error) {
	switch n := node.(type) {
	case *LiteralNode:
		if s, ok := n.Value.(string); ok {
			return expandLiteralText(s, ctx, filters)
		}
		return n.Value, nil

	case *PathNode:
		v, _ := ResolvePath(ctx, n)
		return v, nil

	case *ListNode:
		items := make([]any, 0, len(n.Items))
		for _, itemNode := range n.Items {
			v, err := Eval(itemNode, ctx, filters)
			if err != nil {
				return nil, err
			}
			items = append(items, v)
		}
		return items, nil

	case *FilterNode:
		var value any
		if n.Target != nil {
			v, err := Eval(n.Target, ctx, filters)
			if err != nil {
				return nil, err
			}
			value = v
		}
		args := make([]any, 0, len(n.Args))
		for _, argNode := range n.Args {
			v, err := Eval(argNode, ctx, filters)
			if err != nil {
				return nil, err
			}
			args = append(args, v)
		}
		if filters == nil {
			return nil, fmt.Errorf("filter %q used but no filter registry configured", n.Name)
		}
		return filters.Apply(n.Name, value, args)

	case *CallNode:
		return Eval(&FilterNode{Target: nil, Name: n.Name, Args: n.Args}, ctx, filters)

	case *OrNode:
		l, err := Eval(n.Left, ctx, filters)
		if err != nil {
			return nil, err
		}
		if Truthy(l) {
			return true, nil
		}
		r, err := Eval(n.Right, ctx, filters)
		if err != nil {
			return nil, err
		}
		return Truthy(r), nil

	case *AndNode:
		l, err := Eval(n.Left, ctx, filters)
		if err != nil {
			return nil, err
		}
		if !Truthy(l) {
			return false, nil
		}
		r, err := Eval(n.Right, ctx, filters)
		if err != nil {
			return nil, err
		}
		return Truthy(r), nil

	case *NotNode:
		v, err := Eval(n.Operand, ctx, filters)
		if err != nil {
			return nil, err
		}
		return !Truthy(v), nil

	case *CompareNode:
		a, err := Eval(n.Left, ctx, filters)
		if err != nil {
			return nil, err
		}
		b, err := Eval(n.Right, ctx, filters)
		if err != nil {
			return nil, err
		}
		switch n.Op {
		case "==":
			return valuesEqual(a, b), nil
		case "!=":
			return !valuesEqual(a, b), nil
		case "<":
			return compareValues(a, b) < 0, nil
		case "<=":
			return compareValues(a, b) <= 0, nil
		case ">":
			return compareValues(a, b) > 0, nil
		case ">=":
			return compareValues(a, b) >= 0, nil
		}
		return nil, fmt.Errorf("unknown comparison operator %q", n.Op)

	case *InNode:
		needle, err := Eval(n.Left, ctx, filters)
		if err != nil {
			return nil, err
		}
		haystack, err := Eval(n.Right, ctx, filters)
		if err != nil {
			return nil, err
		}
		result := valueIn(needle, haystack)
		if n.Negate {
			return !result, nil
		}
		return result, nil

	case *IsNode:
		v, err := Eval(n.Left, ctx, filters)
		if err != nil {
			return nil, err
		}
		result, err := typeTest(v, n.Test)
		if err != nil {
			return nil, err
		}
		if n.Negate {
			return !result, nil
		}
		return result, nil
	}

	return nil, fmt.Errorf("cannot evaluate node of type %T", node)
}

// EvalBool evaluates node and casts the result with Truthy — the
// 'when'-condition entry point (Conditions.psm1's usage).
func EvalBool(node Node, ctx map[string]any, filters Filters) (bool, error) {
	v, err := Eval(node, ctx, filters)
	if err != nil {
		return false, err
	}
	return Truthy(v), nil
}

// Truthy mirrors PowerShell's implicit [bool] cast: nil/empty-string/zero/
// empty-list are false; a map is true whenever non-nil (PowerShell casts
// any non-null IDictionary — which isn't an IList — to true regardless of
// emptiness); everything else non-nil is true.
func Truthy(v any) bool {
	switch val := v.(type) {
	case nil:
		return false
	case bool:
		return val
	case string:
		return val != ""
	case float64:
		return val != 0
	case int:
		return val != 0
	case []any:
		return len(val) > 0
	case map[string]any:
		return true
	default:
		return true
	}
}

func isNumber(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	}
	return 0, false
}

func stringify(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// valuesEqual ports Test-ExpressionValuesEqual: nil-aware, bool-coerced
// (via Truthy, matching PowerShell's own [bool] cast) whenever either side
// is a bool, numeric when both look numeric, else ordinal string compare.
func valuesEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if ba, ok := a.(bool); ok {
		return ba == Truthy(b)
	}
	if bb, ok := b.(bool); ok {
		return Truthy(a) == bb
	}
	if na, ok := isNumber(a); ok {
		if nb, ok := isNumber(b); ok {
			return na == nb
		}
	}
	return strings.Compare(stringify(a), stringify(b)) == 0
}

// compareValues ports Compare-ExpressionValues: numeric compare when both
// sides look numeric, ordinal string compare otherwise.
func compareValues(a, b any) int {
	if na, ok := isNumber(a); ok {
		if nb, ok := isNumber(b); ok {
			switch {
			case na < nb:
				return -1
			case na > nb:
				return 1
			default:
				return 0
			}
		}
	}
	return strings.Compare(stringify(a), stringify(b))
}

func valueIn(needle, haystack any) bool {
	switch hs := haystack.(type) {
	case string:
		return strings.Contains(hs, stringify(needle))
	case []any:
		for _, item := range hs {
			if valuesEqual(needle, item) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func typeTest(v any, test string) (bool, error) {
	switch test {
	case "mapping", "map":
		_, ok := v.(map[string]any)
		return ok, nil
	case "boolean", "bool":
		_, ok := v.(bool)
		return ok, nil
	case "string":
		_, ok := v.(string)
		return ok, nil
	case "number":
		_, ok := isNumber(v)
		return ok, nil
	case "list":
		_, ok := v.([]any)
		return ok, nil
	case "defined":
		return v != nil, nil
	case "none", "null":
		return v == nil, nil
	}
	return false, fmt.Errorf("unknown expression test 'is %s'", test)
}

// ResolvePath walks p's segments through nested map[string]any/[]any,
// mirroring Resolve-TemplateContext. Returns (nil, false) if any segment
// is missing, out of range, or the wrong shape.
func ResolvePath(ctx map[string]any, p *PathNode) (any, bool) {
	var current any = ctx
	for _, seg := range p.Segments {
		if current == nil {
			return nil, false
		}
		if seg.IsIndex {
			list, ok := current.([]any)
			if !ok || seg.Index < 0 || seg.Index >= len(list) {
				return nil, false
			}
			current = list[seg.Index]
			continue
		}
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		v, present := m[seg.Key]
		if !present {
			return nil, false
		}
		current = v
	}
	return current, true
}

// VarPaths returns every PathNode referenced anywhere in node, in the same
// traversal order as Add-ExpressionVarPaths — used by internal/template's
// soft-resolution pass to decide whether a compound expression is fully
// resolvable yet. Note a LiteralNode's own nested '${{ }}' spans are
// deliberately NOT walked, matching the PowerShell implementation exactly.
func VarPaths(node Node) []*PathNode {
	var paths []*PathNode
	var walk func(Node)
	walk = func(n Node) {
		if n == nil {
			return
		}
		switch v := n.(type) {
		case *PathNode:
			paths = append(paths, v)
		case *ListNode:
			for _, item := range v.Items {
				walk(item)
			}
		case *FilterNode:
			walk(v.Target)
			for _, a := range v.Args {
				walk(a)
			}
		case *CallNode:
			for _, a := range v.Args {
				walk(a)
			}
		case *OrNode:
			walk(v.Left)
			walk(v.Right)
		case *AndNode:
			walk(v.Left)
			walk(v.Right)
		case *NotNode:
			walk(v.Operand)
		case *CompareNode:
			walk(v.Left)
			walk(v.Right)
		case *InNode:
			walk(v.Left)
			walk(v.Right)
		case *IsNode:
			walk(v.Left)
		}
	}
	walk(node)
	return paths
}

func expandLiteralText(text string, ctx map[string]any, filters Filters) (string, error) {
	if !strings.Contains(text, "${{") {
		return text, nil
	}
	spans := ScanSpans(text)
	if len(spans) == 0 {
		return text, nil
	}

	var sb strings.Builder
	cursor := 0
	for _, span := range spans {
		sb.WriteString(text[cursor:span.Start])
		innerNode, err := Parse(span.Expression)
		if err != nil {
			return "", err
		}
		value, err := Eval(innerNode, ctx, filters)
		if err != nil {
			return "", err
		}
		sb.WriteString(DisplayString(value))
		cursor = span.End
	}
	sb.WriteString(text[cursor:])
	return sb.String(), nil
}

// DisplayString ports ConvertTo-TemplateDisplayString: used when an
// expression's value is interpolated into a larger string rather than
// being the field's whole value.
func DisplayString(v any) string {
	if v == nil {
		return ""
	}
	if list, ok := v.([]any); ok {
		parts := make([]string, len(list))
		for i, item := range list {
			parts[i] = stringify(item)
		}
		return strings.Join(parts, ", ")
	}
	return stringify(v)
}
