// Package protocol defines the line-delimited JSON contract between a
// remote-apply controller and an 'ironstate agent' process (see
// docs/plans/remote-apply.md §9): one job header line in, NDJSON events out.
package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/TacoContent/ironstate/internal/engine"
	"github.com/TacoContent/ironstate/internal/secrets"
)

// Version is the protocol version spoken by this build.
const Version = 1

// MaxLineBytes bounds a single protocol line on either side.
const MaxLineBytes = 8 << 20

// MaxFieldBytes bounds a leaf result's stdout/stderr inside an event.
const MaxFieldBytes = 1 << 20

// Event types.
const (
	TypeJob        = "job"
	TypeHello      = "hello"
	TypeFacts      = "facts"
	TypeLeafStart  = "leaf_start"
	TypeLeafResult = "leaf_result"
	TypeLog        = "log"
	TypeSummary    = "summary"
	TypeError      = "error"
	TypeDone       = "done"
	// TypeCancel is a controller -> agent control line sent after the bundle.
	TypeCancel = "cancel"
)

// Log levels carried by TypeLog events.
const (
	LevelInfo   = "info"
	LevelWarn   = "warn"
	LevelDanger = "danger"
)

// linePrefix is what every protocol line starts with; anything else on the
// agent's stdout is treated as a diagnostic, never a parse error.
var linePrefix = []byte(fmt.Sprintf(`{"v":%d,`, Version))

// Job is the controller's header line, followed on the same stream by
// exactly Bundle.Size bytes of tar.gz.
type Job struct {
	V                 int               `json:"v"`
	Type              string            `json:"type"`
	RunID             string            `json:"run_id"`
	ControllerVersion string            `json:"controller_version"`
	Options           JobOptions        `json:"options"`
	Env               map[string]string `json:"env,omitempty"`
	SecretEnv         map[string]string `json:"secret_env,omitempty"`
	BecomePassword    string            `json:"become_password,omitempty"`
	// ApprovedUses lists remote 'uses:' sources (as remote.Describe renders
	// them) the controller approved and pre-fetched into the bundle.
	ApprovedUses []string   `json:"approved_uses,omitempty"`
	Bundle       BundleInfo `json:"bundle"`
}

