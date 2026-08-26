package handlers

import (
	"fmt"

	"github.com/TacoContent/ironstate/internal/engine"
	"github.com/TacoContent/ironstate/internal/tasks"
)

// asyncHandler runs a nested 'tasks' list in a background goroutine and
// returns immediately, letting the rest of the playbook continue without
// waiting for it. A later 'wait_for' leaf finds this job again by the
// 'id' given here (see engine.StartAsyncJob/LookupAsyncJob) - deliberately
// a field of the async item itself, not this leaf's own top-level 'id'
// (which, like any leaf, just registers this leaf's own immediate
// "started" result and is unrelated).
//
// Deviation, documented: the nested tasks run against a private snapshot
// of the facts/vars/registry available at dispatch time (ctx.Flat), using
// a fresh engine.State - not the live, still-mutating State the rest of
// the run uses (which is not safe for concurrent goroutine access, and
// keeps flowing forward across the sequential run in a way a background
// job can't participate in coherently). So a fact/id set *inside* an
// async block is never visible to sibling leaves - only the aggregate
// exec results ('wait_for.results.<id>') are, once 'wait_for' observes
// completion.
type asyncHandler struct{}

func (asyncHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	return false, nil // always "not installed" -> Install always dispatches
}

func (asyncHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	id := getString(item, "id")
	n := len(asList(item["tasks"]))
	return fmt.Sprintf("async: run %d task(s) in background as '%s'", n, id), nil
}

func (h asyncHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	id := getString(item, "id")
	if id == "" {
		return engine.ExecResult{}, fmt.Errorf("async: 'id' is required so 'wait_for' can reference this job")
	}
	rawTasks := asList(item["tasks"])
	if len(rawTasks) == 0 {
		return engine.ExecResult{}, fmt.Errorf("async '%s': 'tasks' must be a non-empty list", id)
	}

	// Snapshot: ctx.Flat already merges facts+vars+registry down to bare
	// top-level names (internal/engine's mergeFlatContext) - reusing it
	// as the nested run's 'Facts' gives the background tasks the same
	// names visible here, without needing the separate raw facts/vars
	// maps a Handler is never given.
	snapshot := make(map[string]any, len(ctx.Flat))
	for k, v := range ctx.Flat {
		snapshot[k] = v
	}

	leaves, err := tasks.Expand(rawTasks, tasks.Options{
		ModuleNames: AllModuleNames,
		Facts:       snapshot,
		Filters:     ctx.Filters,
	})
	if err != nil {
		return engine.ExecResult{}, fmt.Errorf("async '%s': %w", id, err)
	}

	job := engine.StartAsyncJob(id)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				job.Finish(nil, true, fmt.Errorf("async '%s': panic: %v", id, r))
			}
		}()
		results, stopped, runErr := engine.RunLeaves(leaves, engine.Options{
			Handlers: All(),
			Facts:    snapshot,
			Filters:  ctx.Filters,
			Apply:    ctx.Apply,
		}, engine.NewState())
		failed := stopped || runErr != nil
		if !failed {
			for _, r := range results {
				if r.Failed {
					failed = true
					break
				}
			}
		}
		job.Finish(results, failed, runErr)
	}()

	message := fmt.Sprintf("started async job '%s' (%d task(s))", id, len(leaves))
	engine.Info("%s", message)
	return engine.ExecResult{RC: 0, Stdout: message, StdoutLines: []string{message}}, nil
}

func (h asyncHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return h.Install(item, name, ctx) // never reached: Test always reports "not installed"
}
