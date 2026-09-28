package engine

import (
	"strings"
	"testing"

	ironexec "github.com/TacoContent/ironstate/internal/exec"
	"github.com/TacoContent/ironstate/internal/tasks"
)

func withIsolated(facts, vars map[string]any) func(*tasks.Leaf) {
	return func(l *tasks.Leaf) {
		l.Isolated = true
		l.IsolatedFacts = facts
		l.IsolatedVars = vars
	}
}

// TestRunLeavesIsolatedLeafSeesOnlyPassedContext guards the 'uses:'
// 'isolate: true' contract: an isolated leaf must never see the host's
// gathered facts, the site's vars, or the id registry - only what its
// consuming task handed it in 'with'.
func TestRunLeavesIsolatedLeafSeesOnlyPassedContext(t *testing.T) {
	h := &fakeHandler{installed: false, installExec: ExecResult{RC: 0}}
	opts := baseOpts(map[string]Handler{"widget": h})
	opts.Apply = true
	opts.Facts = map[string]any{"host_secret": "leaked"}
	opts.Vars = map[string]any{"site_token": "leaked"}

	state := NewState()
	state.Registry["earlier"] = map[string]any{"stdout": "leaked"}
	state.UserFacts["user_secret"] = "leaked"

	results, _, err := RunLeaves([]tasks.Leaf{
		leaf("widget", map[string]any{"state": "present"},
			withName("sandboxed"),
			withWhen("host_secret is not defined and site_token is not defined and earlier is not defined and user_secret is not defined and allowed == 'yes'"),
			withIsolated(map[string]any{"platform": "linux"}, map[string]any{"allowed": "yes"}),
		),
	}, opts, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("isolated leaf did not run with only its passed-in context: %#v", results)
	}
}

// TestRunLeavesIsolatedLeafCannotBecome guards the second half of the
// isolation contract: a sandboxed task from a 'uses:' may not elevate.
func TestRunLeavesIsolatedLeafCannotBecome(t *testing.T) {
	h := &fakeHandler{installed: false, installExec: ExecResult{RC: 0}}
	opts := baseOpts(map[string]Handler{"widget": h})
	opts.Apply = true

	origWarn := Warn
	t.Cleanup(func() { Warn = origWarn })
	var warnings []string
	Warn = func(format string, args ...any) { warnings = append(warnings, format) }

	_, _, err := RunLeaves([]tasks.Leaf{
		leaf("widget", map[string]any{"state": "present"},
			withBecome("root"),
			withIsolated(map[string]any{}, map[string]any{}),
		),
	}, opts, NewState())
	if err != nil {
		t.Fatal(err)
	}
	if h.seenBecome != (ironexec.Become{}) {
		t.Fatalf("ctx.Become = %+v, want no elevation inside an isolated 'uses'", h.seenBecome)
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "isolated 'uses'") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a warning about 'become' in an isolated 'uses', got %#v", warnings)
	}
}
