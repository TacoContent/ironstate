package engine

// Package-level async-job registry backing the 'async'/'wait_for' modules
// (internal/handlers/async.go, waitfor.go). Handler.Install/Uninstall only
// receive this leaf's own Item/name/Context - there is no other channel
// back to an 'async' leaf dispatched earlier in the same run, so the
// registry lives here (process-global, like Warn/Info/LookPath) rather
// than on State, which is rebuilt per RunLeaves call and not safe for
// concurrent goroutine access anyway.

import "sync"

// AsyncJob tracks one in-flight or completed 'async' task group, keyed by
// the 'id' the user gave that async leaf.
type AsyncJob struct {
	mu      sync.Mutex
	done    bool
	failed  bool
	err     error
	results []Result
}

// Finish records the outcome of the background run and marks the job
// done - called exactly once, from the goroutine handlers.asyncHandler
// starts.
func (j *AsyncJob) Finish(results []Result, failed bool, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.results = results
	j.failed = failed
	j.err = err
	j.done = true
}

// Snapshot returns the job's current state - safe to call while the
// background goroutine may still be running.
func (j *AsyncJob) Snapshot() (done, failed bool, results []Result, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.done, j.failed, j.results, j.err
}

var (
	asyncJobsMu sync.Mutex
	asyncJobs   = map[string]*AsyncJob{}
)

// StartAsyncJob registers a new, not-yet-finished job under id (last
// write wins on a reused id, matching every other 'id'-keyed registry in
// this package) and returns it so the caller can populate it once its
// goroutine finishes.
func StartAsyncJob(id string) *AsyncJob {
	job := &AsyncJob{}
	asyncJobsMu.Lock()
	asyncJobs[id] = job
	asyncJobsMu.Unlock()
	return job
}

// LookupAsyncJob returns the job registered under id, if any.
func LookupAsyncJob(id string) (*AsyncJob, bool) {
	asyncJobsMu.Lock()
	defer asyncJobsMu.Unlock()
	job, ok := asyncJobs[id]
	return job, ok
}

// ResetAsyncJobs clears the global async-job registry. The registry is
// process-global rather than per-run, so tests that start async jobs must
// call this in cleanup to avoid leaking state into later tests.
func ResetAsyncJobs() {
	asyncJobsMu.Lock()
	defer asyncJobsMu.Unlock()
	asyncJobs = map[string]*AsyncJob{}
}
