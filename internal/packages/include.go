package packages

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/TacoContent/ironstate/internal/expr"
	"github.com/TacoContent/ironstate/internal/model"
	"github.com/TacoContent/ironstate/internal/template"
)

// Included is the result of resolving one 'include:' spec — ports
// Packages.psm1's Import-IncludedPackage return shape.
type Included struct {
	Data    any
	Inputs  map[string]any
	Package map[string]any
}

// Warn reports a non-fatal include problem (missing 'name', package not
// found) — matches ironstate.ps1's Write-Warning+skip behavior rather
// than erroring the whole run. Overridable for tests/CLI redirection.
var Warn = func(format string, args ...any) {
	fmt.Printf("warning: "+format+"\n", args...)
}

var reservedNamespaces = map[string]bool{"package": true, "inputs": true, "facts": true, "vars": true}

// LoadIncludedPackage loads '<packagesRoot>/<name>/main.yml' for the
// 'include' module — ports Import-IncludedPackage. 'name' is just a
// root-relative path fragment (not specific to a 'packages/' or 'roles/'
// folder — see docs/plans/go-rewrite.md §2). Returns (nil, nil) for a
// missing 'name' or a package that doesn't exist (warned, not errored,
// matching the original); a real load/parse failure once the file is
// known to exist is a real error.
func LoadIncludedPackage(includeSpec map[string]any, packagesRoot string, facts, vars map[string]any, filters expr.Filters) (*Included, error) {
	name, _ := model.Prop(includeSpec, "name")
	nameStr, _ := name.(string)
	if nameStr == "" {
		Warn("include has no 'name'; skipping")
		return nil, nil
	}

	pkgDir := filepath.Join(packagesRoot, nameStr)
	pkgFile := filepath.Join(pkgDir, "main.yml")
	if _, err := os.Stat(pkgFile); err != nil {
		Warn("included package '%s' not found: %s", nameStr, pkgFile)
		return nil, nil
	}

	pkgData, err := LoadFile(pkgFile, pkgDir)
	if err != nil {
		return nil, err
	}

	pkg := map[string]any{
		"name":  nameStr,
		"state": model.PropOr(includeSpec, "state", "present"),
		"tags":  toAnySlice(model.AsStringSlice(model.PropOr(includeSpec, "tags", nil))),
	}
	inputs := model.AsMap(model.PropOr(includeSpec, "with", map[string]any{}))

	ctx := map[string]any{
		"package": pkg,
		"inputs":  inputs,
		"facts":   facts,
		"vars":    vars,
	}
	for key, val := range model.Vars(pkgData) {
		if reservedNamespaces[key] {
			Warn("package '%s': its own 'vars.%s' collides with a reserved namespace name; ignoring", nameStr, key)
			continue
		}
		ctx[key] = val
	}

	// -Soft: a bare id/fact reference doesn't belong to any of
	// {package, inputs, facts, vars, <package's own vars keys>} and can't
	// be resolved yet — left untouched for the per-leaf dispatch-time pass
	// once that registry actually exists (internal/engine, Phase 3).
	resolved, err := resolveTemplatesInPlace(pkgData, ctx, filters, nameStr)
	if err != nil {
		return nil, err
	}

	return &Included{Data: resolved, Inputs: inputs, Package: pkg}, nil
}

// resolveTemplatesInPlace handles both a package's mapping-form root
// (Resolve-TemplatesInPlace's usual case) and a bare-list root — the
// PowerShell original only reliably supports the mapping form here (a
// bare list has no '.Keys' to iterate); this is a strictly more permissive
// superset, not a behavior change for any mapping-form package.
func resolveTemplatesInPlace(doc any, ctx map[string]any, filters expr.Filters, label string) (any, error) {
	switch v := doc.(type) {
	case map[string]any:
		if err := template.ResolveInPlace(v, ctx, filters, label, true); err != nil {
			return nil, err
		}
		return v, nil
	case []any:
		return template.ExpandNode(v, ctx, filters, label, true, nil)
	default:
		return doc, nil
	}
}

func toAnySlice(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}