// JobOptions mirrors the apply flags forwarded to the agent. Paths are
// relative to the unpacked bundle root.
type JobOptions struct {
	Playbook      string   `json:"playbook"`
	VarsFiles     []string `json:"vars_files,omitempty"`
	VarOverrides  []string `json:"var_overrides,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Apply         bool     `json:"apply"`
	Verbose       bool     `json:"verbose,omitempty"`
	DisableBecome bool     `json:"disable_become,omitempty"`
	// AllowPluginInstall lets the agent install missing plugins on the target.
	AllowPluginInstall bool `json:"allow_plugin_install,omitempty"`
}

// BundleInfo describes the tar.gz payload following the header.
type BundleInfo struct {
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Event is one agent -> controller line. Field order keeps "v" then "type"
// first so every line starts with linePrefix.
type Event struct {
	V    int    `json:"v"`
	Type string `json:"type"`
	Host string `json:"host,omitempty"`

	RunID   string `json:"run_id,omitempty"`
	Version string `json:"version,omitempty"`
	OS      string `json:"os,omitempty"`
	Arch    string `json:"arch,omitempty"`
	PID     int    `json:"pid,omitempty"`
	RunLog  string `json:"run_log,omitempty"`

	Facts map[string]any `json:"facts,omitempty"`

	Stage string `json:"stage,omitempty"`
	Label string `json:"label,omitempty"`
	Index int    `json:"index,omitempty"`
	Total int    `json:"total,omitempty"`

	Result *engine.JSONResult `json:"result,omitempty"`

	Level   string `json:"level,omitempty"`
	Message string `json:"message,omitempty"`

	Stats     *engine.Stats `json:"stats,omitempty"`
	Stopped   bool          `json:"stopped,omitempty"`
	ElapsedMS float64       `json:"elapsed_ms,omitempty"`

	Phase string `json:"phase,omitempty"`
	Error string `json:"error,omitempty"`

	ExitCode *int `json:"exit_code,omitempty"`
}

// WriteJob writes job as a single header line.
func WriteJob(w io.Writer, job Job) error {
	job.V = Version
	job.Type = TypeJob
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

// ReadJob reads and validates the header line from r. The bundle bytes
// must then be read from the same r.
func ReadJob(r *bufio.Reader) (Job, error) {
	line, err := readLine(r)
	if err != nil {
		return Job{}, fmt.Errorf("read job header: %w", err)
	}
	var job Job
	if err := json.Unmarshal(line, &job); err != nil {
		return Job{}, fmt.Errorf("parse job header: %w", err)
	}
	if job.Type != TypeJob {
		return Job{}, fmt.Errorf("expected %q header, got %q", TypeJob, job.Type)
	}
	if job.V != Version {
		return Job{}, fmt.Errorf("protocol version mismatch: controller %d, agent %d", job.V, Version)
	}
	if job.Bundle.Size < 0 {
		return Job{}, errors.New("negative bundle size")
	}
	return job, nil
}

func readLine(r *bufio.Reader) ([]byte, error) {
	var buf []byte
	for {
		chunk, err := r.ReadSlice('\n')
		buf = append(buf, chunk...)
		if len(buf) > MaxLineBytes {
			return nil, fmt.Errorf("line exceeds %d bytes", MaxLineBytes)
		}
		if err == nil {
			return bytes.TrimRight(buf, "\r\n"), nil
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return nil, err
		}
	}
}

// Control is a controller -> agent line sent after the bundle bytes.
type Control struct {
	V    int    `json:"v"`
	Type string `json:"type"`
}

// WriteCancel sends a cancel control line.
func WriteCancel(w io.Writer) error {
	data, err := json.Marshal(Control{V: Version, Type: TypeCancel})
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

// ReadControl reads the next control line's type from r. Non-control
// lines are skipped; io.EOF means the controller closed the channel.
func ReadControl(r *bufio.Reader) (string, error) {
	for {
		line, err := readLine(r)
		if err != nil {
			return "", err
		}
		var c Control
		if json.Unmarshal(line, &c) == nil && c.V == Version && c.Type != "" {
			return c.Type, nil
		}
	}
}

// ParseEvent decodes one agent stdout line. ok is false for any line that
// isn't a protocol event (shell rc noise, stray prints).
func ParseEvent(line []byte) (Event, bool) {
	line = bytes.TrimRight(line, "\r\n")
	if !bytes.HasPrefix(line, linePrefix) {
		return Event{}, false
	}
	var ev Event
	if err := json.Unmarshal(line, &ev); err != nil || ev.Type == "" {
		return Event{}, false
	}
	return ev, true
}

// Writer emits events as NDJSON to a stream and, optionally, a run log.
// Losing the stream (controller gone) never stops run-log writes. Safe
// for concurrent use.
type Writer struct {
	mu        sync.Mutex
	w         io.Writer
	streamErr error
	log       io.Writer
	done      bool
	cancel    error
}

// NewWriter returns a Writer emitting to w.
func NewWriter(w io.Writer) *Writer { return &Writer{w: w} }

// SetLog tees every later event to log, one complete line per Write.
func (w *Writer) SetLog(log io.Writer) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.log = log
}

// Cancel records why the run should stop after the current leaf. The
// first reason wins.
func (w *Writer) Cancel(reason error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel == nil {
		w.cancel = reason
	}
}

// Cancelled returns the cancel reason, or a disconnect error once the
// stream has failed, or nil.
func (w *Writer) Cancelled() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		return w.cancel
	}
	if w.streamErr != nil {
		return fmt.Errorf("controller disconnected: %w", w.streamErr)
	}
	return nil
}

// Emit writes ev as one line. Events after Done are dropped. Returns the
// sticky stream error, if any.
func (w *Writer) Emit(ev Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.emitLocked(ev)
}

func (w *Writer) emitLocked(ev Event) error {
	if w.done {
		return w.streamErr
	}
	ev.V = Version
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(ev); err != nil {
		return err
	}
	if buf.Len() > MaxLineBytes {
		return fmt.Errorf("event %q exceeds %d bytes", ev.Type, MaxLineBytes)
	}
	if w.log != nil {
		_, _ = w.log.Write(buf.Bytes())
	}
	if w.streamErr == nil {
		if _, err := w.w.Write(buf.Bytes()); err != nil {
			w.streamErr = err
		}
	}
	return w.streamErr
}

// Hello emits the agent's opening event.
func (w *Writer) Hello(runID, version, goos, arch string, pid int, runLog string) error {
	return w.Emit(Event{Type: TypeHello, RunID: runID, Version: version, OS: goos, Arch: arch, PID: pid, RunLog: runLog})
}

// Log emits a redacted log line.
func (w *Writer) Log(level, message string) error {
	return w.Emit(Event{Type: TypeLog, Level: level, Message: secrets.Redact(message)})
}

// LeafStart emits a progress event for the leaf about to dispatch.
func (w *Writer) LeafStart(stage, label string, index, total int) error {
	return w.Emit(Event{Type: TypeLeafStart, Stage: stage, Label: secrets.Redact(label), Index: index, Total: total})
}

// LeafResult emits r, truncating oversized stdout/stderr.
func (w *Writer) LeafResult(r engine.Result) error {
	jr := truncateResult(engine.ToJSONResult(r))
	return w.Emit(Event{Type: TypeLeafResult, Result: &jr})
}

// Facts emits the gathered facts with string values redacted.
func (w *Writer) Facts(facts map[string]any) error {
	redacted, _ := redactValue(facts).(map[string]any)
	return w.Emit(Event{Type: TypeFacts, Facts: redacted})
}

// Summary emits the run's final stats.
func (w *Writer) Summary(stats engine.Stats, stopped bool, elapsed time.Duration) error {
	return w.Emit(Event{Type: TypeSummary, Stats: &stats, Stopped: stopped, ElapsedMS: float64(elapsed) / float64(time.Millisecond)})
}

// Fail emits an error event followed by done, unless done was already sent.
func (w *Writer) Fail(phase string, err error, exitCode int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.done {
		return w.streamErr
	}
	if err != nil {
		_ = w.emitLocked(Event{Type: TypeError, Phase: phase, Error: secrets.Redact(err.Error())})
	}
	return w.doneLocked(exitCode)
}

// Done emits the terminal event. Later calls are no-ops.
func (w *Writer) Done(exitCode int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.doneLocked(exitCode)
}

func (w *Writer) doneLocked(exitCode int) error {
	if w.done {
		return w.streamErr
	}
	code := exitCode
	err := w.emitLocked(Event{Type: TypeDone, ExitCode: &code})
	w.done = true
	return err
}

// IsDone reports whether the done event was already emitted.
func (w *Writer) IsDone() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.done
}

func truncateResult(r engine.JSONResult) engine.JSONResult {
	var cut bool
	r.Exec.Stdout, r.Exec.StdoutLines, cut = truncateOutput(r.Exec.Stdout, r.Exec.StdoutLines)
	r.Exec.Truncated = cut
	r.Exec.Stderr, r.Exec.StderrLines, cut = truncateOutput(r.Exec.Stderr, r.Exec.StderrLines)
	r.Exec.Truncated = r.Exec.Truncated || cut
	return r
}

func truncateOutput(text string, lines []string) (string, []string, bool) {
	cut := false
	if len(text) > MaxFieldBytes {
		text = text[:MaxFieldBytes]
		cut = true
	}
	size := 0
	for i, line := range lines {
		size += len(line) + 1
		if size > MaxFieldBytes {
			return text, lines[:i], true
		}
	}
	return text, lines, cut
}

func redactValue(v any) any {
	switch val := v.(type) {
	case string:
		return secrets.Redact(val)
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, child := range val {
			out[k] = redactValue(child)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, child := range val {
			out[i] = redactValue(child)
		}
		return out
	default:
		return v
	}
}
