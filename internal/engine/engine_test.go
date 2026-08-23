package engine

import (
	"testing"

	"github.com/TacoContent/ironstate/internal/filters"
	"github.com/TacoContent/ironstate/internal/tasks"
)

// fakeHandler is a minimal, call-tracking Handler double used to drive
// internal/engine's dispatch loop without any real filesystem/CLI I/O -
// exactly what the "prove the loop end-to-end" Phase 3 exit criterion
// calls for (docs/plans/go-rewrite.md §10).
type fakeHandler struct {
	installed   bool
	installCall int
	uninstCall  int
	testErr     error
	installExec ExecResult
	installErr  error
}

func (h *fakeHandler) Test(item map[string]any, name string, ctx Context) (bool, error) {
	return h.installed, h.testErr
}
func (h *fakeHandler) Describe(item map[string]any, action Action, ctx Context) (string, error) {
	return string(action), nil
}
func (h *fakeHandler) Install(item map[string]any, name string, ctx Context) (ExecResult, error) {
	h.installCall++
	return h.installExec, h.installErr
}
func (h *fakeHandler) Uninstall(item map[string]any, name string, ctx Context) (ExecResult, error) {
	h.uninstCall++
	return h.installExec, h.installErr
}

func leaf(module string, item map[string]any, opts ...func(*tasks.Leaf)) tasks.Leaf {
	l := tasks.Leaf{Module: module, Item: item}
	for _, o := range opts {
		o(&l)
	}
	return l
}

func withID(id string) func(*tasks.Leaf)     { return func(l *tasks.Leaf) { l.ID = id } }
func withName(name string) func(*tasks.Leaf) { return func(l *tasks.Leaf) { l.Name = name } }
func withWhen(when ...any) func(*tasks.Leaf) { return func(l *tasks.Leaf) { l.When = when } }
func withFailedWhen(fw ...any) func(*tasks.Leaf) {
	return func(l *tasks.Leaf) { l.FailedWhen = fw }
}
func withContinueOnError() func(*tasks.Leaf) { return func(l *tasks.Leaf) { l.ContinueOnError = true } }
func withLooped() func(*tasks.Leaf)          { return func(l *tasks.Leaf) { l.Looped = true } }

func baseOpts(handlers map[string]Handler) Options {
	return Options{
		Handlers: handlers,
		Facts:    map[string]any{},
		Vars:     map[string]any{},
		Filters:  filters.New(),
		// The fake modules used throughout this test file stand in for a
		// real handler with no real backing CLI - "winget" is deliberately
		// left out so TestRunLeavesMissingCommandOnPathProducesNoResultRow
		// can still exercise the real PATH-check path.
		NoCommandCheckModules: map[string]bool{"widget": true, "fact": true, "assert": true},
	}
}

func TestRunLeavesDryRunDoesNotCallInstall(t *testing.T) {
	h := &fakeHandler{installed: false}
	opts := baseOpts(map[string]Handler{"widget": h})
	opts.Apply = false

	results, stopped, err := RunLeaves([]tasks.Leaf{
		leaf("widget", map[string]any{"state": "present"}, withName("w")),
	}, opts, NewState())
	if err != nil || stopped {
		t.Fatalf("err=%v stopped=%v", err, stopped)
	}
	if h.installCall != 0 {
		t.Fatalf("Install should not run under dry-run, called %d times", h.installCall)
	}
	if len(results) != 1 || results[0].Action != ActionInstall {
		t.Fatalf("results = %#v", results)
	}
	if results[0].Apply {
		t.Fatalf("result.Apply should be false in dry-run, got %#v", results[0])
	}
}

func TestRunLeavesApplyCallsInstall(t *testing.T) {
	h := &fakeHandler{installed: false, installExec: ExecResult{RC: 0, Stdout: "ok"}}
	opts := baseOpts(map[string]Handler{"widget": h})
	opts.Apply = true

	results, _, err := RunLeaves([]tasks.Leaf{
		leaf("widget", map[string]any{"state": "present"}, withName("w")),
	}, opts, NewState())
	if err != nil {
		t.Fatal(err)
	}
	if h.installCall != 1 {
		t.Fatalf("expected Install to run once, got %d", h.installCall)
	}
	if results[0].Exec.Stdout != "ok" {
		t.Fatalf("exec = %#v", results[0].Exec)
	}
}

func TestRunLeavesAssertForcesExecutionUnderDryRun(t *testing.T) {
	h := &fakeHandler{installed: false, installExec: ExecResult{RC: 0}}
	opts := baseOpts(map[string]Handler{"assert": h})
	opts.Apply = false // dry-run

	results, _, err := RunLeaves([]tasks.Leaf{
		leaf("assert", map[string]any{"that": []any{}}, withName("a")),
	}, opts, NewState())
	if err != nil {
		t.Fatal(err)
	}
	if h.installCall != 1 {
		t.Fatalf("assert must actually execute even under dry-run, Install called %d times", h.installCall)
	}
	if !results[0].Apply {
		t.Fatalf("assert's own result should report Apply=true even in a dry-run")
	}
}

