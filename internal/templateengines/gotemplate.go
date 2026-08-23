package templateengines

// gotemplate.go: an additive, non-compatibility-required render engine
// using Go's stdlib text/template directly - offered because it's
// effectively free once the engine is Go (docs/plans/go-rewrite.md §4.7).
// gotemplate's own '{{ }}' delimiter is unrelated to ironstate's own
// '${{ }}' expression syntax - the render *context* is still built the
// same way (facts/vars/registry); only the template body's own syntax
// differs per engine.

import (
	"strings"
	"text/template"
)

// RenderGoTemplate renders content through Go's stdlib text/template
// against ctx.
func RenderGoTemplate(content string, ctx map[string]any) (string, error) {
	tmpl, err := template.New("template").Option("missingkey=zero").Parse(content)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, ctx); err != nil {
		return "", err
	}
	return sb.String(), nil
}
