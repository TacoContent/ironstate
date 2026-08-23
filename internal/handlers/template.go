package handlers

import (
	"errors"
	"fmt"
	"os"

	"github.com/TacoContent/ironstate/internal/engine"
	"github.com/TacoContent/ironstate/internal/model"
	"github.com/TacoContent/ironstate/internal/template"
	"github.com/TacoContent/ironstate/internal/templateengines"
)

// templateHandler ports Handlers/Template.psm1: renders 'src' through the
// given 'engine' and writes the result to 'dest'. Single file only -
// unlike 'copy', there's no directory-tree templating.
//
// Deviation from Handlers/Template.psm1: 'eps'/'herestring' engines are
// dropped per docs/plans/go-rewrite.md §1/§11 (audited as unused in this
// repo) - only 'jinja' (native port) and the additive 'gotemplate' (Go
// stdlib text/template) are supported; either other name is a clear error.
type templateHandler struct{}

var errTemplateSourceMissing = errors.New("template source missing")

// getTemplateRenderContext ports Get-TemplateRenderContext: layers this
// task's own 'vars' (if any) on top of the leaf's merged flat context,
// deep-cloned so mutating it below (the self-referential resolve pass)
// never leaks into other leaves sharing the same site-wide vars, then
// runs the third, self-referential '${{ }}' pass described in
// docs/plans/go-rewrite.md §4.4 (a 'vars:' value can reference a sibling
// var directly, a reference neither the whole-document soft pass nor the
// per-leaf strict pass ever revisits).
func getTemplateRenderContext(item map[string]any, ctx engine.Context) (map[string]any, error) {
	merged := model.DeepCopy(model.AsMap(ctx.Flat)).(map[string]any)
	for k, v := range getMap(item, "vars") {
		merged[k] = model.DeepCopy(v)
	}

	wrapper := map[string]any{"ctx": merged}
	if err := template.ResolveInPlace(wrapper, merged, ctx.Filters, "template", false); err != nil {
		return nil, err
	}
	if resolved, ok := wrapper["ctx"].(map[string]any); ok {
		return resolved, nil
	}
	return merged, nil
}

func getTemplateRenderedContent(item map[string]any, ctx engine.Context) (string, error) {
	src := getString(item, "src")
	if src == "" || !fileExists(src) {
		engine.Warn("Source path for template does not exist: %s", src)
		return "", errTemplateSourceMissing
	}
	raw, err := os.ReadFile(src) //nolint:gosec // src is authored YAML content, same trust boundary as the rest of this tool
	if err != nil {
		return "", err
	}

	renderCtx, err := getTemplateRenderContext(item, ctx)
	if err != nil {
		return "", err
	}

	switch engineName := getString(item, "engine"); engineName {
	case "jinja":
		return templateengines.RenderJinja(string(raw), renderCtx, ctx.Filters)
	case "gotemplate":
		return templateengines.RenderGoTemplate(string(raw), renderCtx)
	case "eps", "herestring":
		return "", fmt.Errorf("template engine '%s' is not supported - migrate to 'jinja' or 'gotemplate' (docs/plans/go-rewrite.md §1/§11)", engineName)
	default:
		return "", fmt.Errorf("unknown template engine '%s'", engineName)
	}
}

func (templateHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	dest := resolvePath(getString(item, "dest"))
	src := getString(item, "src")
	if !fileExists(src) {
		engine.Warn("Source path for template does not exist: %s", src)
		return false, nil
	}
	info, err := os.Stat(dest)
	if err != nil || info.IsDir() {
		return false, nil
	}

	rendered, err := getTemplateRenderedContent(item, ctx)
	if err != nil {
		if errors.Is(err, errTemplateSourceMissing) {
			return false, nil
		}
		engine.Warn("Template render failed for '%s': %v", src, err)
		return false, nil
	}

	data, err := os.ReadFile(dest) //nolint:gosec // dest is authored YAML content, same trust boundary as the rest of this tool
	if err != nil {
		return false, nil
	}
	return string(data) == rendered, nil
}

func (templateHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	dest := resolvePath(getString(item, "dest"))
	src := getString(item, "src")
	if action == engine.ActionUninstall {
		return "remove " + dest, nil
	}
	return fmt.Sprintf("render %s -> %s (engine: %s)", src, dest, getString(item, "engine")), nil
}

func (templateHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	dest := resolvePath(getString(item, "dest"))
	rendered, err := getTemplateRenderedContent(item, ctx)
	if err != nil {
		if errors.Is(err, errTemplateSourceMissing) {
			return engine.ExecResult{}, nil
		}
		return engine.ExecResult{}, err
	}
	if err := ensureParentDir(dest); err != nil {
		return engine.ExecResult{}, err
	}
	return engine.ExecResult{}, os.WriteFile(dest, []byte(rendered), 0o644) //nolint:gosec // matches ironstate.ps1's own file permissions, no tighter mode intended
}

func (templateHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	dest := resolvePath(getString(item, "dest"))
	if fileExists(dest) {
		return engine.ExecResult{}, os.Remove(dest)
	}
	return engine.ExecResult{}, nil
}
