package templateengines

import (
	"fmt"
	"path/filepath"

	"github.com/nikolalohinski/gonja/v2/builtins"
	"github.com/nikolalohinski/gonja/v2/config"
	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/loaders"

	"github.com/TacoContent/ironstate/internal/expr"
)

// RenderJinja2 renders source with Gonja's Jinja2-compatible syntax. Includes
// and imports resolve relative to the source file, not the process directory.
func RenderJinja2(source string, ctx map[string]any, filters expr.Filters) (string, error) {
	loader, err := loaders.NewFileSystemLoader(filepath.Dir(source))
	if err != nil {
		return "", fmt.Errorf("jinja2 template %q: %w", source, err)
	}
	filterSet := exec.NewFilterSet(map[string]exec.FilterFunction{}).Update(builtins.Filters)
	if filters != nil {
		named, ok := filters.(interface{ Names() []string })
		if !ok {
			return "", fmt.Errorf("jinja2 template %q: filter registry must expose Names", source)
		}
		for _, name := range named.Names() {
			// Jinja2 built-ins retain their standard semantics on name collisions.
			if filterSet.Exists(name) {
				continue
			}
			if err := filterSet.Register(name, func(_ *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
				if len(params.KwArgs) != 0 {
					return exec.AsValue(fmt.Errorf("ironstate filter %q does not accept keyword arguments", name))
				}
				args := make([]any, len(params.Args))
				for i, arg := range params.Args {
					args[i] = arg.Interface()
				}
				value, err := filters.Apply(name, in.Interface(), args)
				if err != nil {
					return exec.AsValue(err)
				}
				return exec.AsValue(value)
			}); err != nil {
				return "", fmt.Errorf("jinja2 filter %q: %w", name, err)
			}
		}
	}
	env := &exec.Environment{
		Context:           exec.EmptyContext().Update(builtins.GlobalFunctions).Update(builtins.GlobalVariables),
		Filters:           filterSet,
		Tests:             builtins.Tests,
		ControlStructures: builtins.ControlStructures,
		Methods:           builtins.Methods,
	}
	cfg := config.New()
	cfg.KeepTrailingNewline = true
	tmpl, err := exec.NewTemplate(filepath.Base(source), cfg, loader, env)
	if err != nil {
		return "", fmt.Errorf("jinja2 template %q: %w", source, err)
	}
	rendered, err := tmpl.ExecuteToString(exec.NewContext(ctx))
	if err != nil {
		return "", fmt.Errorf("jinja2 template %q: %w", source, err)
	}
	return rendered, nil
}
