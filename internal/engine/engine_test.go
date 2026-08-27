package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/internal/expr"
	"github.com/TacoContent/ironstate/internal/filters"
	"github.com/TacoContent/ironstate/internal/secrets"
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

// fakeFactProducerHandler wraps fakeHandler with a FactProducer
// implementation - a stand-in for a real fact-gathering module like
// 'mount_facts' (internal/handlers/mountfacts.go), whose Install result
// merges into state.UserFacts via ExecResult.Extra["value"] without any
// module-name special case in engine.go itself.
type fakeFactProducerHandler struct {
	fakeHandler
	factName string
}

func (h *fakeFactProducerHandler) FactName(item map[string]any) (string, bool) {
	if h.factName != "" {
		return h.factName, true
	}
	name, _ := item["name"].(string)
	return name, name != ""
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
		NoCommandCheckModules: map[string]bool{"widget": true, "fact": true, "assert": true, "mount_facts": true},
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

func TestSecretVarValueIsRegisteredWhenEvaluated(t *testing.T) {
	secrets.Reset()
	secrets.MarkSecretVarPaths(map[string]any{"$token": "super-secret-value"})

	node, err := expr.Parse("vars.token")
	if err != nil {
		t.Fatal(err)
	}
	ctx := map[string]any{"vars": map[string]any{"token": "super-secret-value"}}
	v, err := expr.Eval(node, ctx, filters.New())
	if err != nil {
		t.Fatal(err)
	}
	if v != "super-secret-value" {
		t.Fatalf("Eval(vars.token) = %#v, want secret value", v)
	}
	masked := secrets.Redact("prefix super-secret-value suffix")
	if strings.Contains(masked, "super-secret-value") {
		t.Fatalf("Redact leaked value: %q", masked)
	}
	if !strings.Contains(masked, "***") {
		t.Fatalf("Redact should mask value, got %q", masked)
	}
}

func TestRunLeavesSecretFactRegistersBareNameAndValue(t *testing.T) {
	h := &fakeHandler{installed: false}
	opts := baseOpts(map[string]Handler{"fact": h})
	opts.Apply = true

	state := NewState()
	_, stopped, err := RunLeaves([]tasks.Leaf{
		leaf("fact", map[string]any{"name": "$token", "value": "super-secret-value"}),
	}, opts, state)
	if err != nil || stopped {
		t.Fatalf("err=%v stopped=%v", err, stopped)
	}
	if state.UserFacts["token"] != "super-secret-value" {
		t.Fatalf("UserFacts[\"token\"] = %#v, want secret value stored under bare name", state.UserFacts)
	}
	if !state.SecretFacts["token"] {
		t.Fatalf("SecretFacts[\"token\"] = false, want true for $-prefixed fact")
	}
}

func TestRunLeavesSecretIDRegistersValueAndUsesBareName(t *testing.T) {
	secrets.Reset()
	h := &fakeHandler{installed: false, installExec: ExecResult{RC: 0, Stdout: "super-secret-value"}}
	opts := baseOpts(map[string]Handler{"widget": h})
	opts.Apply = true

	state := NewState()
	_, stopped, err := RunLeaves([]tasks.Leaf{
		leaf("widget", map[string]any{"state": "present"}, withID("$token")),
	}, opts, state)
	if err != nil || stopped {
		t.Fatalf("err=%v stopped=%v", err, stopped)
	}
	if state.Registry["token"]["stdout"] != "super-secret-value" {
		t.Fatalf("registry[\"token\"] = %#v, want secret stdout under bare name", state.Registry["token"])
	}
	masked := secrets.Redact("prefix super-secret-value suffix")
	if strings.Contains(masked, "super-secret-value") {
		t.Fatalf("Redact leaked value: %q", masked)
	}
}

func TestRunLeavesSecretIDSuppressesVerboseOutput(t *testing.T) {
	oldInfo := Info
	oldWarn := Warn
	oldDanger := Danger
	defer func() {
		Info = oldInfo
		Warn = oldWarn
		Danger = oldDanger
	}()

	var infoBuf strings.Builder
	Info = func(format string, args ...any) {
		fmt.Fprintf(&infoBuf, format, args...)
		infoBuf.WriteByte('\n')
	}
	Warn = func(format string, args ...any) {
		infoBuf.WriteString("WARN: ")
		fmt.Fprintf(&infoBuf, format, args...)
		infoBuf.WriteByte('\n')
	}
	Danger = func(format string, args ...any) {
		infoBuf.WriteString("DANGER: ")
		fmt.Fprintf(&infoBuf, format, args...)
		infoBuf.WriteByte('\n')
	}

	fake := &fakeHandler{installed: false, installExec: ExecResult{RC: 0, Stdout: "secret message", Stderr: "internal warning"}}
	opts := baseOpts(map[string]Handler{"widget": fake})
	opts.Apply = true

	_, _, err := RunLeaves([]tasks.Leaf{
		leaf("widget", map[string]any{"state": "present"}, withID("$token"), withName("do-the-thing")),
	}, opts, NewState())
	if err != nil {
		t.Fatal(err)
	}
	out := infoBuf.String()
	if strings.Contains(out, "secret message") || strings.Contains(out, "do-the-thing") || strings.Contains(out, "internal warning") {
		t.Fatalf("secret verbose output leaked into logs: %q", out)
	}
	if !strings.Contains(out, "***") {
		t.Fatalf("expected a redacted description placeholder for secret IDs, got %q", out)
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

// TestRunLeavesWhenSeesBareGatheredFacts guards mergeFlatContext's
// contract (README's "when accepts... a flat context of gathered facts,
// user-defined vars...") - a gathered fact must be usable as a bare
// identifier in 'when', not only under 'facts.*'.
func TestRunLeavesWhenSeesBareGatheredFacts(t *testing.T) {
	h := &fakeHandler{installed: false, installExec: ExecResult{RC: 0}}
	opts := baseOpts(map[string]Handler{"widget": h})
	opts.Apply = true
	opts.Facts = map[string]any{"shell_pwsh": true, "shell_nu": false}

	results, _, err := RunLeaves([]tasks.Leaf{
		leaf("widget", map[string]any{"state": "present"}, withName("a"), withWhen("shell_pwsh")),
		leaf("widget", map[string]any{"state": "present"}, withName("b"), withWhen("shell_nu")),
	}, opts, NewState())
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Package != "a" {
		t.Fatalf("results = %#v, want only leaf 'a' to run ('shell_pwsh' true, 'shell_nu' false)", results)
	}
	if h.installCall != 1 {
		t.Fatalf("expected exactly 1 Install call, got %d", h.installCall)
	}
}

// TestRunLeavesWhenGatesBeforeFieldTemplateResolution guards the fix
// where a leaf's own field '${{ }}' expressions (e.g. a filter call) used
// to resolve BEFORE 'when' was checked - so a filter with real side
// effects/errors ran even for a leaf 'when' would go on to skip. A
// filter that always errors must never even be invoked for a skipped leaf.
func TestRunLeavesWhenGatesBeforeFieldTemplateResolution(t *testing.T) {
	h := &fakeHandler{installed: false, installExec: ExecResult{RC: 0}}
	opts := baseOpts(map[string]Handler{"widget": h})
	opts.Apply = true
	fset := filters.New()
	boomCalls := 0
	fset.Register("boom", func(value any, args []any) (any, error) {
		boomCalls++
		return nil, fmt.Errorf("boom filter always fails")
	})
	opts.Filters = fset

	results, stopped, err := RunLeaves([]tasks.Leaf{
		leaf("widget", map[string]any{"state": "present", "message": "${{ 'x' | boom }}"}, withWhen("false")),
	}, opts, NewState())
	if err != nil || stopped {
		t.Fatalf("err=%v stopped=%v", err, stopped)
	}
	if len(results) != 0 {
		t.Fatalf("expected the leaf to be skipped entirely, got %#v", results)
	}
	if boomCalls != 0 {
		t.Fatalf("expected the 'boom' filter to never be called for a when-skipped leaf, got %d calls", boomCalls)
	}
}

// TestRunLeavesFieldTemplateResolutionErrorRespectsContinueOnError guards
// the fix making a field's own '${{ }}' resolution error behave like a
// handler's Install/Uninstall throwing (a failed leaf, not a fatal Go
// error) - continue_on_error must be able to recover from it.
func TestRunLeavesFieldTemplateResolutionErrorRespectsContinueOnError(t *testing.T) {
	newOpts := func() (Options, *filters.Registry) {
		h := &fakeHandler{installed: false, installExec: ExecResult{RC: 0}}
		opts := baseOpts(map[string]Handler{"widget": h})
		opts.Apply = true
		fset := filters.New()
		fset.Register("boom", func(value any, args []any) (any, error) {
			return nil, fmt.Errorf("boom filter always fails")
		})
		opts.Filters = fset
		return opts, fset
	}

	t.Run("stops by default", func(t *testing.T) {
		opts, _ := newOpts()
		results, stopped, err := RunLeaves([]tasks.Leaf{
			leaf("widget", map[string]any{"state": "present", "message": "${{ 'x' | boom }}"}, withName("a")),
			leaf("widget", map[string]any{"state": "present"}, withName("b")),
		}, opts, NewState())
		if err != nil {
			t.Fatalf("expected a failed leaf, not a fatal error: %v", err)
		}
		if !stopped {
			t.Fatal("expected the run to stop by default")
		}
		if len(results) != 1 || !results[0].Failed || results[0].Exec.RC != 1 {
			t.Fatalf("results = %#v", results)
		}
	})

	t.Run("continues with continue_on_error", func(t *testing.T) {
		opts, _ := newOpts()
		results, stopped, err := RunLeaves([]tasks.Leaf{
			leaf("widget", map[string]any{"state": "present", "message": "${{ 'x' | boom }}"}, withName("a"), withContinueOnError()),
			leaf("widget", map[string]any{"state": "present"}, withName("b")),
		}, opts, NewState())
		if err != nil {
			t.Fatal(err)
		}
		if stopped {
			t.Fatal("expected continue_on_error to keep the run going")
		}
		if len(results) != 2 || !results[0].Failed || results[1].Failed {
			t.Fatalf("results = %#v", results)
		}
	})
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

func TestRunLeavesFactProducerRegistersUserFactValue(t *testing.T) {
	mounts := []any{map[string]any{"path": "/", "fstype": "ext4"}}
	h := &fakeFactProducerHandler{fakeHandler: fakeHandler{
		installExec: ExecResult{RC: 0, Extra: map[string]any{"value": mounts}},
	}}
	opts := baseOpts(map[string]Handler{"mount_facts": h, "widget": h})
	opts.Apply = true

	state := NewState()
	_, stopped, err := RunLeaves([]tasks.Leaf{
		leaf("mount_facts", map[string]any{"name": "mounts"}),
	}, opts, state)
	if err != nil || stopped {
		t.Fatalf("err=%v stopped=%v", err, stopped)
	}
	if got, ok := state.UserFacts["mounts"]; !ok {
		t.Fatalf("UserFacts = %#v, want 'mounts' registered", state.UserFacts)
	} else if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", mounts) {
		t.Fatalf("UserFacts[mounts] = %#v, want %#v", got, mounts)
	}

	// A later leaf's 'when' must see the FactProducer-registered fact,
	// exactly like a plain 'fact' leaf's value (TestRunLeavesFactRegistersUserFactValue).
	results, stopped, err := RunLeaves([]tasks.Leaf{
		leaf("widget", map[string]any{"state": "present"}, withWhen("facts.mounts[0].fstype == \"ext4\"")),
	}, opts, state)
	if err != nil || stopped {
		t.Fatalf("err=%v stopped=%v", err, stopped)
	}
	if len(results) != 1 {
		t.Fatalf("expected the widget leaf to run since its 'when' should see the registered fact, got %#v", results)
	}
}

func TestRunLeavesFactProducerForcesExecutionUnderDryRun(t *testing.T) {
	h := &fakeFactProducerHandler{fakeHandler: fakeHandler{
		installExec: ExecResult{RC: 0, Extra: map[string]any{"value": "gathered"}},
	}}
	opts := baseOpts(map[string]Handler{"mount_facts": h})
	// opts.Apply left false (dry run) - a FactProducer has no real system
	// side effect (it only reads/computes a value), so its Install must
	// still run for real, exactly like a 'fact' with an embedded 'shell' -
	// otherwise every later leaf's dry-run preview would see this fact as
	// undefined.

	state := NewState()
	_, stopped, err := RunLeaves([]tasks.Leaf{
		leaf("mount_facts", map[string]any{"name": "mounts"}),
	}, opts, state)
	if err != nil || stopped {
		t.Fatalf("err=%v stopped=%v", err, stopped)
	}
	if h.installCall != 1 {
		t.Fatalf("Install call count = %d, want 1 (forced even without --apply)", h.installCall)
	}
	if state.UserFacts["mounts"] != "gathered" {
		t.Fatalf("UserFacts[mounts] = %#v, want \"gathered\"", state.UserFacts["mounts"])
	}
}

func TestRunLeavesFactProducerWithNoValueLeavesExistingFactAlone(t *testing.T) {
	h := &fakeFactProducerHandler{fakeHandler: fakeHandler{
		installExec: ExecResult{RC: 1, Stderr: "boom"},
	}}
	opts := baseOpts(map[string]Handler{"mount_facts": h})
	opts.Apply = true

	state := NewState()
	state.UserFacts["mounts"] = "stale"
	_, stopped, err := RunLeaves([]tasks.Leaf{
		leaf("mount_facts", map[string]any{"name": "mounts"}, withContinueOnError()),
	}, opts, state)
	if err != nil || stopped {
		t.Fatalf("err=%v stopped=%v", err, stopped)
	}
	if state.UserFacts["mounts"] != "stale" {
		t.Fatalf("UserFacts[mounts] = %#v, want untouched ('stale') since Install produced no Extra[\"value\"]", state.UserFacts["mounts"])
	}
}

func TestRunLeavesFactProducerAbsentStateUnsetsFact(t *testing.T) {
	h := &fakeFactProducerHandler{fakeHandler: fakeHandler{installed: true}}
	opts := baseOpts(map[string]Handler{"mount_facts": h})
	opts.Apply = true

	state := NewState()
	state.UserFacts["mounts"] = []any{"whatever"}
	_, stopped, err := RunLeaves([]tasks.Leaf{
		leaf("mount_facts", map[string]any{"name": "mounts", "state": "absent"}),
	}, opts, state)
	if err != nil || stopped {
		t.Fatalf("err=%v stopped=%v", err, stopped)
	}
	if _, ok := state.UserFacts["mounts"]; ok {
		t.Fatalf("UserFacts = %#v, want 'mounts' removed", state.UserFacts)
	}
}

func TestRunFactProducerPassBeforeEverythingElse(t *testing.T) {
	widget := &fakeHandler{installed: false}
	mountFacts := &fakeFactProducerHandler{fakeHandler: fakeHandler{
		installExec: ExecResult{RC: 0, Extra: map[string]any{"value": true}},
	}}
	opts := baseOpts(map[string]Handler{"widget": widget, "mount_facts": mountFacts})
	opts.Apply = true

	// Declared in the opposite order on purpose - Run's two-phase split
	// must route the FactProducer leaf into the facts-first phase purely
	// from Handler capability, not a hardcoded module-name check.
	results, stopped, err := Run([]tasks.Leaf{
		leaf("widget", map[string]any{"state": "present"}, withWhen("facts.ready == true")),
		leaf("mount_facts", map[string]any{"name": "ready"}),
	}, opts)
	if err != nil || stopped {
		t.Fatalf("err=%v stopped=%v", err, stopped)
	}
	if len(results) != 2 {
		t.Fatalf("expected both leaves to run, got %#v", results)
	}
	if results[0].Module != "mount_facts" {
		t.Fatalf("expected the mount_facts leaf's result first (facts pass runs before everything else), got %#v", results[0])
	}
}

func TestRunOnFactsGatheredSeesCompleteMergedFactsBeforeOtherLeavesRun(t *testing.T) {
	widget := &fakeHandler{}
	fact := &fakeHandler{installExec: ExecResult{RC: 0}}
	mountFacts := &fakeFactProducerHandler{fakeHandler: fakeHandler{
		installExec: ExecResult{RC: 0, Extra: map[string]any{"value": []any{"disk0"}}},
	}}
	opts := baseOpts(map[string]Handler{"widget": widget, "fact": fact, "mount_facts": mountFacts})
	opts.Apply = true
	opts.Facts = map[string]any{"platform": "linux", "overridden_by_user_fact": "host-value"}

	var seen map[string]any
	calls := 0
	opts.OnFactsGathered = func(allFacts map[string]any) {
		calls++
		seen = allFacts
		// Must fire strictly after both fact-producing leaves have run,
		// and strictly before the non-fact 'widget' leaf below does.
		if widget.installCall != 0 {
			t.Fatal("OnFactsGathered fired after a non-fact leaf already ran")
		}
	}

	results, stopped, err := Run([]tasks.Leaf{
		leaf("fact", map[string]any{"name": "overridden_by_user_fact", "value": "user-value"}),
		leaf("mount_facts", map[string]any{"name": "mounts"}),
		leaf("widget", map[string]any{"state": "present"}),
	}, opts)
	if err != nil || stopped {
		t.Fatalf("err=%v stopped=%v", err, stopped)
	}
	if len(results) != 3 {
		t.Fatalf("expected all three leaves to run, got %#v", results)
	}
	if calls != 1 {
		t.Fatalf("OnFactsGathered called %d times, want exactly 1", calls)
	}
	if seen["platform"] != "linux" {
		t.Fatalf("seen[platform] = %#v, want the host fact untouched", seen["platform"])
	}
	if seen["overridden_by_user_fact"] != "user-value" {
		t.Fatalf("seen[overridden_by_user_fact] = %#v, want the user fact to win over the host fact of the same name", seen["overridden_by_user_fact"])
	}
	mounts, ok := seen["mounts"].([]any)
	if !ok || len(mounts) != 1 || mounts[0] != "disk0" {
		t.Fatalf("seen[mounts] = %#v, want the mount_facts leaf's gathered value", seen["mounts"])
	}
}

type stubErr string

func (e stubErr) Error() string { return string(e) }

var errNotFound = stubErr("not found")
