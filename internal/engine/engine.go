package engine

// Package engine ports ironstate.ps1's Invoke-PackageItem/Invoke-Tasks/main
// flow (docs/plans/go-rewrite.md §4.10): sequential, in-document-order
// dispatch of flattened tasks.Leaf values against a registered Handler,
// threading a growing id-registry + user-fact set + command-availability
// cache forward across a facts-first pass and then everything else.

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/TacoContent/ironstate/internal/conditions"
	"github.com/TacoContent/ironstate/internal/expr"
	"github.com/TacoContent/ironstate/internal/model"
	"github.com/TacoContent/ironstate/internal/tasks"
	"github.com/TacoContent/ironstate/internal/template"
)

// Action is a resolved Install/Uninstall/Skip decision — ports
// Resolve-PackageAction's return value.
type Action string

const (
	ActionSkip      Action = "Skip"
	ActionInstall   Action = "Install"
	ActionUninstall Action = "Uninstall"
)

// ExecResult is a leaf's normalized execution result — ports the
// '{ rc, stdout, stdout_lines, stderr, stderr_lines }' shape every handler
// result normalizes to. Extra carries any additional native properties a
// handler wants to expose under an 'id' (e.g. a future 'shell.host: pwsh'
// native-object merge - see docs/plans/go-rewrite.md §4.8/§11; unused by
// every Phase 3 handler today but plumbed through so Phase 4's 'shell'
// doesn't need a registry-shape change).
type ExecResult struct {
	RC          int
	Stdout      string
	StdoutLines []string
	Stderr      string
	StderrLines []string
	Extra       map[string]any
}

// Context is what a Handler's Test/Describe/Install/Uninstall sees beyond
// the leaf's own Item: the flat when/template context (facts/vars/
// package/inputs/registry) built by mergeFlatContext, the same filter
// registry '${{ }}'/'when' use (needed by e.g. 'assert', whose 'that'
// conditions can themselves use '| filter(...)'), and whether this
// dispatch is a real (-Apply) or dry-run invocation.
type Context struct {
	Flat    map[string]any
	Filters expr.Filters
	Apply   bool
}

// Handler is the uniform Test/Describe/Install/Uninstall shape every
// module implements — ports modules/Handlers/*.psm1's PSCustomObject
// script-block contract (docs/plans/go-rewrite.md §4.8).
type Handler interface {
	Test(item map[string]any, name string, ctx Context) (bool, error)
	Describe(item map[string]any, action Action, ctx Context) (string, error)
	Install(item map[string]any, name string, ctx Context) (ExecResult, error)
	Uninstall(item map[string]any, name string, ctx Context) (ExecResult, error)
}

// Result is one dispatched leaf's outcome — ports Invoke-PackageItem's
// returned PSCustomObject, plus the 'Failed' field Invoke-Tasks adds.
type Result struct {
	Module  string
	Package string
	State   string
	Action  Action
	Apply   bool
	Exec    ExecResult
	Failed  bool
}

// State threads the growing id-registry, user-defined facts, and
// per-module command-availability cache across one or more RunLeaves
// calls — ports Invoke-Tasks's '$Registry'/'$UserFacts'/
// '$CommandAvailability' parameters/return fields, which the main flow
// passes from a facts-gathering pass into the following "everything else"
// pass as one continuous, threaded run.
type State struct {
	Registry            map[string]map[string]any
	UserFacts           map[string]any
	CommandAvailability map[string]bool
}

// NewState returns an empty, ready-to-use State.
func NewState() *State {
	return &State{
		Registry:            map[string]map[string]any{},
		UserFacts:           map[string]any{},
		CommandAvailability: map[string]bool{},
	}
}

// Warn reports a non-fatal dispatch-time problem (missing handler,
// missing command, a failed leaf, ...). Overridable for tests/CLI output
// redirection, matching internal/tasks and internal/template's pattern.
var Warn = func(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "warning: "+format+"\n", args...)
}

// Info reports a normal operational line (a dispatched leaf's
// description) — Write-Host's equivalent. Overridable for tests/CLI
// output redirection.
var Info = func(format string, args ...any) {
	fmt.Fprintf(os.Stdout, format+"\n", args...)
}

// LookPath resolves a module's backing CLI on PATH — overridable so tests
// never depend on the host machine's real PATH contents.
var LookPath = func(name string) (string, error) { return exec.LookPath(name) }

