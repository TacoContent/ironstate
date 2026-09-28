package packages

import (
	"fmt"
	"path/filepath"

	"github.com/TacoContent/ironstate/internal/expr"
	"github.com/TacoContent/ironstate/internal/model"
	"github.com/TacoContent/ironstate/internal/remote"
)

// Used is the result of resolving one 'uses:' spec - the remote or
// local counterpart to Included (see include.go). Beyond Included's
// shape it also carries the directory the used document came from (so a
// nested 'include:' inside it resolves against that tree, not the
// consuming playbook) and, when isolated, the only facts/vars its tasks
// may ever see.
type Used struct {
	Data     any
	Inputs   map[string]any
	Package  map[string]any
	Root     string
	Isolated bool
	Facts    map[string]any
	Vars     map[string]any
	Source   string
}

// UseOptions carries everything LoadUsedPackage needs beyond the spec
// itself.
type UseOptions struct {
	// BaseDir anchors a relative local 'remote' path - normally the
	// consuming playbook's own directory.
	BaseDir string
	// Facts and Vars are the consuming playbook's context. A
	// non-isolated 'uses' sees them; an isolated one never does.
	Facts map[string]any
	Vars  map[string]any
	// Filters is the expression filter set used for template resolution.
	Filters expr.Filters
	// AllowRemote pre-approves a non-isolated remote source instead of
	// prompting (--allow-remote-uses).
	AllowRemote bool
}

// LoadUsedPackage resolves a 'uses:' spec's source (git over ssh or
// https, a local directory, or a network share), loads the document
// found there, and resolves its templates the same way
// LoadIncludedPackage does for a local 'include:'.
//
// Returns (nil, nil) - warned, not errored, matching 'include:' - when
// the spec has no 'remote' or the operator declines a remote,
// non-isolated source. A source that resolves but then fails to load is
// a real error.
func LoadUsedPackage(usesSpec map[string]any, opts UseOptions) (*Used, error) {
	remoteVal, _ := model.Prop(usesSpec, "remote")
	remoteStr, _ := remoteVal.(string)
	if remoteStr == "" {
		Warn("uses has no 'remote'; skipping")
		return nil, nil
	}

	pathStr, _ := model.PropOr(usesSpec, "path", "").(string)
	refStr, _ := model.PropOr(usesSpec, "ref", "").(string)
	isolated, _ := model.PropOr(usesSpec, "isolate", false).(bool)
	trusted, _ := model.PropOr(usesSpec, "trusted", false).(bool)

	withMap := model.AsMap(model.PropOr(usesSpec, "with", map[string]any{}))
	withVars := model.AsMap(withMap["vars"])
	withFacts := model.AsMap(withMap["facts"])
	// Any remaining 'with' key is an ordinary input, exactly like
	// 'include's own 'with' - only 'vars'/'facts' are special.
	inputs := map[string]any{}
	for k, v := range withMap {
		if k == "vars" || k == "facts" {
			continue
		}
		inputs[k] = v
	}

	spec := remote.Spec{Remote: remoteStr, Path: pathStr, Ref: refStr}
	kind, source := remote.Describe(spec)
	if remote.NeedsConfirmation(kind, isolated) && !trusted && !opts.AllowRemote {
		ok, err := remote.Confirm(kind, source)
		if err != nil {
			return nil, err
		}
		if !ok {
			Warn("remote source '%s' was declined; skipping (mark it 'trusted: true', use 'isolate: true', or pass --allow-remote-uses to pre-approve)", source)
			return nil, nil
		}
	}

	res, err := remote.Resolve(spec, opts.BaseDir)
	if err != nil {
		return nil, err
	}

	// An isolated 'uses' only ever sees what it was explicitly handed.
	effFacts, effVars := opts.Facts, opts.Vars
	if isolated {
		effFacts, effVars = withFacts, withVars
	} else if len(withVars) > 0 {
		merged := make(map[string]any, len(opts.Vars)+len(withVars))
		for k, v := range opts.Vars {
			merged[k] = v
		}
		for k, v := range withVars {
			merged[k] = v
		}
		effVars = merged
	}
	if effFacts == nil {
		effFacts = map[string]any{}
	}
	if effVars == nil {
		effVars = map[string]any{}
	}

	docFile, err := ResolvePlaybookPath(res.Dir)
	if err != nil {
		return nil, fmt.Errorf("uses '%s': %w", res.Source, err)
	}
	usedRoot := filepath.Dir(docFile)

	data, err := LoadFile(docFile, usedRoot)
	if err != nil {
		return nil, err
	}
	data, err = MergeChainOverlays(data, usedRoot, usedRoot, effFacts)
	if err != nil {
		return nil, err
	}

	pkg := map[string]any{
		"name":   model.PropOr(usesSpec, "name", res.Source),
		"state":  model.PropOr(usesSpec, "state", "present"),
		"tags":   toAnySlice(model.AsStringSlice(model.PropOr(usesSpec, "tags", nil))),
		"source": res.Source,
	}

	ctx := map[string]any{
		"package": pkg,
		"inputs":  inputs,
		"facts":   effFacts,
		"vars":    effVars,
	}
	for key, val := range model.Vars(data) {
		if reservedNamespaces[key] {
			Warn("uses '%s': its own 'vars.%s' collides with a reserved namespace name; ignoring", res.Source, key)
			continue
		}
		if _, siteWins := effVars[key]; siteWins {
			// Same deferral reasoning as LoadIncludedPackage's - let the
			// authoritative per-leaf pass resolve it.
			continue
		}
		ctx[key] = val
	}

	resolved, err := resolveTemplatesInPlace(data, ctx, opts.Filters, res.Source)
	if err != nil {
		return nil, err
	}

	return &Used{
		Data:     resolved,
		Inputs:   inputs,
		Package:  pkg,
		Root:     usedRoot,
		Isolated: isolated,
		Facts:    effFacts,
		Vars:     effVars,
		Source:   res.Source,
	}, nil
}
