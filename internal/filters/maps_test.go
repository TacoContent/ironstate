package filters

import (
	"reflect"
	"testing"
)

func TestMapFilter(t *testing.T) {
	want := map[string]any{"foo": "bar", "nested": map[string]any{"enabled": true}}
	for _, value := range []any{
		want,
		`{"foo":"bar","nested":{"enabled":true}}`,
		[]any{[]any{"foo", "bar"}, []any{"nested", map[string]any{"enabled": true}}},
	} {
		got := apply(t, "map", value)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("map(%#v) = %#v, want %#v", value, got, want)
		}
		got.(map[string]any)["nested"].(map[string]any)["enabled"] = false
		if want["nested"].(map[string]any)["enabled"] != true {
			t.Fatal("map filter mutated its input")
		}
	}
	for _, value := range []any{nil, []any{}, map[string]any{}} {
		if got := apply(t, "map", value); !reflect.DeepEqual(got, map[string]any{}) {
			t.Fatalf("map(%#v) = %#v, want an empty map", value, got)
		}
	}
	if got := apply(t, "map", []any{[]any{"key", "first"}, []any{"key", "last"}}); got.(map[string]any)["key"] != "last" {
		t.Fatalf("duplicate pair key = %#v", got)
	}
}

func TestMapFilterRejectsInvalidInput(t *testing.T) {
	for _, value := range []any{
		true, 1, "{broken", "[]", "null", `"scalar"`,
		[]any{"key"}, []any{[]any{"key"}}, []any{[]any{"key", 1, 2}}, []any{[]any{1, "value"}},
	} {
		if _, err := New().Apply("map", value, nil); err == nil {
			t.Errorf("map(%#v) should fail", value)
		}
	}
	if _, err := New().Apply("map", nil, []any{"extra"}); err == nil {
		t.Fatal("map accepted extra arguments")
	}
}

func TestMergeFilterLists(t *testing.T) {
	base := []any{map[string]any{"name": "camalot"}}
	overlay := []any{map[string]any{"name": "extras"}}
	want := []any{map[string]any{"name": "camalot"}, map[string]any{"name": "extras"}}
	got := apply(t, "merge", base, nil, overlay).([]any)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merge lists = %#v, want %#v", got, want)
	}
	got[0].(map[string]any)["name"] = "changed"
	got[1].(map[string]any)["name"] = "changed"
	if base[0].(map[string]any)["name"] != "camalot" || overlay[0].(map[string]any)["name"] != "extras" {
		t.Fatal("merge result shares mutable values with an input")
	}
	if got := apply(t, "merge", base, nil); !reflect.DeepEqual(got, base) {
		t.Fatalf("list | merge(null) = %#v, want %#v", got, base)
	}
	if got := apply(t, "merge", nil, nil, base); !reflect.DeepEqual(got, base) {
		t.Fatalf("null | merge(null, list) = %#v, want %#v", got, base)
	}
	for _, inputs := range [][]any{
		{base, map[string]any{}}, {map[string]any{}, base}, {base, true},
	} {
		if _, err := New().Apply("merge", inputs[0], inputs[1:]); err == nil {
			t.Errorf("merge(%#v) should reject mixed types", inputs)
		}
	}
}

func TestMergeFilter(t *testing.T) {
	base := map[string]any{
		"rules": map[string]any{"*": map[string]any{"indent_size": 2, "charset": "utf-8"}},
		"list":  []any{"original"},
		"keep":  true,
	}
	overlay := map[string]any{
		"rules": map[string]any{"*": map[string]any{"indent_size": 4}, "*.txt": map[string]any{"charset": "unset"}},
		"list":  []any{"replacement"},
		"keep":  nil,
	}
	originalBase, originalOverlay := apply(t, "map", base), apply(t, "map", overlay)
	got := apply(t, "merge", base, overlay, map[string]any{"foo": "bar"}).(map[string]any)
	want := map[string]any{
		"rules": map[string]any{
			"*":     map[string]any{"indent_size": 4, "charset": "utf-8"},
			"*.txt": map[string]any{"charset": "unset"},
		},
		"list": []any{"replacement"}, "keep": nil, "foo": "bar",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merge = %#v, want %#v", got, want)
	}
	got["rules"].(map[string]any)["*"].(map[string]any)["charset"] = "changed"
	got["rules"].(map[string]any)["*.txt"].(map[string]any)["charset"] = "changed"
	got["list"].([]any)[0] = "changed"
	if !reflect.DeepEqual(base, originalBase) || !reflect.DeepEqual(overlay, originalOverlay) {
		t.Fatal("merge result shares mutable values with an input")
	}
	for _, value := range []any{nil, map[string]any{}} {
		if got := apply(t, "merge", value, nil); !reflect.DeepEqual(got, map[string]any{}) {
			t.Fatalf("merge(%#v, nil) = %#v", value, got)
		}
	}
	if got := apply(t, "merge", nil, base); !reflect.DeepEqual(got, base) {
		t.Fatalf("null | merge(base) = %#v", got)
	}
	for _, tc := range []struct {
		value any
		args  []any
	}{
		{nil, nil}, {"not a map", []any{base}}, {base, []any{true}}, {base, []any{`{"foo":"bar"}`}},
	} {
		if _, err := New().Apply("merge", tc.value, tc.args); err == nil {
			t.Errorf("merge(%#v, %#v) should fail", tc.value, tc.args)
		}
	}
}