// DefaultNoCommandCheckModules lists every module with no external CLI to
// check for on PATH — ports ironstate.ps1's '$script:NoCommandCheckModules'.
// Every Phase 3 handler is in this set; Phase 4 adds the package-manager
// modules, which are NOT in this set (they DO need a PATH check).
var DefaultNoCommandCheckModules = map[string]bool{
	"symlinks": true, "zip": true, "copy": true, "shell": true,
	"blockinfile": true, "log": true, "path": true, "fact": true,
	"registry": true, "scheduled_task": true, "file": true, "template": true,
	"assert": true, "ssh_host_block": true,
}

// DefaultModuleCommandNames remaps a module's task-tree name to its actual
// CLI binary name where they differ — ports
// '$script:ModuleCommandNames' (today: exactly one entry).
var DefaultModuleCommandNames = map[string]string{"chocolatey": "choco"}

// Options configures RunLeaves/Run's dispatch behavior.
type Options struct {
	Handlers              map[string]Handler
	Facts                 map[string]any
	Vars                  map[string]any
	Filters               expr.Filters
	Apply                 bool
	NoCommandCheckModules map[string]bool   // nil -> DefaultNoCommandCheckModules
	ModuleCommandNames    map[string]string // nil -> DefaultModuleCommandNames
}

func (o Options) noCommandCheckModules() map[string]bool {
	if o.NoCommandCheckModules != nil {
		return o.NoCommandCheckModules
	}
	return DefaultNoCommandCheckModules
}

func (o Options) moduleCommandNames() map[string]string {
	if o.ModuleCommandNames != nil {
		return o.ModuleCommandNames
	}
	return DefaultModuleCommandNames
}

// Run dispatches leaves in two phases — every 'fact' leaf first, in
// document order, then everything else — threading one continuous State
// across both, matching ironstate.ps1's main flow. Returns every
// dispatched leaf's Result (in dispatch order across both phases) and
// whether the run stopped early on an unhandled failure.
func Run(leaves []tasks.Leaf, opts Options) ([]Result, bool, error) {
	var factLeaves, otherLeaves []tasks.Leaf
	for _, l := range leaves {
		if l.Module == "fact" {
			factLeaves = append(factLeaves, l)
		} else {
			otherLeaves = append(otherLeaves, l)
		}
	}

	state := NewState()
	results, stopped, err := RunLeaves(factLeaves, opts, state)
	if err != nil {
		return results, stopped, err
	}
	if stopped {
		return results, true, nil
	}

	otherResults, stopped2, err := RunLeaves(otherLeaves, opts, state)
	results = append(results, otherResults...)
	return results, stopped2, err
}

// RunLeaves dispatches leaves sequentially, in document order, mutating
// state in place — ports Invoke-Tasks. Each leaf's 'when' and remaining
// '${{ }}' references resolve against facts+vars+registry-so-far
// immediately before it runs, so a later leaf can see an earlier leaf's
// 'id'/'fact'.
func RunLeaves(leaves []tasks.Leaf, opts Options, state *State) ([]Result, bool, error) {
	noCommandCheck := opts.noCommandCheckModules()
	moduleCommandNames := opts.moduleCommandNames()

	var results []Result
	for _, leaf := range leaves {
		module := leaf.Module
		label := leafLabel(leaf)

		handler, ok := opts.Handlers[module]
		if !ok {
			Warn("No handler registered for module '%s'; skipping.", module)
			continue
		}

		if !noCommandCheck[module] {
			commandName := module
			if remap, ok := moduleCommandNames[module]; ok {
				commandName = remap
			}
			if _, checked := state.CommandAvailability[module]; !checked {
				_, err := LookPath(commandName)
				state.CommandAvailability[module] = err == nil
			}
			if !state.CommandAvailability[module] {
				Warn("'%s' command not found on PATH; skipping.", commandName)
				continue
			}
		}

		flatContext := mergeFlatContext(opts.Facts, state.UserFacts, leaf.PackageVars, leaf.PackageInputs, leaf.PackagePackage, opts.Vars, state.Registry)

		// A 'fact' with an embedded 'shell' computes its value from that
		// command's own not-yet-run result - defer 'value's template
		// resolution until after the command runs (see runFactLeaf).
		hasEmbeddedShell := module == "fact" && hasKey(leaf.Item, "shell")
		var deferredFactValue any
		hasDeferredFactValue := false
		if hasEmbeddedShell {
			if v, present := leaf.Item["value"]; present {
				deferredFactValue = v
				hasDeferredFactValue = true
				delete(leaf.Item, "value")
			}
		}

		wrapper := map[string]any{"item": leaf.Item}
		if err := template.ResolveInPlace(wrapper, flatContext, opts.Filters, label, false); err != nil {
			return results, false, err
		}
		leaf.Item = model.AsMap(wrapper["item"])

		whenOK, err := conditions.TestWhen(leaf.When, flatContext, opts.Filters)
		if err != nil {
			return results, false, err
		}
		if !whenOK {
			continue
		}

		// A fact's embedded shell and every 'assert' have no real system
		// side effect, so previews stay accurate even without '-Apply' -
		// see docs/plans/go-rewrite.md §2/§4.10.
		effectiveApply := opts.Apply || hasEmbeddedShell || module == "assert"

		result, err := invokePackageItem(module, leaf.Name, leaf.Item, handler, flatContext, opts.Filters, effectiveApply)
		if err != nil {
			return results, false, err
		}

		changed := result.Action != ActionSkip
		failed := result.Exec.RC != 0
		if len(leaf.FailedWhen) > 0 {
			failedWhenContext := cloneFlat(flatContext)
			mergeExecInto(failedWhenContext, result.Exec)
			failedWhenContext["changed"] = changed
			failed, err = conditions.TestWhen(leaf.FailedWhen, failedWhenContext, opts.Filters)
			if err != nil {
				return results, false, err
			}
		}
		result.Failed = failed
		results = append(results, result)

		if failed {
			if leaf.ContinueOnError {
				Warn("[%s] %s failed (rc=%d); continuing (continue_on_error).", module, label, result.Exec.RC)
			} else {
				Warn("[%s] %s failed (rc=%d); stopping. Set continue_on_error: true to continue past this failure.", module, label, result.Exec.RC)
				return results, true, nil
			}
		}

		if module == "fact" {
			applyFactResult(leaf.Item, result, state, flatContext, opts.Filters, label, hasEmbeddedShell, hasDeferredFactValue, deferredFactValue)
		}

		if leaf.ID != "" {
			registerLeafResult(state, leaf, changed, failed, result.Exec)
		}
	}

	return results, false, nil
}

