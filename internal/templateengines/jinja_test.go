package templateengines

import (
	"testing"

	"github.com/TacoContent/ironstate/internal/filters"
)

func TestRenderJinjaExprOutput(t *testing.T) {
	got, err := RenderJinja("Hello {{ name }}!", map[string]any{"name": "World"}, filters.New())
	if err != nil {
		t.Fatal(err)
	}
	if got != "Hello World!" {
		t.Fatalf("got %q", got)
	}
}

func TestRenderJinjaNestedFor(t *testing.T) {
	content := `{% for ent in enterprise %}{% for org in ent.orgs %}[{{ ent.host }}/{{ org }}]
{% endfor %}{% endfor %}`
	ctx := map[string]any{
		"enterprise": []any{
			map[string]any{"host": "github.example.com", "orgs": []any{"a", "b"}},
		},
	}
	got, err := RenderJinja(content, ctx, filters.New())
	if err != nil {
		t.Fatal(err)
	}
	want := "[github.example.com/a]\n[github.example.com/b]\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderJinjaIfElifElse(t *testing.T) {
	render := func(x float64) string {
		got, err := RenderJinja("{% if x > 1 %}big{% elif x == 1 %}one{% else %}small{% endif %}", map[string]any{"x": x}, filters.New())
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if render(2) != "big" || render(1) != "one" || render(0) != "small" {
		t.Fatalf("if/elif/else branch selection incorrect: %q %q %q", render(2), render(1), render(0))
	}
}

func TestRenderJinjaSetIsScoped(t *testing.T) {
	// A 'set' inside a 'for' body is isolated to that iteration.
	content := `{% for i in items %}{% set doubled = i %}{{ doubled }}{% endfor %}|{{ doubled | default('unset') }}`
	got, err := RenderJinja(content, map[string]any{"items": []any{float64(1), float64(2)}}, filters.New())
	if err != nil {
		t.Fatal(err)
	}
	if got != "12|unset" {
		t.Fatalf("got %q", got)
	}
}

func TestRenderJinjaFilterPipeline(t *testing.T) {
	got, err := RenderJinja("{{ name | upper }}", map[string]any{"name": "abc"}, filters.New())
	if err != nil {
		t.Fatal(err)
	}
	if got != "ABC" {
		t.Fatalf("got %q", got)
	}
}

func TestRenderJinjaUnterminatedTagLeavesRestUntouched(t *testing.T) {
	got, err := RenderJinja("before {{ unterminated", map[string]any{}, filters.New())
	if err != nil {
		t.Fatal(err)
	}
	if got != "before {{ unterminated" {
		t.Fatalf("got %q", got)
	}
}

func TestRenderGoTemplateBasic(t *testing.T) {
	got, err := RenderGoTemplate("Hello {{ .Name }}!", map[string]any{"Name": "World"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Hello World!" {
		t.Fatalf("got %q", got)
	}
}
