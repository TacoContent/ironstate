package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/internal/filters"
	"github.com/TacoContent/ironstate/internal/tasks"
)

func envSource(source map[string]any) map[string]any {
	return map[string]any{"valueFrom": source}
}

func TestResolveEnvironmentPrimitivesAndFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "value.txt")
	if err := os.WriteFile(path, []byte("content\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := Context{Flat: map[string]any{"facts": map[string]any{"answer": 42}}, Filters: filters.New()}
	values := map[string]any{
		"STRING": "text", "BOOL": true, "NUMBER": 12,
		"EXPR": "${{ facts.answer }}", "FILE": envSource(map[string]any{"file": path}),
	}
	resolved, err := ResolveEnvironment(values, ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resolved["STRING"] != "text" || resolved["BOOL"] != true || resolved["NUMBER"] != 12 || resolved["EXPR"] != 42 || resolved["FILE"] != "content\r\n" {
		t.Fatalf("resolved = %#v", resolved)
	}
	if _, provider := values["FILE"].(map[string]any); !provider {
		t.Fatal("input source was mutated")
	}
}

func TestResolveEnvironmentTaskEvaluatesValueAfterShell(t *testing.T) {
	shell := &fakeHandler{installExec: ExecResult{Stdout: "  computed\n", Stderr: "diagnostic"}}
	ctx := Context{Flat: map[string]any{"vars": map[string]any{"command": "example"}, "stdout": "old"}, Filters: filters.New()}
	values := map[string]any{"VALUE": envSource(map[string]any{"task": map[string]any{
		"shell": map[string]any{"command": "${{ vars.command }}"},
		"value": "${{ stdout | trim }}",
	}})}
	resolved, err := ResolveEnvironment(values, ctx, shell, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resolved["VALUE"] != "computed" || shell.installCall != 1 || shell.seenItem["command"] != "example" {
		t.Fatalf("resolved=%#v shell=%#v", resolved, shell)
	}
	if ctx.Flat["stdout"] != "old" {
		t.Fatal("command result leaked into the caller's context")
	}
}

func TestEnvironmentFileSourceCachingAndRelativeExpressionPaths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "value.txt")
	if err := os.WriteFile(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := Context{Flat: map[string]any{"vars": map[string]any{"file": "value.txt"}}, Filters: filters.New(), BaseDir: dir}
	values := map[string]any{"VALUE": envSource(map[string]any{"file": "${{ vars.file }}"})}
	cache := map[string]any{}
	if _, err := ResolveEnvironment(values, ctx, nil, cache); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveEnvironment(values, ctx, nil, cache)
	if err != nil || resolved["VALUE"] != "first" {
		t.Fatalf("cached result=%#v err=%v", resolved, err)
	}
	resolved, err = ResolveEnvironment(values, ctx, nil, nil)
	if err != nil || resolved["VALUE"] != "second" {
		t.Fatalf("fresh result=%#v err=%v", resolved, err)
	}
}

func TestEnvironmentTaskScriptPathsAndPrimitiveResult(t *testing.T) {
	dir := t.TempDir()
	shell := &fakeHandler{}
	ctx := Context{
		Flat:    map[string]any{"vars": map[string]any{"script": "source.ps1"}},
		Filters: filters.New(), BaseDir: dir,
	}
	values := map[string]any{"VALUE": envSource(map[string]any{"task": map[string]any{
		"shell": map[string]any{"script": "${{ vars.script }}", "args": []any{"argument"}},
		"value": "${{ rc == 0 }}",
	}})}
	result, err := ResolveEnvironment(values, ctx, shell, nil)
	if err != nil || result["VALUE"] != true || shell.seenItem["script"] != filepath.Join(dir, "source.ps1") {
		t.Fatalf("result=%#v shell=%#v err=%v", result, shell.seenItem, err)
	}
}

func TestResolveEnvironmentRejectsInvalidSources(t *testing.T) {
	for name, raw := range map[string]any{
		"map":                      map[string]any{"value": "bad"},
		"unknown":                  envSource(map[string]any{"unknown": "bad"}),
		"multiple":                 envSource(map[string]any{"file": "x", "task": map[string]any{}}),
		"missing-file":             envSource(map[string]any{"file": filepath.Join(t.TempDir(), "missing")}),
		"invalid-file":             envSource(map[string]any{"file": true}),
		"invalid-task":             envSource(map[string]any{"task": "bad"}),
		"missing-value":            envSource(map[string]any{"task": map[string]any{"shell": map[string]any{"command": "x"}}}),
		"empty-shell":              envSource(map[string]any{"task": map[string]any{"shell": map[string]any{}, "value": "x"}}),
		"unknown-field":            envSource(map[string]any{"task": map[string]any{"value": "x", "other": true}}),
		"list":                     []any{"bad"},
		"map-expression":           "${{ vars.object }}",
		"missing-value-expression": "${{ vars.missing }}",
		"nul":                      "a\x00b",
	} {
		t.Run(name, func(t *testing.T) {
			ctx := Context{Flat: map[string]any{"vars": map[string]any{"object": map[string]any{"a": 1}}}, Filters: filters.New()}
			if _, err := ResolveEnvironment(map[string]any{"VALUE": raw}, ctx, &fakeHandler{}, nil); err == nil || !strings.Contains(err.Error(), "envs.VALUE") {
				t.Fatalf("error = %v, want an explicit variable-specific error", err)
			}
		})
	}
}

func TestRunLeavesEnvironmentSourcesAreCachedPerScopeAcrossPhases(t *testing.T) {
	const key = "IRONSTATE_SOURCE_CACHE_TEST"
	t.Setenv(key, "original")
	shell := &fakeHandler{installExec: ExecResult{Stdout: "source"}}
	handler := &fakeHandler{}
	opts := baseOpts(map[string]Handler{"shell": shell, "fact": handler, "widget": handler})
	opts.Apply = true
	scope := &tasks.EnvironmentScope{Values: map[string]any{key: envSource(map[string]any{"task": map[string]any{
		"shell": map[string]any{"command": "compute"}, "value": "${{ stdout }}",
	}})}}
	leaves := []tasks.Leaf{
		{Module: "fact", ScopeEnvs: []*tasks.EnvironmentScope{scope}, Item: map[string]any{"name": "first", "value": true}},
		{Module: "widget", ScopeEnvs: []*tasks.EnvironmentScope{scope}, Item: map[string]any{"value": "${{ envs.IRONSTATE_SOURCE_CACHE_TEST }}"}},
	}
	if _, stopped, err := Run(leaves, opts); err != nil || stopped {
		t.Fatalf("stopped=%v err=%v", stopped, err)
	}
	if shell.installCall != 1 || handler.seenItem["value"] != "source" || os.Getenv(key) != "original" {
		t.Fatalf("source calls=%d item=%#v env=%q", shell.installCall, handler.seenItem, os.Getenv(key))
	}
	if _, _, err := Run(leaves, opts); err != nil || shell.installCall != 2 {
		t.Fatalf("source cache leaked across runs: calls=%d err=%v", shell.installCall, err)
	}
}

func TestRunLeavesTaskSourcesRunPerLeafAndSkipWhenFalse(t *testing.T) {
	shell := &fakeHandler{installExec: ExecResult{Stdout: "source"}}
	opts := baseOpts(map[string]Handler{"shell": shell, "widget": &fakeHandler{}})
	values := map[string]any{"IRONSTATE_TASK_SOURCE_TEST": envSource(map[string]any{"task": map[string]any{
		"shell": map[string]any{"command": "compute"}, "value": "${{ stdout }}",
	}})}
	leaves := []tasks.Leaf{
		{Module: "widget", Envs: values, When: []any{false}},
		{Module: "widget", Envs: values},
		{Module: "widget", Envs: values},
	}
	if _, _, err := RunLeaves(leaves, opts, NewState()); err != nil || shell.installCall != 2 {
		t.Fatalf("calls=%d err=%v", shell.installCall, err)
	}
}

func TestEnvironmentScopeUsesDeclaringVarsAndCurrentFacts(t *testing.T) {
	handler := &fakeHandler{}
	opts := baseOpts(map[string]Handler{"widget": handler})
	opts.Apply = true
	scope := &tasks.EnvironmentScope{
		Context: map[string]any{
			"vars":  map[string]any{"own": "declared"},
			"facts": map[string]any{"changing": "old"},
		},
		Values: map[string]any{"IRONSTATE_DECLARING_SCOPE_TEST": envSource(map[string]any{"task": map[string]any{
			"value": "${{ own }}:${{ changing }}:${{ child_only | default('outer') }}",
		}})},
	}
	state := NewState()
	state.UserFacts["changing"] = "new"
	leaves := []tasks.Leaf{{
		Module: "widget", ScopeEnvs: []*tasks.EnvironmentScope{scope},
		PackageVars: map[string]any{"own": "inner", "child_only": "leaked"},
		Item:        map[string]any{"value": "${{ envs.IRONSTATE_DECLARING_SCOPE_TEST }}"},
	}}
	_, _, err := RunLeaves(leaves, opts, state)
	if err != nil || handler.seenItem["value"] != "declared:new:outer" {
		t.Fatalf("item=%#v err=%v", handler.seenItem, err)
	}
}

func TestRunLeavesEnvironmentFailureRestoresOuterLayers(t *testing.T) {
	const key = "IRONSTATE_SOURCE_FAILURE_TEST"
	t.Setenv(key, "original")
	shell := &fakeHandler{installExec: ExecResult{RC: 7, Stderr: "source failed"}}
	opts := baseOpts(map[string]Handler{"shell": shell, "widget": &fakeHandler{}})
	leaf := tasks.Leaf{
		Module: "widget", ScopeEnvs: []*tasks.EnvironmentScope{{Values: map[string]any{key: "outer"}}},
		Envs: map[string]any{key: envSource(map[string]any{"task": map[string]any{
			"shell": map[string]any{"command": "compute"}, "value": "${{ stdout }}",
		}})},
	}
	_, _, err := RunLeaves([]tasks.Leaf{leaf}, opts, NewState())
	if err == nil || !strings.Contains(err.Error(), "code 7") || os.Getenv(key) != "original" {
		t.Fatalf("err=%v env=%q", err, os.Getenv(key))
	}
}