func applyFactResult(item map[string]any, result Result, state *State, flatContext map[string]any, filters expr.Filters, label string, hasEmbeddedShell, hasDeferredFactValue bool, deferredFactValue any) {
	factName, _ := item["name"].(string)
	if factName == "" {
		return
	}
	switch {
	case result.Action == ActionUninstall:
		delete(state.UserFacts, factName)
	case hasEmbeddedShell:
		if hasDeferredFactValue {
			valueContext := cloneFlat(flatContext)
			mergeExecInto(valueContext, result.Exec)
			valueWrapper := map[string]any{"value": deferredFactValue}
			if err := template.ResolveInPlace(valueWrapper, valueContext, filters, label, false); err != nil {
				Warn("fact '%s': resolving deferred value failed: %v", factName, err)
				return
			}
			state.UserFacts[factName] = valueWrapper["value"]
		} else {
			state.UserFacts[factName] = strings.TrimSpace(result.Exec.Stdout)
		}
	default:
		state.UserFacts[factName] = item["value"]
	}
}

func registerLeafResult(state *State, leaf tasks.Leaf, changed, failed bool, exec ExecResult) {
	registered := map[string]any{
		"changed":      changed,
		"failed":       failed,
		"rc":           float64(exec.RC),
		"stdout":       exec.Stdout,
		"stdout_lines": toAnySlice(exec.StdoutLines),
		"stderr":       exec.Stderr,
		"stderr_lines": toAnySlice(exec.StderrLines),
	}
	for k, v := range exec.Extra {
		if _, exists := registered[k]; !exists {
			registered[k] = v
		}
	}

	if leaf.Looped {
		var priorResults []any
		if prior, ok := state.Registry[leaf.ID]; ok {
			if pr, ok := prior["results"].([]any); ok {
				priorResults = pr
			}
		}
		allResults := make([]any, 0, len(priorResults)+1)
		allResults = append(allResults, priorResults...)
		allResults = append(allResults, registered)

		flatView := map[string]any{}
		for k, v := range registered {
			flatView[k] = v
		}
		flatView["results"] = allResults
		state.Registry[leaf.ID] = flatView
	} else {
		state.Registry[leaf.ID] = registered
	}
}

