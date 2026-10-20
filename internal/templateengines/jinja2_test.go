package templateengines

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/internal/filters"
)

func writeJinja2Source(t *testing.T, dir, name, content string) string {
	t.Helper()
	source := filepath.Join(dir, name)
	if err := os.WriteFile(source, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return source
}

func TestRenderJinja2Editorconfig(t *testing.T) {
	source := writeJinja2Source(t, t.TempDir(), "editorconfig.j2", `root = true

{% for pattern, rules in editorconfig.rules.items() %}
[{{ pattern }}]
{%- for key, value in rules.items() %}
{{ key }} = {{ value }}
{%- endfor %}
{% endfor %}`)
	ctx := map[string]any{
		"editorconfig": map[string]any{
			"rules": map[string]any{
				"*":     map[string]any{"indent_size": 2, "indent_style": "space"},
				"*.txt": map[string]any{"charset": "unset"},
			},
		},
	}
	got, err := RenderJinja2(source, ctx, filters.New())
	if err != nil {
		t.Fatal(err)
	}
	want := "root = true\n\n\n[*]\nindent_size = 2\nindent_style = space\n\n[*.txt]\ncharset = unset\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderJinja2LanguageFeatures(t *testing.T) {
	for _, tc := range []struct {
		name, content, want string
	}{
		{"macro", `{% macro greet(name) %}Hello {{ name | upper }}!{% endmacro %}{{ greet("world") }}`, "Hello WORLD!"},
		{"loop metadata and else", `{% for x in [1, 2] %}{{ loop.index }}:{{ x }};{% endfor %}{% for x in [] %}bad{% else %}empty{% endfor %}`, "1:1;2:2;empty"},
		{"whitespace", "before \n{%- if true -%}\n after {%- endif %}\n", "beforeafter\n"},
		{"default and kwargs", `{{ missing | default("fallback") }}|{{ "" | default("blank", boolean=true) }}`, "fallback|blank"},
		{"configuration text", "{{ text }}\n", "<>&\"'\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := writeJinja2Source(t, t.TempDir(), "template.j2", tc.content)
			got, err := RenderJinja2(source, map[string]any{"text": "<>&\"'"}, filters.New())
			if err != nil || got != tc.want {
				t.Fatalf("got %q, err %v, want %q", got, err, tc.want)
			}
		})
	}
}

func TestRenderJinja2IncludesAndInheritance(t *testing.T) {
	dir := t.TempDir()
	writeJinja2Source(t, dir, "base.j2", "before:{% block body %}base{% endblock %}:after\n")
	writeJinja2Source(t, dir, "partial.j2", "{{ name }}")
	source := writeJinja2Source(t, dir, "child.j2", `{% extends "base.j2" %}{% block body %}{% include "partial.j2" %}{% endblock %}`)
	got, err := RenderJinja2(source, map[string]any{"name": "value"}, filters.New())
	if err != nil || got != "before:value:after\n" {
		t.Fatalf("got %q, err %v", got, err)
	}
}

func TestRenderJinja2IronstateFilters(t *testing.T) {
	source := writeJinja2Source(t, t.TempDir(), "filters.j2", `{{ "value" | custom("!") }}|{{ "" | default("blank", boolean=true) }}`)
	for _, suffix := range []string{"first", "second"} {
		t.Run(suffix, func(t *testing.T) {
			t.Parallel()
			registry := filters.New()
			registry.Register("custom", func(value any, args []any) (any, error) {
				if value != "value" || len(args) != 1 || args[0] != "!" {
					return nil, errors.New("incorrect custom filter arguments")
				}
				return suffix, nil
			})
			got, err := RenderJinja2(source, nil, registry)
			if err != nil || got != suffix+"|blank" {
				t.Fatalf("got %q, err %v", got, err)
			}
		})
	}
}

func TestRenderJinja2Errors(t *testing.T) {
	for _, tc := range []struct {
		name, content, message string
	}{
		{"syntax", "{% for a, b in mapping.items() %}", "parse"},
		{"unknown filter", "{{ value | unknown_filter }}", "unknown_filter"},
		{"filter failure", "{{ value | broken }}", "filter failed"},
		{"filter kwargs", "{{ value | broken(argument=1) }}", "does not accept keyword arguments"},
		{"missing include", `{% include "missing.j2" %}`, "missing.j2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := writeJinja2Source(t, t.TempDir(), "bad.j2", tc.content)
			registry := filters.New()
			registry.Register("broken", func(any, []any) (any, error) {
				return nil, errors.New("filter failed")
			})
			got, err := RenderJinja2(source, nil, registry)
			if err == nil || !strings.Contains(err.Error(), tc.message) || !strings.Contains(err.Error(), strconv.Quote(source)) || got != "" {
				t.Fatalf("got %q, err %v, want source path and %q", got, err, tc.message)
			}
		})
	}
}
