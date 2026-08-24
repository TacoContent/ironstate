// Package tasks implements task/action tree normalization and flattening
// — a port of modules/Tasks.psm1's Expand-TaskTree. See that module's
// docstring (mirrored in docs/plans/go-rewrite.md §4.9) for the full
// algorithm description: tags/when accumulate top-down; 'with'/'items'
// materializes a task once per loop value *before* anything else about it
// is evaluated; 'include' loads another document's tasks via
// internal/packages, isolated from the parent's PackageVars/loop context.
package tasks

import (
	"fmt"
	"os"
	"strings"

	"github.com/TacoContent/ironstate/internal/expr"
	"github.com/TacoContent/ironstate/internal/model"
	"github.com/TacoContent/ironstate/internal/packages"
	"github.com/TacoContent/ironstate/internal/template"
	"github.com/TacoContent/ironstate/internal/ui"
)

// Leaf is one flattened, dispatch-ready action:
// { Name, Tags, When, ID, ... , Module, Item }, mirroring
// Expand-TaskTree's per-leaf PSCustomObject shape. 'When'/'FailedWhen'
// stay unevaluated (each entry is a condition string, or a bool when the
// YAML literally wrote 'when: true') — evaluation happens per-leaf, later,
// once an id/fact registry exists (internal/engine, Phase 3).
type Leaf struct {
	Name            string
	Tags            []string
	When            []any
	ID              string
	SecretID        bool
	FailedWhen      []any
	ContinueOnError bool
	Looped          bool
	PackageVars     map[string]any
	PackageInputs   map[string]any
	PackagePackage  map[string]any
	Module          string
	Item            map[string]any
}

// Options carries the inputs Expand-TaskTree needs beyond the task list
// itself. ModuleNames is an ORDERED list of recognized leaf module keys
// (e.g. "winget", "copy", ...) — order matters only for the (user-error)
// case of a leaf carrying more than one recognized module key, where the
// first one in this list wins; the original PowerShell instead used
// whatever key order ConvertFrom-Yaml -Ordered preserved from the YAML
// source, which this port does not reproduce (map key order is not
// preserved through Go's map[string]any — see docs/plans/go-rewrite.md
// §4.2). This only affects a document that is already invalid by the
// schema (two module keys on one leaf), so the different tie-break is a
// documented, low-risk gap rather than a silent behavior change for any
// valid document.
type Options struct {
	ModuleNames  []string
	PackagesRoot string
	Facts        map[string]any
	Vars         map[string]any
	Filters      expr.Filters
}

// Warn reports a non-fatal tree-shape problem (unrecognized module key,
// 'id' on a grouping task, missing PackagesRoot for an 'include', ...).
// Overridable for tests/CLI redirection. Styled the same as
// internal/engine's Warn (yellow, stderr) for a consistent look across
// every warning in a run.
var Warn = func(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(os.Stderr, ui.Yellow("⚠ "+msg))
}

type scope struct {
	packageVars    map[string]any
	packageInputs  map[string]any
	packagePackage map[string]any
	parentTags     []string
	parentWhen     []any
	parentLooped   bool
	parentItemCtx  map[string]any // nil outside any loop
}

// Expand flattens tasksList into leaves, matching Expand-TaskTree's
// top-level entry point (parent tags/when empty, not inside any loop).
func Expand(tasksList []any, opts Options) ([]Leaf, error) {
	return expand(tasksList, opts, scope{
		packageVars:    map[string]any{},
		packageInputs:  map[string]any{},
		packagePackage: map[string]any{},
	})
}

