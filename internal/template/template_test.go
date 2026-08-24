package template

import (
	"testing"

	"github.com/TacoContent/ironstate/internal/expr"
	"github.com/TacoContent/ironstate/internal/filters"
)

var fset = filters.New()

func TestWholeValuePreservesNativeType(t *testing.T) {
	ctx := map[string]any{"vars": map[string]any{"list": []any{"a", "b"}}}
	got, err := ExpandValue("${{ vars.list }}", ctx, fset, "test", false)
	if err != nil {
		t.Fatal(err)
	}
	list, ok := got.([]any)
	if !ok || len(list) != 2 {
		t.Fatalf("whole-value list = %#v, want native []any", got)
	}
}

func TestEmbeddedSubstitutionStaysAString(t *testing.T) {
	ctx := map[string]any{"vars": map[string]any{"name": "world"}}
	got, err := ExpandValue("hello ${{ vars.name }}!", ctx, fset, "test", false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello world!" {
		t.Fatalf("embedded = %v", got)
	}
}

func TestWholeValueUnresolvedOmitsField(t *testing.T) {
	got, err := ExpandValue("${{ vars.missing }}", map[string]any{"vars": map[string]any{}}, fset, "test", false)
	if err != nil {
		t.Fatal(err)
	}
	if !IsOmit(got) {
		t.Fatalf("expected Omit marker, got %#v", got)
	}
}

func TestSoftPassDefersUnknownNamespace(t *testing.T) {
	// 'foo' isn't a known namespace in this (soft) context yet — the field
	// must come back completely untouched for a later strict pass.
	original := "${{ foo.bar }}"
	got, err := ExpandValue(original, map[string]any{"vars": map[string]any{}}, fset, "test", true)
	if err != nil {
		t.Fatal(err)
	}
	if got != original {
		t.Fatalf("soft defer = %#v, want original string unchanged", got)
	}
}

