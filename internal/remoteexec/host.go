package remoteexec

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"

	"github.com/TacoContent/ironstate/internal/engine"
	"github.com/TacoContent/ironstate/internal/remoteexec/protocol"
)

// maxStderrBytes bounds how much agent stderr is kept per host.
const maxStderrBytes = 64 << 10

// HostResult is everything one agent run reported.
type HostResult struct {
	RunID   string
	Hello   *protocol.Event
	Facts   map[string]any
	Results []engine.JSONResult
	Logs    []protocol.Event
	Summary *protocol.Event
	Errors  []string
	// FailedPhase is the phase of the first 'error' event, if any.
	FailedPhase string
	ExitCode    int
	// Completed is true only if the agent's 'done' event arrived.
	Completed bool
	// Diagnostics holds non-protocol stdout lines (e.g. shell rc noise).
	Diagnostics []string
	Stderr      string
}

// RunOptions tunes one RunHost call.
type RunOptions struct {
	// OnEvent, if set, sees each event as it arrives.
	OnEvent func(protocol.Event)
	// Cancel, when closed, asks the agent to stop after its current leaf.
	Cancel <-chan struct{}
	// AgentArgs are appended to the agent command line.
	AgentArgs []string
}

// RunHost starts the agent at agentPath through t, sends job + bundle, and
// collects the event stream. The channel stays open after the bundle for
// control lines; closing it is how the agent learns the controller is
// gone. A non-nil error means the run could not be carried out or didn't
// finish; the partial HostResult is still returned.
func RunHost(ctx context.Context, t Transport, agentPath string, job *PreparedJob, opts RunOptions) (*HostResult, error) {
	bundleFile, err := os.Open(job.BundlePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = bundleFile.Close() }()

	var header bytes.Buffer
	if err := protocol.WriteJob(&header, job.Job); err != nil {
		return nil, err
	}
	controlR, controlW := io.Pipe()
	stdin := io.MultiReader(&header, bundleFile, controlR)

	result := &HostResult{RunID: job.Job.RunID}
	stdoutR, stdoutW := io.Pipe()
	var wg sync.WaitGroup
	var readErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		// The transport only returns once stdin is drained, so the control
		// channel must close as soon as the agent says it's done.
		readErr = collectEvents(stdoutR, result, func(ev protocol.Event) {
			if ev.Type == protocol.TypeDone {
				_ = controlW.Close()
			}
			if opts.OnEvent != nil {
				opts.OnEvent(ev)
			}
		})
		_ = controlW.Close()
		// Keep draining so the agent never blocks on a full pipe.
		_, _ = io.Copy(io.Discard, stdoutR)
	}()
	if opts.Cancel != nil {
		stop := make(chan struct{})
		defer close(stop)
		go func() {
			select {
			case <-opts.Cancel:
				_ = protocol.WriteCancel(controlW)
			case <-stop:
			}
		}()
	}

	stderr := &tailBuffer{max: maxStderrBytes}
	args := append([]string{"agent", "--protocol", strconv.Itoa(protocol.Version)}, opts.AgentArgs...)
	cmd := RemoteCommand{Program: agentPath, Args: args}
	exitCode, execErr := t.Exec(ctx, cmd, stdin, stdoutW, stderr)
	_ = stdoutW.Close()
	_ = controlW.Close()
	wg.Wait()
	result.Stderr = stderr.String()

	if execErr != nil {
		return result, fmt.Errorf("start agent: %w", execErr)
	}
	if readErr != nil {
		return result, readErr
	}
	if !result.Completed {
		result.ExitCode = exitCode
		return result, fmt.Errorf("agent exited %d without completing the run", exitCode)
	}
	if result.Hello != nil && result.Hello.RunID != job.Job.RunID {
		return result, fmt.Errorf("agent answered for run %q, expected %q", result.Hello.RunID, job.Job.RunID)
	}
	return result, nil
}

func collectEvents(r io.Reader, result *HostResult, onEvent func(protocol.Event)) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), protocol.MaxLineBytes+1)
	for scanner.Scan() {
		line := scanner.Bytes()
		ev, ok := protocol.ParseEvent(line)
		if !ok {
			if len(bytes.TrimSpace(line)) > 0 {
				result.Diagnostics = append(result.Diagnostics, string(line))
			}
			continue
		}
		applyEvent(result, ev)
		if onEvent != nil {
			onEvent(ev)
		}
	}
	return scanner.Err()
}

func applyEvent(result *HostResult, ev protocol.Event) {
	switch ev.Type {
	case protocol.TypeHello:
		hello := ev
		result.Hello = &hello
	case protocol.TypeFacts:
		result.Facts = ev.Facts
	case protocol.TypeLeafResult:
		if ev.Result != nil {
			result.Results = append(result.Results, *ev.Result)
		}
	case protocol.TypeLog:
		result.Logs = append(result.Logs, ev)
	case protocol.TypeSummary:
		summary := ev
		result.Summary = &summary
	case protocol.TypeError:
		if len(result.Errors) == 0 {
			result.FailedPhase = ev.Phase
		}
		result.Errors = append(result.Errors, ev.Error)
	case protocol.TypeDone:
		result.Completed = true
		if ev.ExitCode != nil {
			result.ExitCode = *ev.ExitCode
		}
	}
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if over := len(b.buf) - b.max; over > 0 {
		b.buf = b.buf[over:]
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}
