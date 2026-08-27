package handlers

import (
	"fmt"
	"time"

	"github.com/TacoContent/ironstate/internal/conditions"
	"github.com/TacoContent/ironstate/internal/engine"
	"github.com/TacoContent/ironstate/internal/expr"
	"github.com/TacoContent/ironstate/internal/facts"
)

// mountFactsHandler gathers the host's current mounts (internal/facts/
// mounts*.go) and stores them as a fact, callable as a normal task so it
// only runs (and pays the gathering cost) when a playbook actually asks
// for it — mirrors Ansible's mount_facts module (docs/handlers/
// mount_facts.md), reusing the 'fact' module's present/absent idiom: Test
// always reports "not installed" except for state 'absent', so
// 'present'/'latest' always (re)gather and 'absent' always unsets the
// fact instead. Unlike 'fact', its value isn't read from the item — it
// implements engine.FactProducer so internal/engine's dispatch loop merges
// Install's gathered mounts into state.UserFacts on its own.
type mountFactsHandler struct{}

// mountFactsName ports Common.psm1's Get-ItemState default-fallback idiom
// for this module's one configurable field: 'name' defaults to "mounts",
// so 'facts.mounts' works with no configuration at all.
func mountFactsName(item map[string]any) string {
	return getStringOr(item, "name", "mounts")
}

// mountFactsTimeout reads 'timeout' (seconds, default 10) - 0 or less
// means "no bound", matching internal/facts.GatherMounts' contract.
func mountFactsTimeout(item map[string]any) time.Duration {
	seconds := getFloat(item, "timeout", 10)
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds * float64(time.Second))
}

func (mountFactsHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	return itemState(item) == "absent", nil
}

func (mountFactsHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	factName := mountFactsName(item)
	if action == engine.ActionUninstall {
		return fmt.Sprintf("unset fact '%s'", factName), nil
	}
	return fmt.Sprintf("gather mount facts -> fact '%s'", factName), nil
}

func (mountFactsHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	mounts, err := facts.GatherMounts(mountFactsTimeout(item))
	if err != nil {
		msg := fmt.Sprintf("mount_facts: %v", err)
		return engine.ExecResult{RC: 1, Stderr: msg, StderrLines: []string{msg}}, nil
	}

	value, err := filterMounts(mounts, asList(item["filter"]), ctx.Filters)
	if err != nil {
		msg := fmt.Sprintf("mount_facts: %v", err)
		return engine.ExecResult{RC: 1, Stderr: msg, StderrLines: []string{msg}}, nil
	}
	message := fmt.Sprintf("gathered %d mount(s)", len(value))
	return engine.ExecResult{
		RC:          0,
		Stdout:      message,
		StdoutLines: []string{message},
		Extra:       map[string]any{"value": value},
	}, nil
}

// filterMounts keeps only the mounts that pass every 'filter' condition -
// a list of bare when/that-style expressions (implicit AND, same as
// conditions.TestWhen elsewhere), each evaluated against a single mount's
// own fields (device/fstype/options/path/source) in isolation. Mirrors
// Ansible's mount_facts filter kwargs but as free-form expressions instead
// of fixed device/fstype allow-lists.
func filterMounts(mounts []facts.MountFact, filter []any, filters expr.Filters) ([]any, error) {
	value := make([]any, 0, len(mounts))
	for _, m := range mounts {
		mountMap := m.AsMap()
		keep, err := conditions.TestWhen(filter, mountMap, filters)
		if err != nil {
			return nil, err
		}
		if !keep {
			continue
		}
		value = append(value, mountMap)
	}
	return value, nil
}

func (mountFactsHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, nil
}

// FactName implements engine.FactProducer - see mount_facts.go's doc
// comment above.
func (mountFactsHandler) FactName(item map[string]any) (string, bool) {
	name := mountFactsName(item)
	return name, name != ""
}
