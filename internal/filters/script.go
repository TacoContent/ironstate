package filters

// script.go implements the external-script filter adapter
// (docs/plans/go-rewrite.md §4.5): a discovered script file is called
// over a small, stable JSON protocol, so an existing filters/*.ps1,
// *.sh, *.zsh, *.fish, or *.nu file keeps working completely unmodified
// - the shim (embed/shim.*) is the only new artifact per script
// language, generic and parameterized by target script path, not one per
// filter. Every shim but nu's runs as a persistent per-filter worker
// process kept warm across calls (pool.go); nu's shim is spawned fresh
// per call instead (see oneshotFilterFunc) since nu has no reliable way
// to read one request line, respond, then read a later one on the same
// still-open stdin pipe (see embed/shim.nu's own doc comment).

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// DefaultInterpreters maps a script file extension to the interpreter
// argv prefix used to run its shim - the "eventually support other
// script types" hook (docs/plans/go-rewrite.md §4.5). PowerShell, bash,
// zsh, fish, and nu each ship a shim today.
func DefaultInterpreters() map[string][]string {
	return map[string][]string{
		".ps1":  {"pwsh", "-NoProfile", "-File"},
		".sh":   {resolveBashExecutable()},
		".zsh":  {"zsh"},
		".fish": {"fish", "--no-config"},
		".nu":   {"nu", "--stdin", "--no-config-file"},
	}
}

// scriptFilterExtensionPriority is the tie-break order used when a
// filters directory has more than one script implementing the same
// filter name (e.g. both leet.ps1 and leet.sh) - PowerShell wins, then
// bash, zsh, fish, nushell (user-requested order). An extension outside
// this list (e.g. a custom one added via filters.interpreters config)
// still works, just always loses to any of these five for the same name.
var scriptFilterExtensionPriority = []string{".ps1", ".sh", ".zsh", ".fish", ".nu"}

func scriptFilterExtensionRank(ext string) int {
	for i, candidate := range scriptFilterExtensionPriority {
		if candidate == ext {
			return i
		}
	}
	return len(scriptFilterExtensionPriority)
}

// resolveBashExecutable picks the bash to run '.sh' filter shims with. On
// Windows, a bare "bash" on PATH very commonly resolves to Windows' own
// WSL launcher stub (System32\bash.exe, or its WindowsApps app-execution-
// alias twin) - that launcher re-parses argv for the Linux side and
// silently eats backslashes in Windows-style paths (like the ones
// os.CreateTemp/DiscoverScriptFilters produce), breaking the shim/script
// paths outright. Walk PATH ourselves and skip those two known-broken-
// for-this-purpose launchers, preferring a real Windows-path-aware bash
// (Git for Windows, MSYS2, Cygwin) if one is also on PATH; if PATH has
// nothing usable, fall back to a few well-known Git for Windows install
// locations (not on PATH by default on many machines); only as a last
// resort fall back to the bare "bash" name, so the resulting error is at
// least the normal "not found"/exec failure rather than a silently-
// swallowed one. Non-Windows platforms have no such launcher shim, so
// this is a no-op there.
func resolveBashExecutable() string {
	if runtime.GOOS != "windows" {
		return "bash"
	}
	if p := usableBashOnPath(); p != "" {
		return p
	}
	for _, candidate := range wellKnownWindowsBashLocations() {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() { //nolint:gosec // candidate is a fixed Git-for-Windows subpath under a well-known env var (ProgramFiles/LocalAppData), not user/network input
			return candidate
		}
	}
	return "bash"
}

