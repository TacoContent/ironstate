package expr

import (
	"reflect"
	"testing"
)

func TestMapLiterals(t *testing.T) {
	ctx := map[string]any{"bar": "value", "number": float64(4)}
	for _, tc := range []struct {
		expression string
		want       map[string]any
	}{
		{"{}", map[string]any{}},
		{"{foo: bar}", map[string]any{"foo": "value"}},
		{`{"quoted key": [bar, {nested: number}],}`, map[string]any{"quoted key": []any{"value", map[string]any{"nested": float64(4)}}}},
		{"{foo: 1, foo: 2}", map[string]any{"foo": float64(2)}},
		{`{foo: missing | default("fallback")}`, map[string]any{"foo": "fallback"}},
	} {
		if got := evalOK(t, tc.expression, ctx); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Eval(%s) = %#v, want %#v", tc.expression, got, tc.want)
		}
	}
	for _, expression := range []string{"{", "{foo}", "{foo:}", "{foo: 1", "{1: 2}", "{foo: 1 bar: 2}", "{foo: 1,,}", "{foo: |}"} {
		if _, err := Parse(expression); err == nil {
			t.Errorf("Parse(%s) should fail", expression)
		}
	}
	if _, err := Eval(mustParse(t, "{foo: value | unknown}"), ctx, fakeFilters{}); err == nil {
		t.Fatal("map literal swallowed a filter error")
	}
	paths := VarPaths(mustParse(t, "{literalKey: bar, nested: {other: number}}"))
	if len(paths) != 2 || paths[0].Segments[0].Key != "bar" || paths[1].Segments[0].Key != "number" {
		t.Fatalf("map VarPaths = %#v", paths)
	}
}

func TestScanSpansWithMapLiterals(t *testing.T) {
	for _, text := range []string{
		"${{{foo: {bar: 1}}}}",
		"${{ {foo: {bar: 1}} }}",
		`prefix ${{ {foo: "}}", bar: {nested: 1}} }} suffix`,
	} {
		spans := ScanSpans(text)
		if len(spans) != 1 {
			t.Fatalf("ScanSpans(%q) = %#v", text, spans)
		}
		if _, err := Parse(spans[0].Expression); err != nil {
			t.Fatalf("ScanSpans(%q) truncated a map: %v", text, err)
		}
	}
}
