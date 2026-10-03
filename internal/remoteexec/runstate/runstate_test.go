package runstate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireIsExclusiveAndReleasable(t *testing.T) {
	dir := t.TempDir()
	first, err := Acquire(dir, "run-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(dir, "run-b"); !errors.Is(err, ErrLocked) {
		t.Fatalf("second Acquire err = %v, want ErrLocked", err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	again, err := Acquire(dir, "run-c")
	if err != nil {
		t.Fatalf("Acquire after release: %v", err)
	}
	_ = again.Release()
}

func TestOpenRunLogPrunesOldRuns(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)
	for i := 0; i < KeepRuns+5; i++ {
		runDir := filepath.Join(dir, "runs", fmt.Sprintf("old-%02d", i))
		if err := os.MkdirAll(runDir, 0o700); err != nil {
			t.Fatal(err)
		}
		stamp := base.Add(time.Duration(i) * time.Second)
		if err := os.Chtimes(runDir, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	f, err := OpenRunLog(dir, "newest")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	entries, err := os.ReadDir(filepath.Join(dir, "runs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != KeepRuns {
		t.Fatalf("kept %d runs, want %d", len(entries), KeepRuns)
	}
	if _, err := os.Stat(filepath.Join(dir, "runs", "old-00")); err == nil {
		t.Error("oldest run was not pruned")
	}
}

func TestOpenRunLogRejectsUnsafeRunID(t *testing.T) {
	for _, id := range []string{"", "../x", "a/b", `a\b`} {
		if _, err := OpenRunLog(t.TempDir(), id); err == nil {
			t.Errorf("OpenRunLog(%q) accepted", id)
		}
	}
}