func expand(tasksList []any, opts Options, sc scope) ([]Leaf, error) {
	var results []Leaf

	for _, raw := range tasksList {
		if raw == nil {
			continue
		}
		item, ok := raw.(map[string]any)
		if !ok {
			Warn("task item is not a mapping (%T); skipping", raw)
			continue
		}

		_, hasWith := item["with"]
		_, hasItems := item["items"]
		if hasWith || hasItems {
			loopKey := "with"
			if hasItems {
				loopKey = "items"
			}
			children, err := expandLoop(item, loopKey, opts, sc)
			if err != nil {
				return nil, err
			}
			results = append(results, children...)
			continue
		}

		label := leafLabel(item)
		effectiveTags := uniqueStrings(append(append([]string{}, sc.parentTags...), model.AsStringSlice(item["tags"])...))
		effectiveWhen := append(append([]any{}, sc.parentWhen...), model.AsList(item["when"])...)

		if actions, ok := item["actions"]; ok {
			if _, hasID := item["id"]; hasID {
				Warn("task '%s' has an 'id' but is a grouping task (has 'actions'); 'id' is only supported on leaf actions - ignoring", label)
			}
			children, err := expand(model.AsList(actions), opts, scope{
				packageVars:    sc.packageVars,
				packageInputs:  sc.packageInputs,
				packagePackage: sc.packagePackage,
				parentTags:     effectiveTags,
				parentWhen:     effectiveWhen,
				parentLooped:   sc.parentLooped,
				parentItemCtx:  sc.parentItemCtx,
			})
			if err != nil {
				return nil, err
			}
			results = append(results, children...)
			continue
		}

		if includeSpec, ok := item["include"]; ok {
			if _, hasID := item["id"]; hasID {
				Warn("task '%s' has an 'id' but is an 'include'; 'id' is only supported on leaf actions - ignoring", label)
			}
			if opts.PackagesRoot == "" {
				Warn("task '%s' has an 'include' but no PackagesRoot was configured; skipping", label)
				continue
			}
			included, err := packages.LoadIncludedPackage(model.AsMap(includeSpec), opts.PackagesRoot, opts.Facts, opts.Vars, opts.Filters)
			if err != nil {
				return nil, err
			}
			if included == nil {
				continue
			}
			includedTasks, err := model.TaskList(included.Data)
			if err != nil {
				return nil, err
			}
			children, err := expand(includedTasks, opts, scope{
				packageVars:    model.Vars(included.Data),
				packageInputs:  included.Inputs,
				packagePackage: included.Package,
				parentTags:     effectiveTags,
				parentWhen:     effectiveWhen,
				parentLooped:   sc.parentLooped,
				parentItemCtx:  nil,
			})
			if err != nil {
				return nil, err
			}
			results = append(results, children...)
			continue
		}

		moduleName := firstModuleKey(item, opts.ModuleNames)
		if moduleName == "" {
			Warn("task '%s' has no recognized module key; skipping", label)
			continue
		}

		name, _ := item["name"].(string)
		id, _ := item["id"].(string)
		secretID := strings.HasPrefix(id, "$")
		if secretID {
			id = strings.TrimPrefix(id, "$")
		}
		continueOnError, _ := item["continue_on_error"].(bool)

		results = append(results, Leaf{
			Name:            name,
			Tags:            effectiveTags,
			When:            effectiveWhen,
			ID:              id,
			SecretID:        secretID,
			FailedWhen:      model.AsList(item["failed_when"]),
			ContinueOnError: continueOnError,
			Looped:          sc.parentLooped,
			PackageVars:     sc.packageVars,
			PackageInputs:   sc.packageInputs,
			PackagePackage:  sc.packagePackage,
			Module:          moduleName,
			Item:            model.AsMap(item[moduleName]),
		})
	}

	return results, nil
}

func expandLoop(item map[string]any, key string, opts Options, sc scope) ([]Leaf, error) {
	label := leafLabel(item)
	if _, hasWith := item["with"]; hasWith && key == "items" {
		Warn("task '%s' has both 'with' and 'items'; using 'items' and ignoring 'with'", label)
	}

	var loopValues []any
	if key == "items" {
		loopValues = model.AsList(item["items"])
	} else {
		loopValues = []any{item["with"]}
	}

	tmpl := model.DeepCopy(item).(map[string]any)
	delete(tmpl, "with")
	delete(tmpl, "items")

	var results []Leaf
	for _, loopValue := range loopValues {
		materialized := model.DeepCopy(tmpl)
		wrapper := map[string]any{"task": materialized}
		itemCtx := map[string]any{"item": loopValue}
		if sc.parentItemCtx != nil {
			itemCtx["parent"] = sc.parentItemCtx
		}
		if err := template.ResolveInPlace(wrapper, itemCtx, opts.Filters, label, true, "items", "with"); err != nil {
			return nil, err
		}

		children, err := expand([]any{wrapper["task"]}, opts, scope{
			packageVars:    sc.packageVars,
			packageInputs:  sc.packageInputs,
			packagePackage: sc.packagePackage,
			parentTags:     sc.parentTags,
			parentWhen:     sc.parentWhen,
			parentLooped:   true,
			parentItemCtx:  itemCtx,
		})
		if err != nil {
			return nil, err
		}
		results = append(results, children...)
	}
	return results, nil
}

func leafLabel(item map[string]any) string {
	if name, ok := item["name"].(string); ok && name != "" {
		return name
	}
	if pkg, ok := item["package"].(string); ok && pkg != "" {
		return pkg
	}
	return "<unnamed>"
}

func firstModuleKey(item map[string]any, moduleNames []string) string {
	var found []string
	for _, name := range moduleNames {
		if _, ok := item[name]; ok {
			found = append(found, name)
		}
	}
	if len(found) == 0 {
		return ""
	}
	if len(found) > 1 {
		label := leafLabel(item)
		Warn("task '%s' has multiple module keys (%v); using '%s' and ignoring the rest", label, found, found[0])
	}
	return found[0]
}

func uniqueStrings(list []string) []string {
	seen := make(map[string]bool, len(list))
	out := make([]string, 0, len(list))
	for _, s := range list {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
