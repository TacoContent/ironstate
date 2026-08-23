package expr

import (
	"reflect"
	"testing"
)

// fakeFilters is a minimal Filters implementation for tests.
type fakeFilters struct{}

func (fakeFilters) Apply(name string, value any, args []any) (any, error) {
	switch name {
	case "default":
		if value == nil {
			return args[0], nil
		}
		return value, nil
	case "upper":
		s, _ := value.(string)
		out := ""
		for _, r := range s {
			if r >= 'a' && r <= 'z' {
				r -= 32
			}
			out += string(r)
		}
		return out, nil
	case "trim":
		s, _ := value.(string)
		return trimForTest(s), nil
	case "echo":
		if len(args) > 0 {
			return args[0], nil
		}
		return nil, nil
	}
	return nil, errUnknownTestFilter(name)
}

func trimForTest(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

type errUnknownTestFilter string

func (e errUnknownTestFilter) Error() string { return "unknown test filter: " + string(e) }

func mustParse(t *testing.T, expression string) Node {
	t.Helper()
	node, err := Parse(expression)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", expression, err)
	}
	return node
}

func evalOK(t *testing.T, expression string, ctx map[string]any) any {
	t.Helper()
	node := mustParse(t, expression)
	v, err := Eval(node, ctx, fakeFilters{})
	if err != nil {
		t.Fatalf("Eval(%q) error: %v", expression, err)
	}
	return v
}

func evalBoolOK(t *testing.T, expression string, ctx map[string]any) bool {
	t.Helper()
	node := mustParse(t, expression)
	v, err := EvalBool(node, ctx, fakeFilters{})
	if err != nil {
		t.Fatalf("EvalBool(%q) error: %v", expression, err)
	}
	return v
}

func TestLiteralsAndArithmeticComparisons(t *testing.T) {
	cases := []struct {
		expr string
		want any
	}{
		{`1 == 1`, true},
		{`1 == 2`, false},
		{`1 < 2`, true},
		{`2 <= 2`, true},
		{`3 > 2`, true},
		{`2 >= 3`, false},
		{`"abc" == "abc"`, true},
		{`"abc" == "ABC"`, false}, // case-sensitive, unlike PowerShell's native -eq
		{`"a" < "b"`, true},
		{`true == true`, true},
		{`null == null`, true},
		{`1 != 2`, true},
	}
	for _, c := range cases {
		got := evalOK(t, c.expr, nil)
		if got != c.want {
			t.Errorf("%s = %v, want %v", c.expr, got, c.want)
		}
	}
}

func TestBooleanEqualityCoercesOtherSideTruthy(t *testing.T) {
	// "some_map == true is also true for any non-empty mapping" (README).
	ctx := map[string]any{"m": map[string]any{"k": "v"}}
	got := evalOK(t, "m == true", ctx)
	if got != true {
		t.Fatalf("m == true = %v, want true", got)
	}
}

func TestAndOrNotShortCircuitAndTruthy(t *testing.T) {
	ctx := map[string]any{
		"empty_str": "",
		"str":       "x",
		"zero":      float64(0),
		"list":      []any{},
		"nonempty":  []any{1},
	}
	cases := []struct {
		expr string
		want bool
	}{
		{"true and false", false},
		{"true or false", true},
		{"not false", true},
		{"empty_str or str", true},
		{"zero and str", false},
	}
	for _, c := range cases {
		got := evalOK(t, c.expr, ctx)
		if got != c.want {
			t.Errorf("%s = %v, want %v", c.expr, got, c.want)
		}
	}

	// A bare variable with no operator is only truthy-checked by EvalBool
	// (the 'when'-condition entry point) — Eval itself returns the raw
	// value for a plain PathNode, matching Get-ExpressionValue's 'Var' case
	// (Conditions.psm1 applies the final [bool] cast, not Get-ExpressionValue).
	if got := evalBoolOK(t, "list", ctx); got != false {
		t.Errorf("bare empty list truthy = %v, want false", got)
	}
	if got := evalBoolOK(t, "nonempty", ctx); got != true {
		t.Errorf("bare non-empty list truthy = %v, want true", got)
	}
}

func TestInAndNotIn(t *testing.T) {
	ctx := map[string]any{"tags": []any{"cli", "security"}}
	if got := evalOK(t, `"cli" in tags`, ctx); got != true {
		t.Errorf("in list: got %v", got)
	}
	if got := evalOK(t, `"missing" not in tags`, ctx); got != true {
		t.Errorf("not in list: got %v", got)
	}
	if got := evalOK(t, `"gam" in "gaming"`, nil); got != true {
		t.Errorf("substring in: got %v", got)
	}
}

