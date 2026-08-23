package filters

// script.go implements the external-script filter adapter
// (docs/plans/go-rewrite.md §4.5): a discovered script file is called
// over a small, stable JSON protocol via a persistent per-file worker
// process (pool.go), so an existing modules/Filters/*.ps1 file keeps
// working completely unmodified - the shim (embed/shim.ps1) is the only
// new artifact, generic and parameterized by target script path, not one
// per filter.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// DefaultInterpreters maps a script file extension to the interpreter
// argv prefix used to run its shim - the "eventually support other
// script types" hook (docs/plans/go-rewrite.md §4.5), wired up now even
// though only PowerShell ships a shim today (embed/shim.ps1).
func DefaultInterpreters() map[string][]string {
	return map[string][]string{
		".ps1": {"pwsh", "-NoProfile", "-File"},
	}
}

// scriptRequest/scriptResponse are the JSON-over-stdio protocol every
// script filter shim speaks - one line of request in, one line of
// response out, per call.
type scriptRequest struct {
	Value any   `json:"value"`
	Args  []any `json:"args"`
}

type scriptResponse struct {
	Result any    `json:"result"`
	Error  string `json:"error,omitempty"`
}

// shimExtractor resolves the on-disk path of the shim script for ext,
// extracting an embedded shim to a temp file on first use. Only '.ps1'
// has a real shim today (embed/shim.ps1) - any other extension is a
// documented gap (see DiscoverScriptFilters), not silently ignored.
func shimPathFor(ext string) (string, error) {
	switch ext {
	case ".ps1":
		return extractedPS1ShimPath()
	default:
		return "", fmt.Errorf("no script-filter shim available for extension %q (only .ps1 ships one today)", ext)
	}
}

var (
	ps1ShimOnce sync.Once
	ps1ShimPath string
	ps1ShimErr  error
)

func extractedPS1ShimPath() (string, error) {
	ps1ShimOnce.Do(func() {
		f, err := os.CreateTemp("", "ironstate-filter-shim-*.ps1")
		if err != nil {
			ps1ShimErr = err
			return
		}
		if _, err := f.Write(shimPS1); err != nil {
			_ = f.Close()
			ps1ShimErr = err
			return
		}
		ps1ShimErr = f.Close()
		ps1ShimPath = f.Name()
	})
	return ps1ShimPath, ps1ShimErr
}

// workerProcess is the thin process-facing surface a scriptWorker needs -
// overridable via startWorkerProcess so tests never spawn a real
// interpreter.
type workerProcess struct {
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
	Stderr *bytes.Buffer
	Wait   func() error
}

var startWorkerProcess = func(argv []string) (*workerProcess, error) {
	cmd := exec.Command(argv[0], argv[1:]...) //nolint:gosec // argv is built from configured interpreters + a discovered script path, same trust boundary as the rest of this tool
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &workerProcess{Stdin: stdin, Stdout: stdout, Stderr: &stderr, Wait: cmd.Wait}, nil
}

// scriptWorker is one persistent, lazily-started interpreter subprocess
// backing a single script filter - opened on first Call and kept warm
// for the process lifetime (docs/plans/go-rewrite.md §4.5's "process-per-
// call has real overhead" mitigation), rather than spawned fresh per call.
type scriptWorker struct {
	argv []string

	mu     sync.Mutex
	proc   *workerProcess
	reader *bufio.Reader
}

func newScriptWorker(argv []string) *scriptWorker {
	return &scriptWorker{argv: argv}
}

func (w *scriptWorker) ensureStarted() error {
	if w.proc != nil {
		return nil
	}
	proc, err := startWorkerProcess(w.argv)
	if err != nil {
		return fmt.Errorf("starting script filter %v: %w", w.argv, err)
	}
	w.proc = proc
	w.reader = bufio.NewReader(proc.Stdout)
	return nil
}

// Call sends one request and waits for its matching response - serialized
// per worker (a single interpreter process handles one call at a time),
// but multiple *different* script filters each get their own worker/
// process, so calls to distinct filters proceed concurrently.
func (w *scriptWorker) Call(value any, args []any) (any, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.ensureStarted(); err != nil {
		return nil, err
	}
	if args == nil {
		args = []any{}
	}

	data, err := json.Marshal(scriptRequest{Value: value, Args: args})
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	if _, err := w.proc.Stdin.Write(data); err != nil {
		return nil, w.wrapProcessErr(err)
	}

	line, err := w.reader.ReadString('\n')
	if err != nil {
		return nil, w.wrapProcessErr(err)
	}

	var resp scriptResponse
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(line)), &resp); jsonErr != nil {
		return nil, fmt.Errorf("script filter %v: invalid response %q: %w", w.argv, line, jsonErr)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("script filter %v: %s", w.argv, resp.Error)
	}
	return resp.Result, nil
}

func (w *scriptWorker) wrapProcessErr(err error) error {
	stderr := ""
	if w.proc.Stderr != nil {
		stderr = strings.TrimSpace(w.proc.Stderr.String())
	}
	if stderr != "" {
		return fmt.Errorf("script filter %v: %w (stderr: %s)", w.argv, err, stderr)
	}
	return fmt.Errorf("script filter %v: %w", w.argv, err)
}

// Close terminates the worker's subprocess, if one was ever started.
func (w *scriptWorker) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.proc == nil {
		return nil
	}
	_ = w.proc.Stdin.Close()
	err := w.proc.Wait()
	w.proc = nil
	return err
}

func workerFilterFunc(w *scriptWorker) Func {
	return func(value any, args []any) (any, error) {
		return w.Call(value, args)
	}
}

// DiscoverScriptFilters scans dir for filter script files (one per
// filter, named '<name><ext>') and registers each one into r under its
// base name - but only if r doesn't already have a filter of that name
// (a Go built-in, or one discovered earlier, always wins; today's
// modules/Filters/*.ps1 files continue to work unmodified, and a net-new
// custom filter can be dropped in the same directory without recompiling
// the binary). interpreters maps a recognized extension to its
// interpreter argv prefix (DefaultInterpreters for the shipped default).
//
// A missing dir is not an error (nothing to discover); an extension with
// no known interpreter, or no known shim (shimPathFor), is skipped
// (documented gap - only PowerShell ships one today), not a hard failure,
// since a plain "not a recognized filter script" file living alongside
// real ones is expected (e.g. modules/Filters/README.md).
//
// Returns a Pool the caller must Close() when done (usually at process
// exit), and the names actually registered (for 'filters list'/'doctor'
// reporting).
func DiscoverScriptFilters(r *Registry, dir string, interpreters map[string][]string) (*Pool, []string, error) {
	pool := NewPool()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return pool, nil, nil
		}
		return pool, nil, err
	}

	var registered []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		name := strings.TrimSuffix(entry.Name(), ext)
		if name == "" || r.Has(name) {
			continue
		}
		argvPrefix, ok := interpreters[ext]
		if !ok {
			continue
		}
		shimPath, err := shimPathFor(ext)
		if err != nil {
			continue
		}

		scriptPath := filepath.Join(dir, entry.Name())
		argv := make([]string, 0, len(argvPrefix)+2)
		argv = append(argv, argvPrefix...)
		argv = append(argv, shimPath, scriptPath)

		worker := pool.WorkerFor(name, argv)
		r.Register(name, workerFilterFunc(worker))
		registered = append(registered, name)
	}
	return pool, registered, nil
}