func TestRunLeavesFactEmbeddedShellForcesExecutionUnderDryRun(t *testing.T) {
	h := &fakeHandler{installed: false, installExec: ExecResult{RC: 0, Stdout: "computed"}}
	opts := baseOpts(map[string]Handler{"fact": h})
	opts.Apply = false

	results, _, err := RunLeaves([]tasks.Leaf{
		leaf("fact", map[string]any{"name": "myfact", "shell": map[string]any{"command": "echo hi"}}, withName("f")),
	}, opts, NewState())
	if err != nil {
		t.Fatal(err)
	}
	if h.installCall != 1 {
		t.Fatalf("a fact with an embedded shell must actually execute even under dry-run, called %d times", h.installCall)
	}
	if !results[0].Apply {
		t.Fatal("expected Apply=true forced for embedded-shell fact")
	}
}

func TestRunLeavesFactWithoutShellDoesNotForceApply(t *testing.T) {
	h := &fakeHandler{installed: false, installExec: ExecResult{RC: 0}}
	opts := baseOpts(map[string]Handler{"fact": h})
	opts.Apply = false

	results, _, err := RunLeaves([]tasks.Leaf{
		leaf("fact", map[string]any{"name": "myfact", "value": "x"}, withName("f")),
	}, opts, NewState())
	if err != nil {
		t.Fatal(err)
	}
	if h.installCall != 0 {
		t.Fatalf("a plain fact (no embedded shell) should not force real execution under dry-run")
	}
	if results[0].Apply {
		t.Fatal("expected Apply=false for a plain fact under dry-run")
	}
}

func TestRunLeavesFactRegistersUserFactValue(t *testing.T) {
	h := &fakeHandler{installed: false}
	opts := baseOpts(map[string]Handler{"fact": h, "widget": h})
	opts.Apply = true

	state := NewState()
	_, stopped, err := RunLeaves([]tasks.Leaf{
		leaf("fact", map[string]any{"name": "greeting", "value": "hello"}),
	}, opts, state)
	if err != nil || stopped {
		t.Fatalf("err=%v stopped=%v", err, stopped)
	}
	if state.UserFacts["greeting"] != "hello" {
		t.Fatalf("UserFacts = %#v", state.UserFacts)
	}

	// A later leaf's 'when' must see the fact registered above.
	results, stopped, err := RunLeaves([]tasks.Leaf{
		leaf("widget", map[string]any{"state": "present"}, withWhen("facts.greeting == \"hello\"")),
	}, opts, state)
	if err != nil || stopped {
		t.Fatalf("err=%v stopped=%v", err, stopped)
	}
	if len(results) != 1 {
		t.Fatalf("expected the widget leaf to run since its 'when' should see the registered fact, got %#v", results)
	}
}

func TestRunLeavesIDRegistersResultForLaterTemplateUse(t *testing.T) {
	h := &fakeHandler{installed: false, installExec: ExecResult{RC: 0, Stdout: "hello-world"}}
	opts := baseOpts(map[string]Handler{"widget": h})
	opts.Apply = true

	state := NewState()
	_, _, err := RunLeaves([]tasks.Leaf{
		leaf("widget", map[string]any{"state": "present"}, withID("w1")),
	}, opts, state)
	if err != nil {
		t.Fatal(err)
	}
	if state.Registry["w1"]["stdout"] != "hello-world" {
		t.Fatalf("registry[w1] = %#v", state.Registry["w1"])
	}

	// A later leaf's item can reference '${{ w1.stdout }}'.
	results, _, err := RunLeaves([]tasks.Leaf{
		leaf("widget", map[string]any{"state": "present", "message": "${{ w1.stdout }}"}),
	}, opts, state)
	if err != nil {
		t.Fatal(err)
	}
	// (fakeHandler doesn't consume 'message', but the resolve must not error out
	// and the item map must carry the resolved value.)
	_ = results
}

func TestRunLeavesLoopedIDAccumulatesResults(t *testing.T) {
	h := &fakeHandler{installed: false, installExec: ExecResult{RC: 0, Stdout: "x"}}
	opts := baseOpts(map[string]Handler{"widget": h})
	opts.Apply = true

	state := NewState()
	_, _, err := RunLeaves([]tasks.Leaf{
		leaf("widget", map[string]any{"state": "present"}, withID("loopid"), withLooped()),
		leaf("widget", map[string]any{"state": "present"}, withID("loopid"), withLooped()),
	}, opts, state)
	if err != nil {
		t.Fatal(err)
	}
	results, ok := state.Registry["loopid"]["results"].([]any)
	if !ok || len(results) != 2 {
		t.Fatalf("expected 2 accumulated results, got %#v", state.Registry["loopid"])
	}
}