func TestIsTypeTests(t *testing.T) {
	ctx := map[string]any{
		"m":   map[string]any{},
		"b":   true,
		"s":   "x",
		"n":   float64(1),
		"l":   []any{1, 2},
		"nul": nil,
	}
	cases := []struct {
		expr string
		want bool
	}{
		{"m is mapping", true},
		{"m is map", true},
		{"b is boolean", true},
		{"b is bool", true},
		{"s is string", true},
		{"n is number", true},
		{"l is list", true},
		{"m is defined", true},
		{"nul is defined", false},
		{"nul is none", true},
		{"m is not boolean", true},
	}
	for _, c := range cases {
		got := evalOK(t, c.expr, ctx)
		if got != c.want {
			t.Errorf("%s = %v, want %v", c.expr, got, c.want)
		}
	}
}

func TestIsNullTestNameIsUnreachableLikeThePowerShellOriginal(t *testing.T) {
	// 'null' is documented as an alias for the 'none' type-test (see
	// Test-ExpressionTypeTest / typeTest), but the tokenizer lexes the bare
	// word 'null' as the null-*literal* keyword, never as an identifier —
	// so '<expr> is null' can never actually parse in the original
	// PowerShell grammar either (Parse-ExpressionMembership requires an
	// 'ident' token after 'is'). Ported byte-for-byte rather than silently
	// "fixed", per docs/plans/go-rewrite.md's no-silent-deviation rule —
	// use 'is none' instead.
	_, err := Parse("x is null")
	if err == nil {
		t.Fatal("expected 'is null' to be a parse error, matching the original grammar")
	}
}

func TestDottedAndIndexedPaths(t *testing.T) {
	ctx := map[string]any{
		"a": map[string]any{
			"b": []any{
				map[string]any{"c": float64(42)},
			},
		},
	}
	got := evalOK(t, "a.b[0].c", ctx)
	if got != float64(42) {
		t.Fatalf("a.b[0].c = %v, want 42", got)
	}
}

func TestMissingPathResolvesToNil(t *testing.T) {
	got := evalOK(t, "missing.nested", map[string]any{})
	if got != nil {
		t.Fatalf("missing path = %v, want nil", got)
	}
}

func TestFilterPipelineAndChaining(t *testing.T) {
	got := evalOK(t, `missing | default("fallback")`, map[string]any{})
	if got != "fallback" {
		t.Fatalf("default filter = %v", got)
	}
	got = evalOK(t, `"  hi  " | trim | upper`, nil)
	if got != "HI" {
		t.Fatalf("chained filters = %v", got)
	}
}

func TestBareFilterCall(t *testing.T) {
	got := evalOK(t, `echo("bare")`, nil)
	if got != "bare" {
		t.Fatalf("bare call = %v", got)
	}
}

func TestListLiteral(t *testing.T) {
	got := evalOK(t, `[1, "two", true]`, nil)
	want := []any{float64(1), "two", true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("list literal = %#v, want %#v", got, want)
	}
}

func TestStringLiteralEmbedsSpans(t *testing.T) {
	ctx := map[string]any{"name": "world"}
	got := evalOK(t, `"hello ${{ name }}"`, ctx)
	if got != "hello world" {
		t.Fatalf("nested span expansion = %v", got)
	}
}

func TestVarPathsSkipsLiteralNestedSpans(t *testing.T) {
	node := mustParse(t, `a == "b ${{ c }}"`)
	paths := VarPaths(node)
	if len(paths) != 1 || paths[0].Segments[0].Key != "a" {
		t.Fatalf("VarPaths = %#v, want only the top-level 'a' path (literal spans are not walked)", paths)
	}
}

func TestScanSpansIgnoresBracesInsideQuotes(t *testing.T) {
	spans := ScanSpans(`${{ lookup("url", "https://x/}}") }}`)
	if len(spans) != 1 {
		t.Fatalf("expected exactly 1 span, got %d: %#v", len(spans), spans)
	}
	if spans[0].Expression != `lookup("url", "https://x/}}")` {
		t.Fatalf("unexpected span expression: %q", spans[0].Expression)
	}
}

func TestUnterminatedStringLiteralIsSyntaxError(t *testing.T) {
	_, err := Parse(`"unterminated`)
	if err == nil {
		t.Fatal("expected a syntax error for an unterminated string literal")
	}
}

func TestUnknownFilterErrors(t *testing.T) {
	_, err := Eval(mustParse(t, "1 | nope"), nil, fakeFilters{})
	if err == nil {
		t.Fatal("expected an error for an unknown filter")
	}
}
