package filters

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
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
	if err := os.WriteFile(filepath.Join(dir, "upper.ps1"), []byte("# not actually run"), 0o600); err != nil {
		t.Fatal(err)
	}

	r := New() // has the built-in 'upper' already registered
	pool, registered, err := DiscoverScriptFilters(r, dir, DefaultInterpreters())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pool.Close() }()

	if len(registered) != 0 {
		t.Fatalf("expected the built-in 'upper' to win, got newly registered: %v", registered)
	}
}

func TestDiscoverScriptFiltersRegistersNewScript(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "double.ps1"), []byte("# not actually run"), 0o600); err != nil {
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
	defer func() { _ = pool.Close() }()

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
	defer func() { _ = pool.Close() }()
	if len(registered) != 0 {
		t.Fatalf("registered = %v", registered)
	}
}

func TestDiscoverScriptFiltersSkipsUnknownExtensions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("docs"), 0o600); err != nil {
		t.Fatal(err)
	}

	r := &Registry{fns: map[string]Func{}}
	pool, registered, err := DiscoverScriptFilters(r, dir, DefaultInterpreters())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pool.Close() }()
	if len(registered) != 0 {
		t.Fatalf("registered = %v, want none (unrecognized extension)", registered)
	}
}

// TestDiscoverScriptFiltersRunsRealBashFilters exercises the actual
// embed/shim.sh against the real playbooks/sample/filters/*.sh files (no
// fakeProcess) - the only test in this package that runs a real
// interpreter end-to-end, since fakeProcess-based tests never touch the
// shim scripts themselves. Skips if bash or jq aren't on PATH (shim.sh
// requires both).
func TestDiscoverScriptFiltersRunsRealBashFilters(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not on PATH")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not on PATH")
	}

	dir := filepath.Join("..", "..", "playbooks", "sample", "filters")
	r := &Registry{fns: map[string]Func{}}
	pool, registered, err := DiscoverScriptFilters(r, dir, DefaultInterpreters())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pool.Close() }()

	if !r.Has("leet") || !r.Has("buffalo") {
		t.Fatalf("registered = %v, want leet and buffalo among them", registered)
	}

	if got, err := r.Apply("leet", "leet speak", nil); err != nil {
		t.Fatal(err)
	} else if got != "l337 5p34k " {
		t.Fatalf("leet filter result = %q", got)
	}

	if got, err := r.Apply("buffalo", "Hello 123 world", nil); err != nil {
		t.Fatal(err)
	} else if got != "Buffalo 123 buffalo " {
		t.Fatalf("buffalo filter result = %q", got)
	}
}

// TestDiscoverScriptFiltersRunsRealZshFilters mirrors the bash test above
// for '.zsh', using a self-contained temp script (no repo fixture) since
// no playbooks/sample/filters/*.zsh exists. Skips if zsh or jq aren't on
// PATH (shim.zsh requires both).
func TestDiscoverScriptFiltersRunsRealZshFilters(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not on PATH")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not on PATH")
	}
	// A "zsh" on PATH isn't necessarily a fully working one (e.g. a
	// custom WSL-forwarding wrapper that can't see jq, or can't fork at
	// all in a constrained environment) - preflight it with a trivial
	// command and skip rather than fail on an environment-specific
	// problem unrelated to shim.zsh itself.
	if err := exec.Command("zsh", "-c", "command -v jq").Run(); err != nil {
		t.Skipf("zsh on PATH can't run a basic 'command -v jq' (%v) - skipping", err)
	}

	dir := t.TempDir()
	script := "if [ -z \"$1\" ]; then\n\techo \"\"\n\texit 0\nfi\necho -n \"$1\" | tr '[:lower:]' '[:upper:]'\n"
	if err := os.WriteFile(filepath.Join(dir, "shout.zsh"), []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}

	r := &Registry{fns: map[string]Func{}}
	pool, registered, err := DiscoverScriptFilters(r, dir, DefaultInterpreters())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pool.Close() }()

	if len(registered) != 1 || registered[0] != "shout" {
		t.Fatalf("registered = %v, want [shout]", registered)
	}
	if got, err := r.Apply("shout", "hi there", nil); err != nil {
		t.Fatal(err)
	} else if got != "HI THERE" {
		t.Fatalf("shout filter result = %q", got)
	}
}

