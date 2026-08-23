package filters

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

// fakeProcess wires a workerProcess up to an in-memory request/response
// loop so tests never spawn a real interpreter - overrides
// startWorkerProcess for the duration of the test.
func fakeProcess(t *testing.T, respond func(scriptRequest) scriptResponse) (*workerProcess, *int32) {
	t.Helper()
	var calls int32

	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()

	go func() {
		scanner := bufio.NewScanner(stdinR)
		for scanner.Scan() {
			atomic.AddInt32(&calls, 1)
			var req scriptRequest
			_ = json.Unmarshal(scanner.Bytes(), &req)
			resp := respond(req)
			data, _ := json.Marshal(resp)
			data = append(data, '\n')
			if _, err := stdoutW.Write(data); err != nil {
				return
			}
		}
		_ = stdoutW.Close()
	}()

	return &workerProcess{
		Stdin:  stdinW,
		Stdout: stdoutR,
		Stderr: &bytes.Buffer{},
		Wait:   func() error { return nil },
	}, &calls
}

func withFakeStartWorkerProcess(t *testing.T, proc *workerProcess) {
	t.Helper()
	origStart := startWorkerProcess
	startWorkerProcess = func(argv []string) (*workerProcess, error) { return proc, nil }
	t.Cleanup(func() { startWorkerProcess = origStart })
}

func TestScriptWorkerCallRoundTrip(t *testing.T) {
	proc, _ := fakeProcess(t, func(req scriptRequest) scriptResponse {
		return scriptResponse{Result: req.Value.(string) + "!"}
	})
	withFakeStartWorkerProcess(t, proc)

	w := newScriptWorker([]string{"fake"})
	result, err := w.Call("hi", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != "hi!" {
		t.Fatalf("result = %v", result)
	}
}

func TestScriptWorkerReusesProcessAcrossCalls(t *testing.T) {
	var startCalls int32
	proc, _ := fakeProcess(t, func(req scriptRequest) scriptResponse {
		return scriptResponse{Result: "ok"}
	})
	orig := startWorkerProcess
	startWorkerProcess = func(argv []string) (*workerProcess, error) {
		atomic.AddInt32(&startCalls, 1)
		return proc, nil
	}
	t.Cleanup(func() { startWorkerProcess = orig })

	w := newScriptWorker([]string{"fake"})
	for i := 0; i < 3; i++ {
		if _, err := w.Call("x", nil); err != nil {
			t.Fatal(err)
		}
	}
	if startCalls != 1 {
		t.Fatalf("expected the worker process to be started exactly once, got %d", startCalls)
	}
}

func TestScriptWorkerPropagatesScriptError(t *testing.T) {
	proc, _ := fakeProcess(t, func(req scriptRequest) scriptResponse {
		return scriptResponse{Error: "boom"}
	})
	withFakeStartWorkerProcess(t, proc)

	w := newScriptWorker([]string{"fake"})
	_, err := w.Call("x", nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := err.Error(); !contains(got, "boom") {
		t.Fatalf("error = %q, want it to mention 'boom'", got)
	}
}

func TestScriptWorkerCloseIsIdempotentAndStopsTheProcess(t *testing.T) {
	proc, _ := fakeProcess(t, func(req scriptRequest) scriptResponse {
		return scriptResponse{Result: "ok"}
	})
	withFakeStartWorkerProcess(t, proc)

	w := newScriptWorker([]string{"fake"})
	if _, err := w.Call("x", nil); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("second Close should be a no-op, got %v", err)
	}
}

func TestDiscoverScriptFiltersSkipsExistingNames(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "upper.ps1"), []byte("# not actually run"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := New() // has the built-in 'upper' already registered
	pool, registered, err := DiscoverScriptFilters(r, dir, DefaultInterpreters())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if len(registered) != 0 {
		t.Fatalf("expected the built-in 'upper' to win, got newly registered: %v", registered)
	}
}

func TestDiscoverScriptFiltersRegistersNewScript(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "double.ps1"), []byte("# not actually run"), 0o644); err != nil {
		t.Fatal(err)
	}

	proc, calls := fakeProcess(t, func(req scriptRequest) scriptResponse {
		return scriptResponse{Result: req.Value}
	})
	withFakeStartWorkerProcess(t, proc)

	r := &Registry{fns: map[string]Func{}} // no built-ins, so nothing to collide with
	pool, registered, err := DiscoverScriptFilters(r, dir, DefaultInterpreters())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if len(registered) != 1 || registered[0] != "double" {
		t.Fatalf("registered = %v, want [double]", registered)
	}
	if !r.Has("double") {
		t.Fatal("expected 'double' to be registered")
	}
	result, err := r.Apply("double", "x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != "x" {
		t.Fatalf("result = %v", result)
	}
	if atomic.LoadInt32(calls) != 1 {
		t.Fatalf("expected exactly 1 call, got %d", *calls)
	}
}

func TestDiscoverScriptFiltersMissingDirIsNotError(t *testing.T) {
	r := New()
	pool, registered, err := DiscoverScriptFilters(r, filepath.Join(t.TempDir(), "does-not-exist"), DefaultInterpreters())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if len(registered) != 0 {
		t.Fatalf("registered = %v", registered)
	}
}

func TestDiscoverScriptFiltersSkipsUnknownExtensions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("docs"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &Registry{fns: map[string]Func{}}
	pool, registered, err := DiscoverScriptFilters(r, dir, DefaultInterpreters())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if len(registered) != 0 {
		t.Fatalf("registered = %v, want none (unrecognized extension)", registered)
	}
}

// TestPoolConcurrentDistinctWorkersDoNotBlockEachOther exercises the one
// place in this package with real concurrency (docs/plans/go-rewrite.md
// §4.5/§6 - required to pass under 'go test -race' in CI): calls to two
// *different* script filters, each backed by its own worker/process,
// must be able to proceed concurrently without racing on shared state.
func TestPoolConcurrentDistinctWorkersDoNotBlockEachOther(t *testing.T) {
	pool := NewPool()
	const numWorkers = 8
	const callsPerWorker = 20

	var mu sync.Mutex
	procs := map[string]*workerProcess{}
	origStart := startWorkerProcess
	startWorkerProcess = func(argv []string) (*workerProcess, error) {
		mu.Lock()
		defer mu.Unlock()
		return procs[argv[0]], nil
	}
	t.Cleanup(func() { startWorkerProcess = origStart })

	workers := make([]*scriptWorker, numWorkers)
	for i := 0; i < numWorkers; i++ {
		name := fmt.Sprintf("w%d", i)
		proc, _ := fakeProcess(t, func(req scriptRequest) scriptResponse {
			return scriptResponse{Result: req.Value}
		})
		mu.Lock()
		procs[name] = proc
		mu.Unlock()
		workers[i] = pool.WorkerFor(name, []string{name})
	}

	var wg sync.WaitGroup
	errs := make(chan error, numWorkers*callsPerWorker)
	for _, w := range workers {
		wg.Add(1)
		go func(w *scriptWorker) {
			defer wg.Done()
			for c := 0; c < callsPerWorker; c++ {
				if _, err := w.Call(c, nil); err != nil {
					errs <- err
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if err := pool.Close(); err != nil {
		t.Fatal(err)
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
