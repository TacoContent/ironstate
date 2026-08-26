package handlers

import (
	"testing"

	"github.com/TacoContent/ironstate/internal/engine"
)

func TestAsyncHandlerRequiresID(t *testing.T) {
	h := asyncHandler{}
	item := map[string]any{"tasks": []any{map[string]any{"log": map[string]any{"message": "hi"}}}}
	if _, err := h.Install(item, "", testCtx()); err == nil {
		t.Fatal("expected error for missing 'id'")
	}
}

func TestAsyncHandlerRequiresTasks(t *testing.T) {
	h := asyncHandler{}
	item := map[string]any{"id": "job1"}
	if _, err := h.Install(item, "", testCtx()); err == nil {
		t.Fatal("expected error for missing/empty 'tasks'")
	}
}

func TestAsyncHandlerRunsInBackgroundAndWaitForObservesCompletion(t *testing.T) {
	t.Cleanup(engine.ResetAsyncJobs)

	async := asyncHandler{}
	item := map[string]any{
		"id":    "job1",
		"tasks": []any{map[string]any{"log": map[string]any{"message": "hi"}}},
	}
	exec, err := async.Install(item, "", testCtx())
	if err != nil {
		t.Fatal(err)
	}
	if exec.RC != 0 {
		t.Fatalf("async start rc=%d, want 0", exec.RC)
	}

	wait := waitForHandler{}
	waitItem := map[string]any{
		"for":      []any{"job1"},
		"timeout":  5.0,
		"interval": 0.05,
	}
	waitExec, err := wait.Install(waitItem, "", testCtx())
	if err != nil {
		t.Fatal(err)
	}
	if waitExec.RC != 0 {
		t.Fatalf("wait_for rc=%d, want 0; stderr=%s", waitExec.RC, waitExec.Stderr)
	}
}

func TestWaitForTimesOutWhenJobNeverStarted(t *testing.T) {
	t.Cleanup(engine.ResetAsyncJobs)

	wait := waitForHandler{}
	item := map[string]any{
		"for":      []any{"never-started"},
		"timeout":  0.2,
		"interval": 0.05,
	}
	exec, err := wait.Install(item, "", testCtx())
	if err != nil {
		t.Fatal(err)
	}
	if exec.RC != 1 {
		t.Fatalf("rc=%d, want 1 (timeout)", exec.RC)
	}
}

func TestWaitForConditionTrueImmediately(t *testing.T) {
	wait := waitForHandler{}
	item := map[string]any{"condition": "1 == 1", "timeout": 1.0, "interval": 0.05}
	exec, err := wait.Install(item, "", testCtx())
	if err != nil {
		t.Fatal(err)
	}
	if exec.RC != 0 {
		t.Fatalf("rc=%d, want 0", exec.RC)
	}
}

func TestWaitForRequiresForOrCondition(t *testing.T) {
	wait := waitForHandler{}
	if _, err := wait.Install(map[string]any{}, "", testCtx()); err == nil {
		t.Fatal("expected error when neither 'for' nor 'condition' is given")
	}
}

func TestAsyncHandlerFailedNestedTaskSurfacesAsWaitForFailure(t *testing.T) {
	t.Cleanup(engine.ResetAsyncJobs)

	async := asyncHandler{}
	item := map[string]any{
		"id":    "job2",
		"tasks": []any{map[string]any{"fail": map[string]any{"message": "boom"}}},
	}
	if _, err := async.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}

	wait := waitForHandler{}
	waitItem := map[string]any{"for": []any{"job2"}, "timeout": 5.0, "interval": 0.05}
	exec, err := wait.Install(waitItem, "", testCtx())
	if err != nil {
		t.Fatal(err)
	}
	if exec.RC != 1 {
		t.Fatalf("rc=%d, want 1 (nested task failed)", exec.RC)
	}
}