func invokePackageItem(module, name string, item map[string]any, handler Handler, flatContext map[string]any, filters expr.Filters, apply bool) (Result, error) {
	label := name
	if label == "" {
		label = itemLabel(item)
	}
	state, _ := item["state"].(string)
	if state == "" {
		state = "present"
	}

	ctx := Context{Flat: flatContext, Filters: filters, Apply: apply}

	installed, err := handler.Test(item, name, ctx)
	if err != nil {
		return Result{}, fmt.Errorf("%s %s: Test: %w", module, label, err)
	}
	action, err := resolvePackageAction(state, installed)
	if err != nil {
		return Result{}, fmt.Errorf("%s %s: %w", module, label, err)
	}

	exec := ExecResult{StdoutLines: []string{}, StderrLines: []string{}}
	if action != ActionSkip {
		description, err := handler.Describe(item, action, ctx)
		if err != nil {
			return Result{}, fmt.Errorf("%s %s: Describe: %w", module, label, err)
		}
		if !apply {
			Info("[DryRun][%s] %s", module, description)
		} else {
			Info("[%s] %s", module, description)
			var execErr error
			if action == ActionInstall {
				exec, execErr = handler.Install(item, name, ctx)
			} else {
				exec, execErr = handler.Uninstall(item, name, ctx)
			}
			if execErr != nil {
				msg := execErr.Error()
				Warn("[%s] %s threw: %s", module, label, msg)
				exec = ExecResult{RC: 1, Stderr: msg, StderrLines: []string{msg}}
			}
		}
	}
	if exec.StdoutLines == nil {
		exec.StdoutLines = []string{}
	}
	if exec.StderrLines == nil {
		exec.StderrLines = []string{}
	}

	return Result{
		Module:  module,
		Package: label,
		State:   state,
		Action:  action,
		Apply:   apply,
		Exec:    exec,
	}, nil
}

func resolvePackageAction(state string, installed bool) (Action, error) {
	switch state {
	case "present":
		if installed {
			return ActionSkip, nil
		}
		return ActionInstall, nil
	case "latest":
		return ActionInstall, nil
	case "absent":
		if installed {
			return ActionUninstall, nil
		}
		return ActionSkip, nil
	default:
		return "", fmt.Errorf("unknown state %q", state)
	}
}

// mergeFlatContext ports Common.psm1's Merge-FlatContext: gathered host
// facts and user-registered facts merge under one 'facts' key (user facts
// win); everything else - a leaf's owning package's own local vars, then
// site vars (override), then the growing id-registry (override) - stays
// flattened to bare top-level names, last write wins.
func mergeFlatContext(facts, userFacts, packageVars, packageInputs, packagePackage, vars map[string]any, registry map[string]map[string]any) map[string]any {
	mergedFacts := map[string]any{}
	for k, v := range facts {
		mergedFacts[k] = v
	}
	for k, v := range userFacts {
		mergedFacts[k] = v
	}

	flat := map[string]any{}
	flat["facts"] = mergedFacts
	flat["inputs"] = packageInputs
	flat["package"] = packagePackage
	for k, v := range packageVars {
		flat[k] = v
	}
	for k, v := range vars {
		flat[k] = v
	}
	for k, v := range registry {
		flat[k] = v
	}
	return flat
}

func cloneFlat(ctx map[string]any) map[string]any {
	out := make(map[string]any, len(ctx))
	for k, v := range ctx {
		out[k] = v
	}
	return out
}

func mergeExecInto(ctx map[string]any, exec ExecResult) {
	ctx["rc"] = float64(exec.RC)
	ctx["stdout"] = exec.Stdout
	ctx["stdout_lines"] = toAnySlice(exec.StdoutLines)
	ctx["stderr"] = exec.Stderr
	ctx["stderr_lines"] = toAnySlice(exec.StderrLines)
	for k, v := range exec.Extra {
		ctx[k] = v
	}
}

func toAnySlice(lines []string) []any {
	out := make([]any, len(lines))
	for i, l := range lines {
		out[i] = l
	}
	return out
}

func hasKey(item map[string]any, key string) bool {
	if item == nil {
		return false
	}
	_, ok := item[key]
	return ok
}

func itemLabel(item map[string]any) string {
	for _, key := range []string{"package", "name", "dest", "path", "script"} {
		if s, ok := item[key].(string); ok && s != "" {
			return s
		}
	}
	if cmd, ok := item["command"].(string); ok && cmd != "" {
		firstLine := strings.SplitN(cmd, "\n", 2)[0]
		return strings.TrimSpace(firstLine)
	}
	return "<unknown>"
}

func leafLabel(leaf tasks.Leaf) string {
	if leaf.Name != "" {
		return leaf.Name
	}
	if v, ok := leaf.Item["name"].(string); ok && v != "" {
		return v
	}
	if v, ok := leaf.Item["package"].(string); ok && v != "" {
		return v
	}
	return "<unnamed>"
}