func TestRunLeavesFailedWhenStopsRunByDefault(t *testing.T) {
	h := &fakeHandler{installed: false, installExec: ExecResult{RC: 0}}
	opts := baseOpts(map[string]Handler{"widget": h})
	opts.Apply = true

	results, stopped, err := RunLeaves([]tasks.Leaf{
		leaf("widget", map[string]any{"state": "present"}, withFailedWhen("rc == 0")),
		leaf("widget", map[string]any{"state": "present"}),
	}, opts, NewState())
	if err != nil {
		t.Fatal(err)
	}
	if !stopped {
		t.Fatal("expected the run to stop on the first leaf's failed_when")
	}
	if len(results) != 1 {
		t.Fatalf("expected only the first (failed) leaf's result, got %#v", results)
	}
	if !results[0].Failed {
		t.Fatalf("expected Failed=true, got %#v", results[0])
	}
}

func TestRunLeavesContinueOnErrorKeepsGoing(t *testing.T) {
	h := &fakeHandler{installed: false, installExec: ExecResult{RC: 0}}
	opts := baseOpts(map[string]Handler{"widget": h})
	opts.Apply = true

	results, stopped, err := RunLeaves([]tasks.Leaf{
		leaf("widget", map[string]any{"state": "present"}, withFailedWhen("rc == 0"), withContinueOnError()),
		leaf("widget", map[string]any{"state": "present"}),
	}, opts, NewState())
	if err != nil {
		t.Fatal(err)
	}
	if stopped {
		t.Fatal("expected continue_on_error to keep the run going")
	}
	if len(results) != 2 {
		t.Fatalf("expected both leaves to run, got %#v", results)
	}
}

func TestRunLeavesWhenFalseSkipsLeaf(t *testing.T) {
	h := &fakeHandler{installed: false}
	opts := baseOpts(map[string]Handler{"widget": h})
	opts.Apply = true

	results, _, err := RunLeaves([]tasks.Leaf{
		leaf("widget", map[string]any{"state": "present"}, withWhen("1 == 2")),
	}, opts, NewState())
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected the leaf to be skipped entirely (no result row), got %#v", results)
	}
	if h.installCall != 0 {
		t.Fatal("Install should not run when 'when' is false")
	}
}

func TestRunLeavesMissingHandlerProducesNoResultRow(t *testing.T) {
	opts := baseOpts(map[string]Handler{})
	results, stopped, err := RunLeaves([]tasks.Leaf{
		leaf("nosuchmodule", map[string]any{}),
	}, opts, NewState())
	if err != nil || stopped {
		t.Fatalf("err=%v stopped=%v", err, stopped)
	}
	if len(results) != 0 {
		t.Fatalf("expected no result row for a leaf with no registered handler, got %#v", results)
	}
}

func TestRunLeavesMissingCommandOnPathProducesNoResultRow(t *testing.T) {
	h := &fakeHandler{installed: false}
	opts := baseOpts(map[string]Handler{"winget": h})
	origLookPath := LookPath
	LookPath = func(name string) (string, error) { return "", errNotFound }
	defer func() { LookPath = origLookPath }()

	results, _, err := RunLeaves([]tasks.Leaf{
		leaf("winget", map[string]any{"state": "present"}),
	}, opts, NewState())
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no result row when the backing command isn't on PATH, got %#v", results)
	}
}

func TestRunFactsPassBeforeEverythingElse(t *testing.T) {
	widget := &fakeHandler{installed: false}
	factHandler := &fakeHandler{installed: false}
	opts := baseOpts(map[string]Handler{"widget": widget, "fact": factHandler})
	opts.Apply = true

	// The widget leaf is declared *first* but references a fact declared
	// *after* it - Run's two-phase split must still make the fact visible.
	results, stopped, err := Run([]tasks.Leaf{
		leaf("widget", map[string]any{"state": "present"}, withWhen("facts.ready == true")),
		leaf("fact", map[string]any{"name": "ready", "value": true}),
	}, opts)
	if err != nil || stopped {
		t.Fatalf("err=%v stopped=%v", err, stopped)
	}
	if len(results) != 2 {
		t.Fatalf("expected both the fact and the widget leaf to run, got %#v", results)
	}
	if results[0].Module != "fact" {
		t.Fatalf("expected the fact leaf's result first (facts pass runs before everything else), got %#v", results[0])
	}
}

type stubErr string

func (e stubErr) Error() string { return string(e) }

var errNotFound = stubErr("not found")
