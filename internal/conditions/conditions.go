// Package conditions implements 'when'/'that' condition evaluation — a
// thin consumer of internal/expr's shared tokenizer/parser/evaluator, a
// port of modules/Conditions.psm1. See that module's docstring (mirrored
// in docs/plans/go-rewrite.md §4.3/§10) for why 'when' is deliberately not
// '${{ }}'-wrapped (bare identifiers, e.g. 'computer_name == "KRAYT"').
package conditions

import (
	"fmt"
	"strings"

	"github.com/TacoContent/ironstate/internal/expr"
)

// TestCondition parses and evaluates a single condition expression against
// ctx, casting the result with expr.Truthy — ports Test-Condition.
func TestCondition(expression string, ctx map[string]any, filters expr.Filters) (bool, error) {
	node, err := expr.Parse(expression)
	if err != nil {
		return false, fmt.Errorf("condition %q: %w", expression, err)
	}
	return expr.EvalBool(node, ctx, filters)
}

// TestWhen ports Test-WhenClause: 'when'/'failed_when'/'that' all accept a
// single condition string or a list of strings (list = implicit AND,
// matching Ansible) - a missing/empty/nil entry set always passes. Each
// entry is 'any' rather than 'string' because a leaf's raw When/FailedWhen
// carries unevaluated YAML content, which may include a literal YAML
// boolean (e.g. 'when: true') rather than a string.
//
// A bool entry is stringified as Go's own lowercase "true"/"false" before
// being parsed - NOT PowerShell's "True"/"False". PowerShell's tokenizer
// happens to accept "True" too (case-insensitive keyword matching), but
// internal/expr's tokenizer is deliberately case-sensitive (see
// docs/plans/go-rewrite-progress.md's "Phase 3's 'when'-boolean gotcha"),
// so the fix belongs here, at the boundary, not in the tokenizer.
func TestWhen(when []any, ctx map[string]any, filters expr.Filters) (bool, error) {
	for _, raw := range when {
		exprText, ok := stringifyWhenEntry(raw)
		if !ok {
			continue
		}
		result, err := TestCondition(exprText, ctx, filters)
		if err != nil {
			return false, err
		}
		if !result {
			return false, nil
		}
	}
	return true, nil
}

func stringifyWhenEntry(raw any) (string, bool) {
	switch v := raw.(type) {
	case nil:
		return "", false
	case string:
		if strings.TrimSpace(v) == "" {
			return "", false
		}
		return v, true
	case bool:
		if v {
			return "true", true
		}
		return "false", true
	default:
		return fmt.Sprintf("%v", v), true
	}
}
