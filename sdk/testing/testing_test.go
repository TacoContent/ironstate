package testing

import (
	"os"
	"path/filepath"
	"testing"
)

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

func TestProfileWritesHeapProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "heap.pprof")
	if err := Profile(path, "heap", func() error { return nil }); err != nil {
		t.Fatalf("Profile returned error: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat profile: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("heap profile is empty")
	}
}

func TestProfileRejectsUnsupportedKind(t *testing.T) {
	if err := Profile(filepath.Join(t.TempDir(), "profile"), "trace", func() error { return nil }); err == nil {
		t.Fatal("Profile accepted unsupported kind")
	}
}
