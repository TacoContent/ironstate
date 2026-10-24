package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/internal/filters"
	"github.com/TacoContent/ironstate/internal/tasks"
)

func TestSilentShellOutputPreservesLifecycleAndResults(t *testing.T) {
	for _, tc := range []struct {
		name       string
		rc         int
		failedWhen []any
		apply      bool
		installed  bool
		failed     bool
		lifecycle  string
	}{
		{name: "success", apply: true, lifecycle: "installed"},
		{name: "failure", apply: true, rc: 7, failed: true, lifecycle: "installing"},
		{name: "failed_when failure", apply: true, failedWhen: []any{true}, failed: true, lifecycle: "installing"},
		{name: "handled failure", apply: true, rc: 7, failedWhen: []any{false}, lifecycle: "installing"},
		{name: "dry run", lifecycle: "would install"},
		{name: "unchanged", apply: true, installed: true, lifecycle: "skip"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var messages []string
			logger := func(format string, args ...any) { messages = append(messages, fmt.Sprintf(format, args...)) }
			oldInfo, oldWarn, oldDanger := Info, Warn, Danger
			Info, Warn, Danger = logger, logger, logger
			t.Cleanup(func() { Info, Warn, Danger = oldInfo, oldWarn, oldDanger })
			handler := &fakeHandler{
				installed:   tc.installed,
				installExec: ExecResult{RC: tc.rc, Stdout: "captured-stdout", Stderr: "captured-stderr"},
			}
			opts := baseOpts(map[string]Handler{"shell": handler})
			opts.Apply = tc.apply
			opts.Verbose = true
			opts.Vars = map[string]any{"quiet": true}
			progress := 0
			opts.Progress = func(string, string, int, int) { progress++ }
			var reported []Result
			opts.OnResult = func(result Result) { reported = append(reported, result) }
			state := NewState()
			results, stopped, err := RunLeaves([]tasks.Leaf{{
				Module: "shell", ID: "captured", ContinueOnError: true, FailedWhen: tc.failedWhen,
				Item: map[string]any{"command": "example", "silent": "${{ vars.quiet }}"},
			}}, opts, state)
			if err != nil || stopped || len(results) != 1 {
				t.Fatalf("results=%#v stopped=%v err=%v", results, stopped, err)
			}
			result := results[0]
			if result.Failed != tc.failed || progress != 1 || len(reported) != 1 {
				t.Fatalf("result=%#v progress=%d reported=%#v", result, progress, reported)
			}
			if state.Registry["captured"] == nil || ComputeStats(results).Total != 1 {
				t.Fatal("silent execution lost its ID or statistics")
			}
			text := strings.Join(messages, "\n")
			if !strings.Contains(text, tc.lifecycle) {
				t.Fatalf("lifecycle message missing %q: %s", tc.lifecycle, text)
			}
			if tc.failed {
				for _, expected := range []string{"captured-stdout", "captured-stderr", "failed"} {
					if !strings.Contains(text, expected) {
						t.Fatalf("failure diagnostics missing %q: %s", expected, text)
					}
				}
			} else if strings.Contains(text, "captured-stdout") || strings.Contains(text, "captured-stderr") {
				t.Fatalf("successful silent shell logged execution output: %s", text)
			}
		})
	}
}

func TestSilentShellRejectsNonBooleanProperty(t *testing.T) {
	opts := baseOpts(map[string]Handler{"shell": &fakeHandler{}})
	_, _, err := RunLeaves([]tasks.Leaf{{
		Module: "shell", Item: map[string]any{"command": "example", "silent": "yes"},
	}}, opts, NewState())
	if err == nil || !strings.Contains(err.Error(), "shell.silent") {
		t.Fatalf("err=%v", err)
	}
}

func TestSilentEnvironmentSourceFailureReportsDiagnostics(t *testing.T) {
	var messages []string
	oldInfo, oldWarn := Info, Warn
	logger := func(format string, args ...any) { messages = append(messages, fmt.Sprintf(format, args...)) }
	Info, Warn = logger, logger
	t.Cleanup(func() { Info, Warn = oldInfo, oldWarn })
	shell := &fakeHandler{installExec: ExecResult{RC: 7, Stdout: "source-out", Stderr: "source-err"}}
	values := map[string]any{"VALUE": envSource(map[string]any{"task": map[string]any{
		"shell": map[string]any{"command": "example", "silent": true}, "value": "${{ stdout }}",
	}})}
	_, err := ResolveEnvironment(values, Context{Flat: map[string]any{}, Filters: filters.New()}, shell, nil)
	if err == nil || !strings.Contains(err.Error(), "code 7") {
		t.Fatalf("err=%v", err)
	}
	output := strings.Join(messages, "\n")
	if !strings.Contains(output, "source-out") || !strings.Contains(output, "source-err") {
		t.Fatalf("failure diagnostics missing: %s", output)
	}
}

func TestSilentFactRemainsAvailableAndDisplayed(t *testing.T) {
	handler := &fakeHandler{installExec: ExecResult{Stdout: "computed"}}
	consumer := &fakeHandler{}
	opts := baseOpts(map[string]Handler{"fact": handler, "widget": consumer})
	opts.Apply = true
	var displayed map[string]any
	opts.OnFactsGathered = func(facts map[string]any) { displayed = facts }
	leaves := []tasks.Leaf{{
		Module: "fact", ID: "source",
		Item: map[string]any{
			"name": "quiet_fact", "value": "${{ stdout }}",
			"shell": map[string]any{"command": "example", "silent": true},
		},
	}, {
		Module: "widget", Item: map[string]any{"value": "${{ facts.quiet_fact }}", "captured": "${{ source.stdout }}"},
	}}
	results, stopped, err := Run(leaves, opts)
	if err != nil || stopped || len(results) != 2 {
		t.Fatalf("results=%#v stopped=%v err=%v", results, stopped, err)
	}
	if displayed["quiet_fact"] != "computed" {
		t.Fatalf("silent fact missing from displayed facts: %#v", displayed)
	}
	if consumer.seenItem["value"] != "computed" || consumer.seenItem["captured"] != "computed" {
		t.Fatalf("fact or registered output was lost: %#v", consumer.seenItem)
	}
}