// TestDiscoverScriptFiltersRunsRealFishFilters mirrors the bash test
// above for '.fish'. Skips if fish or jq aren't on PATH (shim.fish
// requires both).
func TestDiscoverScriptFiltersRunsRealFishFilters(t *testing.T) {
	if _, err := exec.LookPath("fish"); err != nil {
		t.Skip("fish not on PATH")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not on PATH")
	}

	dir := t.TempDir()
	script := "if test -z \"$argv[1]\"\n\techo \"\"\n\texit 0\nend\necho -n (string upper $argv[1])\n"
	if err := os.WriteFile(filepath.Join(dir, "shout.fish"), []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}

	r := &Registry{fns: map[string]Func{}}
	pool, registered, err := DiscoverScriptFilters(r, dir, DefaultInterpreters())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pool.Close() }()

	if len(registered) != 1 || registered[0] != "shout" {
		t.Fatalf("registered = %v, want [shout]", registered)
	}
	if got, err := r.Apply("shout", "hi there", nil); err != nil {
		t.Fatal(err)
	} else if got != "HI THERE" {
		t.Fatalf("shout filter result = %q", got)
	}
}

// TestDiscoverScriptFiltersRunsRealNuFilters mirrors the bash test above
// for '.nu' - the one script-filter language that goes through
// oneshotFilterFunc instead of a persistent worker (see embed/shim.nu).
// No jq needed (nu has native JSON support); skips if nu isn't on PATH.
func TestDiscoverScriptFiltersRunsRealNuFilters(t *testing.T) {
	if _, err := exec.LookPath("nu"); err != nil {
		t.Skip("nu not on PATH")
	}

	dir := t.TempDir()
	script := "def main [value, ...args] {\n  print ($value | str upcase)\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "shout.nu"), []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}

	r := &Registry{fns: map[string]Func{}}
	pool, registered, err := DiscoverScriptFilters(r, dir, DefaultInterpreters())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pool.Close() }()

	if len(registered) != 1 || registered[0] != "shout" {
		t.Fatalf("registered = %v, want [shout]", registered)
	}
	if got, err := r.Apply("shout", "hi there", nil); err != nil {
		t.Fatal(err)
	} else if got != "HI THERE" {
		t.Fatalf("shout filter result = %q", got)
	}
	// '.nu' is spawned fresh per call (see oneshotExtensions), never
	// pooled as a persistent worker.
	if _, ok := pool.workers["shout"]; ok {
		t.Fatal("expected no persistent worker registered for a '.nu' filter")
	}
}

// TestDiscoverScriptFiltersPriorityOrder verifies the user-requested
// tie-break order (powershell -> bash -> zsh -> fish -> nushell) when a
// filters directory has more than one script implementing the same
// name - entirely via fakes, so it runs without any real interpreter on
// PATH.
func TestDiscoverScriptFiltersPriorityOrder(t *testing.T) {
	interpreters := map[string][]string{
		".ps1":  {"pwsh"},
		".sh":   {"bash"},
		".zsh":  {"zsh"},
		".fish": {"fish"},
		".nu":   {"nu", "--stdin"},
	}

	proc, _ := fakeProcess(t, func(req scriptRequest) scriptResponse {
		return scriptResponse{Result: "ok"}
	})
	withFakeStartWorkerProcess(t, proc)

	origOneshot := runOneshotProcess
	runOneshotProcess = func(argv []string, stdin []byte) ([]byte, []byte, error) {
		return []byte(`{"result":"ok"}`), nil, nil
	}
	t.Cleanup(func() { runOneshotProcess = origOneshot })

	tests := []struct {
		name      string
		exts      []string
		wantArgv0 string
		oneshot   bool
	}{
		{"ps1 wins over everything", []string{".ps1", ".sh", ".zsh", ".fish", ".nu"}, "pwsh", false},
		{"sh wins over zsh, fish, nu", []string{".sh", ".zsh", ".fish", ".nu"}, "bash", false},
		{"zsh wins over fish, nu", []string{".zsh", ".fish", ".nu"}, "zsh", false},
		{"fish wins over nu", []string{".fish", ".nu"}, "fish", false},
		{"nu alone uses the oneshot path", []string{".nu"}, "nu", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, ext := range tc.exts {
				if err := os.WriteFile(filepath.Join(dir, "multi"+ext), []byte("# not actually run"), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			r := &Registry{fns: map[string]Func{}}
			pool, registered, err := DiscoverScriptFilters(r, dir, interpreters)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = pool.Close() }()

			if len(registered) != 1 || registered[0] != "multi" {
				t.Fatalf("registered = %v, want exactly [multi]", registered)
			}

			if tc.oneshot {
				if _, ok := pool.workers["multi"]; ok {
					t.Fatal("expected no persistent worker for the oneshot (.nu) winner")
				}
				return
			}
			worker, ok := pool.workers["multi"]
			if !ok {
				t.Fatalf("expected a persistent worker for 'multi', want interpreter %q", tc.wantArgv0)
			}
			if len(worker.argv) == 0 || worker.argv[0] != tc.wantArgv0 {
				t.Fatalf("argv = %v, want interpreter %q to win", worker.argv, tc.wantArgv0)
			}
		})
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
