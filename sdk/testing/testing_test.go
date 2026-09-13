package testing

import "testing"

func TestRecordingRunnerCopiesArguments(t *testing.T) {
	runner := &RecordingRunner{}
	args := []string{"install", "package"}
	if _, err := runner.Run("tool", args); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	args[1] = "changed"

	if got, want := runner.Calls[0].Args[1], "package"; got != want {
		t.Fatalf("recorded argument = %q, want %q", got, want)
	}
}