func usableBashOnPath() string {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		candidate := filepath.Join(dir, "bash.exe")
		lower := strings.ToLower(candidate)
		if strings.HasSuffix(lower, `\system32\bash.exe`) || strings.Contains(lower, `\windowsapps\`) {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() { //nolint:gosec // candidate is PATH-dir + a fixed "bash.exe" suffix, not user/network input
			return candidate
		}
	}
	return ""
}

func wellKnownWindowsBashLocations() []string {
	var candidates []string
	if pf := os.Getenv("ProgramFiles"); pf != "" {
		candidates = append(candidates, filepath.Join(pf, "Git", "bin", "bash.exe"))
	}
	if pf86 := os.Getenv("ProgramFiles(x86)"); pf86 != "" {
		candidates = append(candidates, filepath.Join(pf86, "Git", "bin", "bash.exe"))
	}
	if lad := os.Getenv("LocalAppData"); lad != "" {
		candidates = append(candidates, filepath.Join(lad, "Programs", "Git", "bin", "bash.exe"))
	}
	return candidates
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
// extracting an embedded shim to a temp file on first use. '.ps1', '.sh',
// '.zsh', '.fish', and '.nu' each have a real shim today - any other
// extension is a documented gap (see DiscoverScriptFilters), not
// silently ignored.
func shimPathFor(ext string) (string, error) {
	switch ext {
	case ".ps1":
		return extractedShimPath(&ps1ShimOnce, &ps1ShimPath, &ps1ShimErr, "ironstate-filter-shim-*.ps1", shimPS1)
	case ".sh":
		return extractedShimPath(&shShimOnce, &shShimPath, &shShimErr, "ironstate-filter-shim-*.sh", shimSH)
	case ".zsh":
		return extractedShimPath(&zshShimOnce, &zshShimPath, &zshShimErr, "ironstate-filter-shim-*.zsh", shimZsh)
	case ".fish":
		return extractedShimPath(&fishShimOnce, &fishShimPath, &fishShimErr, "ironstate-filter-shim-*.fish", shimFish)
	case ".nu":
		return extractedShimPath(&nuShimOnce, &nuShimPath, &nuShimErr, "ironstate-filter-shim-*.nu", shimNu)
	default:
		return "", fmt.Errorf("no script-filter shim available for extension %q (only .ps1/.sh/.zsh/.fish/.nu ship one today)", ext)
	}
}

var (
	ps1ShimOnce sync.Once
	ps1ShimPath string
	ps1ShimErr  error

	shShimOnce sync.Once
	shShimPath string
	shShimErr  error

	zshShimOnce sync.Once
	zshShimPath string
	zshShimErr  error

	fishShimOnce sync.Once
	fishShimPath string
	fishShimErr  error

	nuShimOnce sync.Once
	nuShimPath string
	nuShimErr  error
)

// extractedShimPath writes content to a temp file matching pattern once
// (guarded by once/cachedPath/cachedErr, one triple per shim) and returns
// its path on every call thereafter.
func extractedShimPath(once *sync.Once, cachedPath *string, cachedErr *error, pattern string, content []byte) (string, error) {
	once.Do(func() {
		f, err := os.CreateTemp("", pattern)
		if err != nil {
			*cachedErr = err
			return
		}
		if _, err := f.Write(content); err != nil {
			_ = f.Close()
			*cachedErr = err
			return
		}
		*cachedErr = f.Close()
		*cachedPath = f.Name()
	})
	return *cachedPath, *cachedErr
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

// oneshotExtensions are the extensions whose shim is spawned fresh per
// call rather than reused as a persistent warm worker (see script.go's
// top-of-file comment and embed/shim.nu's doc comment for why).
var oneshotExtensions = map[string]bool{".nu": true}

// runOneshotProcess starts argv, writes stdin to its standard input
// (closing it immediately after, since every oneshot shim expects
// exactly one request per process), and returns its captured stdout/
// stderr - overridable in tests so they never spawn a real interpreter.
var runOneshotProcess = func(argv []string, stdin []byte) (stdout []byte, stderr []byte, err error) {
	cmd := exec.Command(argv[0], argv[1:]...) //nolint:gosec // argv is built from configured interpreters + a discovered script path, same trust boundary as startWorkerProcess
	cmd.Stdin = bytes.NewReader(stdin)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	runErr := cmd.Run()
	return stdoutBuf.Bytes(), stderrBuf.Bytes(), runErr
}

// oneshotFilterFunc is the non-persistent counterpart to
// workerFilterFunc/scriptWorker - it speaks the same one-line-JSON-
// request/one-line-JSON-response protocol, but over a fresh process per
// call instead of a long-lived worker.
func oneshotFilterFunc(argv []string) Func {
	return func(value any, args []any) (any, error) {
		if args == nil {
			args = []any{}
		}
		data, err := json.Marshal(scriptRequest{Value: value, Args: args})
		if err != nil {
			return nil, err
		}
		data = append(data, '\n')

		stdout, stderr, runErr := runOneshotProcess(argv, data)
		if runErr != nil {
			if errText := strings.TrimSpace(string(stderr)); errText != "" {
				return nil, fmt.Errorf("script filter %v: %w (stderr: %s)", argv, runErr, errText)
			}
			return nil, fmt.Errorf("script filter %v: %w", argv, runErr)
		}

		line := strings.TrimSpace(string(stdout))
		var resp scriptResponse
		if jsonErr := json.Unmarshal([]byte(line), &resp); jsonErr != nil {
			return nil, fmt.Errorf("script filter %v: invalid response %q: %w", argv, line, jsonErr)
		}
		if resp.Error != "" {
			return nil, fmt.Errorf("script filter %v: %s", argv, resp.Error)
		}
		return resp.Result, nil
	}
}

// DiscoverScriptFilters scans dir for filter script files (one per
// filter, named '<name><ext>') and registers each one into r under its
// base name - but only if r doesn't already have a filter of that name
// (a Go built-in, or one discovered earlier, always wins; today's
// filters/*.ps1, *.sh, *.zsh, *.fish, and *.nu files continue to work
// unmodified, and a net-new custom filter can be dropped in the same
// directory without recompiling the binary). interpreters maps a
// recognized extension to its interpreter argv prefix (DefaultInterpreters
// for the shipped default).
//
// If a name has more than one script implementing it (e.g. both leet.ps1
// and leet.sh), only the highest-priority extension is registered - see
// scriptFilterExtensionPriority.
//
// A missing dir is not an error (nothing to discover); an extension with
// no known interpreter, or no known shim (shimPathFor), is skipped
// (documented gap - only .ps1/.sh/.zsh/.fish/.nu ship one today), not a
// hard failure, since a plain "not a recognized filter script" file
// living alongside real ones is expected (e.g. filters/README.md).
//
// Returns a Pool the caller must Close() when done (usually at process
// exit) - holding every persistent worker started (not used for '.nu',
// which has no persistent worker to close, see oneshotExtensions) - and
// the names actually registered (for 'filters list'/'doctor' reporting).
func DiscoverScriptFilters(r *Registry, dir string, interpreters map[string][]string) (*Pool, []string, error) {
	pool := NewPool()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return pool, nil, nil
		}
		return pool, nil, err
	}

	// Group candidate files by filter name first, so a name with scripts
	// in more than one language picks its winner by scriptFilterExtensionPriority
	// rather than by directory-listing order.
	candidatesByName := map[string]map[string]string{} // name -> ext -> full path
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		name := strings.TrimSuffix(entry.Name(), ext)
		if name == "" {
			continue
		}
		if _, ok := interpreters[ext]; !ok {
			continue
		}
		byExt, ok := candidatesByName[name]
		if !ok {
			byExt = map[string]string{}
			candidatesByName[name] = byExt
		}
		byExt[ext] = filepath.Join(dir, entry.Name())
	}

	names := make([]string, 0, len(candidatesByName))
	for name := range candidatesByName {
		names = append(names, name)
	}
	sort.Strings(names)

	var registered []string
	for _, name := range names {
		if r.Has(name) {
			continue
		}
		byExt := candidatesByName[name]
		exts := make([]string, 0, len(byExt))
		for ext := range byExt {
			exts = append(exts, ext)
		}
		sort.Slice(exts, func(i, j int) bool {
			ri, rj := scriptFilterExtensionRank(exts[i]), scriptFilterExtensionRank(exts[j])
			if ri != rj {
				return ri < rj
			}
			return exts[i] < exts[j]
		})

		for _, ext := range exts {
			shimPath, shimErr := shimPathFor(ext)
			if shimErr != nil {
				continue
			}

			argvPrefix := interpreters[ext]
			argv := make([]string, 0, len(argvPrefix)+2)
			argv = append(argv, argvPrefix...)
			argv = append(argv, shimPath, byExt[ext])

			if oneshotExtensions[ext] {
				r.Register(name, oneshotFilterFunc(argv))
			} else {
				worker := pool.WorkerFor(name, argv)
				r.Register(name, workerFilterFunc(worker))
			}
			registered = append(registered, name)
			break
		}
	}
	return pool, registered, nil
}