func TestSoftPassFactsRequiresFullPathResolution(t *testing.T) {
	ctx := map[string]any{"facts": map[string]any{"computer_name": "KRAYT"}}

	// A gathered fact resolves even under -Soft (namespace key present AND
	// the full path already resolves).
	got, err := ExpandValue("${{ facts.computer_name }}", ctx, fset, "test", true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "KRAYT" {
		t.Fatalf("known fact under soft pass = %#v", got)
	}

	// A user-defined fact that hasn't run yet must defer, even though
	// 'facts' itself is present in ctx — this is the special case that
	// differs from every other namespace (docs/plans/go-rewrite.md §4.3).
	original := "${{ facts.not_yet_gathered }}"
	got, err = ExpandValue(original, ctx, fset, "test", true)
	if err != nil {
		t.Fatal(err)
	}
	if got != original {
		t.Fatalf("undefined fact under soft pass = %#v, want deferred unchanged", got)
	}
}

func TestSoftPassVarsAndPackageNamespacesResolveOnTopSegmentAlone(t *testing.T) {
	ctx := map[string]any{"vars": map[string]any{}, "package": map[string]any{"name": "git"}}
	// 'vars' namespace exists (even though 'missing' key inside it doesn't)
	// -> not deferred, resolves straight to nil -> Omit.
	got, err := ExpandValue("${{ vars.missing }}", ctx, fset, "test", true)
	if err != nil {
		t.Fatal(err)
	}
	if !IsOmit(got) {
		t.Fatalf("vars.missing under soft pass = %#v, want Omit (not deferred)", got)
	}

	got, err = ExpandValue("${{ package.name }}", ctx, fset, "test", true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "git" {
		t.Fatalf("package.name = %#v", got)
	}
}

// erroringFilters lets a test force a specific filter call to fail,
// regardless of the piped value/args.
type erroringFilters struct{ name string }

func (f erroringFilters) Apply(name string, value any, args []any) (any, error) {
	if name == f.name {
		return nil, errBoom
	}
	return fset.Apply(name, value, args)
}

var errBoom = &boomErr{}

type boomErr struct{}

func (*boomErr) Error() string { return "boom filter always fails" }

// TestSoftPassDefersOnFilterError guards a real bug: an expression with
// no var-path to defer on (e.g. a literal piped through a filter) used
// to propagate a filter's error straight out of a soft pass, aborting
// the whole document-wide walk even for a leaf 'when' would go on to
// skip once tasks are flattened. A soft pass must instead defer it,
// unchanged, for the later per-leaf strict pass (soft=false), which DOES
// propagate the error - exercised separately below.
func TestSoftPassDefersOnFilterError(t *testing.T) {
	original := "${{ 'x' | boom }}"
	got, err := ExpandValue(original, map[string]any{}, erroringFilters{name: "boom"}, "test", true)
	if err != nil {
		t.Fatalf("soft pass must defer a filter error, not propagate it: %v", err)
	}
	if got != original {
		t.Fatalf("soft defer = %#v, want original string unchanged", got)
	}
}

func TestStrictPassPropagatesFilterError(t *testing.T) {
	_, err := ExpandValue("${{ 'x' | boom }}", map[string]any{}, erroringFilters{name: "boom"}, "test", false)
	if err == nil {
		t.Fatal("expected the strict (soft=false) pass to propagate the filter error")
	}
}

func TestExpandNodeRecursesAndOmitsKeys(t *testing.T) {
	data := map[string]any{
		"keep": "static",
		"gone": "${{ vars.missing }}",
		"nested": map[string]any{
			"value": "${{ vars.x }}",
		},
	}
	ctx := map[string]any{"vars": map[string]any{"x": "resolved"}}
	resolved, err := ExpandNode(data, ctx, fset, "test", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := resolved.(map[string]any)
	if _, present := m["gone"]; present {
		t.Fatalf("expected 'gone' key to be removed entirely, got %#v", m)
	}
	if m["keep"] != "static" {
		t.Fatalf("keep = %#v", m["keep"])
	}
	nested := m["nested"].(map[string]any)
	if nested["value"] != "resolved" {
		t.Fatalf("nested.value = %#v", nested["value"])
	}
}

func TestBoundaryKeysStopNestedLoopFieldsFromResolvingEarly(t *testing.T) {
	// Simulates Tasks.psm1's per-iteration expansion: only 'items' (the
	// loop selector) should resolve in the enclosing pass; 'log.message'
	// belongs to the nested loop's own later pass and must stay untouched.
	data := map[string]any{
		"items": "${{ vars.outer }}",
		"log":   map[string]any{"message": "${{ item }}"},
	}
	ctx := map[string]any{"vars": map[string]any{"outer": []any{"a", "b"}}}
	resolved, err := ExpandNode(data, ctx, fset, "test", false, []string{"items", "with"})
	if err != nil {
		t.Fatal(err)
	}
	m := resolved.(map[string]any)
	items, ok := m["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("items = %#v, want resolved list", m["items"])
	}
	logMsg := m["log"].(map[string]any)["message"]
	if logMsg != "${{ item }}" {
		t.Fatalf("nested log.message = %#v, want left untouched", logMsg)
	}
}

func TestOmittedListElementBecomesNilNotRemoved(t *testing.T) {
	data := []any{"a", "${{ vars.missing }}", "c"}
	ctx := map[string]any{"vars": map[string]any{}}
	resolved, err := ExpandNode(data, ctx, fset, "test", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := resolved.([]any)
	if len(list) != 3 || list[1] != nil {
		t.Fatalf("list = %#v, want middle element nil (not removed)", list)
	}
}

func TestNamespaceKnownDirectly(t *testing.T) {
	ctx := map[string]any{"vars": map[string]any{}, "facts": map[string]any{"a": 1}}
	pathIn := func(expression string) *expr.PathNode {
		node, err := expr.Parse(expression)
		if err != nil {
			t.Fatal(err)
		}
		return node.(*expr.PathNode)
	}
	if !NamespaceKnown(ctx, pathIn("vars.anything")) {
		t.Error("vars.* should be known once 'vars' namespace exists")
	}
	if !NamespaceKnown(ctx, pathIn("facts.a")) {
		t.Error("facts.a should be known once it resolves")
	}
	if NamespaceKnown(ctx, pathIn("facts.b")) {
		t.Error("facts.b should NOT be known when it hasn't resolved yet")
	}
	if NamespaceKnown(ctx, pathIn("some_id.rc")) {
		t.Error("an unrecognized top-level namespace should not be known")
	}
}
