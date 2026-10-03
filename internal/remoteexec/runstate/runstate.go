// Package runstate holds the target-side state of remote-apply runs: the
// single-writer lock and per-run event logs (docs/plans/remote-apply.md §10).
package runstate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// KeepRuns is how many run logs are retained.
const KeepRuns = 20

// ErrLocked is returned when another run holds the lock.
var ErrLocked = errors.New("another ironstate run is in progress")

var runIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// DefaultDir is the per-user state dir (UserCacheDir/ironstate).
func DefaultDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "ironstate"), nil
}

// Lock is a held apply lock; Release frees it.
type Lock struct {
	f *os.File
}

// Acquire takes the non-blocking exclusive apply lock in dir and records
// runID in it. A held lock yields an error wrapping ErrLocked that names
// the holding run when readable.
func Acquire(dir, runID string) (*Lock, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "apply.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // fixed name inside the user's own state dir
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		_ = f.Close()
		holder := ""
		if data, readErr := os.ReadFile(path); readErr == nil { //nolint:gosec // same path as above
			holder = strings.TrimSpace(string(data))
		}
		if holder != "" {
			return nil, fmt.Errorf("%w (run %s)", ErrLocked, holder)
		}
		return nil, ErrLocked
	}
	if err := f.Truncate(0); err == nil {
		_, _ = f.WriteAt([]byte(runID+"\n"), 0)
	}
	return &Lock{f: f}, nil
}

// Release frees the lock.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = unlockFile(l.f)
	err := l.f.Close()
	l.f = nil
	return err
}

// OpenRunLog creates dir/runs/<runID>/events.ndjson (0600) and prunes all
// but the newest KeepRuns run directories.
func OpenRunLog(dir, runID string) (*os.File, error) {
	if !runIDPattern.MatchString(runID) {
		return nil, fmt.Errorf("invalid run id %q", runID)
	}
	runsDir := filepath.Join(dir, "runs")
	runDir := filepath.Join(runsDir, runID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return nil, err
	}
	// Append: the same machine can be targeted twice in one run (aliases);
	// the apply lock keeps those passes sequential.
	f, err := os.OpenFile(filepath.Join(runDir, "events.ndjson"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // run id validated above
	if err != nil {
		return nil, err
	}
	prune(runsDir, runID)
	return f, nil
}

func prune(runsDir, keep string) {
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		return
	}
	type run struct {
		name string
		mod  int64
	}
	var runs []run
	for _, e := range entries {
		if !e.IsDir() || e.Name() == keep {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		runs = append(runs, run{e.Name(), info.ModTime().UnixNano()})
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].mod > runs[j].mod })
	for i, r := range runs {
		if i >= KeepRuns-1 {
			_ = os.RemoveAll(filepath.Join(runsDir, r.name))
		}
	}
}
