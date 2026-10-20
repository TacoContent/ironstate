package template

import (
	"reflect"
	"testing"
)

func TestMapFilterPipelinePreservesNativeType(t *testing.T) {
	for _, soft := range []bool{false, true} {
		ctx := map[string]any{
			"defaults": map[string]any{"nested": map[string]any{"keep": true, "override": "default"}},
			"custom":   map[string]any{"nested": map[string]any{"override": "user"}},
			"bar":      "value",
		}
		text := "${{ defaults | merge(custom) | merge({foo: bar})}}"
		got, err := ExpandValue(text, ctx, fset, "maps", soft)
		want := map[string]any{"nested": map[string]any{"keep": true, "override": "user"}, "foo": "value"}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("soft=%v: got %#v, err=%v, want %#v", soft, got, err, want)
		}
	}
	for _, text := range []string{
		"${{ map() }}", "${{ missing | map }}", "${{ '{}' | map }}", "${{ [] | map }}", "${{{}}}",
	} {
		got, err := ExpandValue(text, nil, fset, "maps", false)
		if err != nil || !reflect.DeepEqual(got, map[string]any{}) {
			t.Fatalf("%s = %#v, err=%v, want empty map", text, got, err)
		}
	}
	text := "${{ map() | merge({key: future.value})}}"
	got, err := ExpandValue(text, nil, fset, "maps", true)
	if err != nil || got != text {
		t.Fatalf("soft pass failed to defer map value: %#v, %v", got, err)
	}
}
