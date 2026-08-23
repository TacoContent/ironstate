package model

import (
	"reflect"
	"testing"
)

func TestUnmarshalMappingForm(t *testing.T) {
	doc, err := Unmarshal([]byte(`
vars:
  editor: nvim
tasks:
  - name: a
    log: { message: hi }
`))
	if err != nil {
		t.Fatal(err)
	}
	m, ok := doc.(map[string]any)
	if !ok {
		t.Fatalf("doc type = %T", doc)
	}
	if Vars(m)["editor"] != "nvim" {
		t.Fatalf("vars.editor = %#v", Vars(m)["editor"])
	}
	tasks, err := TaskList(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("tasks = %#v", tasks)
	}
}

func TestUnmarshalBareListForm(t *testing.T) {
	doc, err := Unmarshal([]byte(`
- name: a
  log: { message: hi }
- name: b
  log: { message: bye }
`))
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := TaskList(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("tasks = %#v", tasks)
	}
	if Vars(doc) == nil || len(Vars(doc)) != 0 {
		t.Fatalf("Vars(bare list) = %#v, want empty map", Vars(doc))
	}
}

func TestUnmarshalEmptyDocument(t *testing.T) {
	doc, err := Unmarshal([]byte("# just a comment\n"))
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := TaskList(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("tasks = %#v, want empty", tasks)
	}
}

func TestTaskListRejectsScalarRoot(t *testing.T) {
	doc, err := Unmarshal([]byte(`"just a string"`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := TaskList(doc); err == nil {
		t.Fatal("expected an error for a scalar document root")
	}
}

func TestDeepCopyIndependence(t *testing.T) {
	original := map[string]any{
		"list": []any{"a", map[string]any{"k": "v"}},
	}
	copied := DeepCopy(original).(map[string]any)
	copiedList := copied["list"].([]any)
	copiedMap := copiedList[1].(map[string]any)
	copiedMap["k"] = "mutated"

	originalMap := original["list"].([]any)[1].(map[string]any)
	if originalMap["k"] != "v" {
		t.Fatalf("original was mutated through the copy: %#v", originalMap)
	}
}

func TestPropAndPropOr(t *testing.T) {
	item := map[string]any{"package": "age", "state": "present"}
	if v, ok := Prop(item, "package"); !ok || v != "age" {
		t.Fatalf("Prop(package) = %v, %v", v, ok)
	}
	if _, ok := Prop(item, "missing"); ok {
		t.Fatal("Prop(missing) should not be found")
	}
	if v := PropOr(item, "missing", "fallback"); v != "fallback" {
		t.Fatalf("PropOr(missing) = %v", v)
	}
	if v := PropOr("not-a-map", "x", "fallback"); v != "fallback" {
		t.Fatalf("PropOr on a non-map = %v", v)
	}
}

func TestAsListWrapsScalar(t *testing.T) {
	if got := AsList(nil); got != nil {
		t.Fatalf("AsList(nil) = %#v", got)
	}
	if got := AsList("solo"); !reflect.DeepEqual(got, []any{"solo"}) {
		t.Fatalf("AsList(scalar) = %#v", got)
	}
	if got := AsList([]any{1, 2}); !reflect.DeepEqual(got, []any{1, 2}) {
		t.Fatalf("AsList(list) = %#v", got)
	}
}

func TestAsStringSlice(t *testing.T) {
	got := AsStringSlice([]any{"cli", "security", 42})
	want := []string{"cli", "security"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AsStringSlice = %#v, want %#v (non-strings dropped)", got, want)
	}
}
