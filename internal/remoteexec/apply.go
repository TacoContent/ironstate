package remoteexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/TacoContent/ironstate/internal/remoteexec/protocol"
)

// Host statuses.
const (
	StatusOK          = "ok"
	StatusFailed      = "failed"
	StatusUnreachable = "unreachable"
	StatusError       = "error"
)

// Agent error phases that mean nothing was applied (bootstrap-like).
var bootstrapPhases = map[string]bool{"job": true, "lock": true, "bundle": true}

// HostOptions configures ApplyHost/PingHost.
type HostOptions struct {
	SSH       SSHOptions
	Agents    *AgentSource
	AgentDir  string
	AgentArgs []string
	OnEvent   func(protocol.Event)
	Cancel    <-chan struct{}
	// Dial, if set, replaces the SSH transport for non-local targets.
	Dial func(target string) (Transport, error)
}

// HostReport is one target's outcome.
type HostReport struct {
	Name     string
	Status   string
	Platform string
	// Uploaded is true when the agent binary had to be (re)sent.
	Uploaded bool
	Result   *HostResult
	Err      error
	Duration time.Duration
}

// ExitCode maps a set of reports to the remote-run exit code:
// 1 if any host failed, else 3 if any was unreachable/errored, else 0.
func ExitCode(reports []HostReport) int {
	code := 0
	for _, r := range reports {
		switch r.Status {
		case StatusFailed:
			return 1
		case StatusUnreachable, StatusError:
			code = 3
		}
	}
	return code
}

// transportFor returns the transport for a target name.
func transportFor(target string, opts HostOptions) (Transport, error) {
	if target == LocalTarget {
		return LocalTransport{}, nil
	}
	parsed, err := ParseSSHTarget(target)
	if err != nil {
		return nil, err
	}
	if opts.Dial != nil {
		return opts.Dial(target)
	}
	return NewSSHTransport(parsed, opts.SSH), nil
}

// prepareAgent connects, probes, checks preconditions and ensures the
// agent is present, returning the transport and the agent path to run.
func prepareAgent(ctx context.Context, target string, job *PreparedJob, opts HostOptions, report *HostReport) (Transport, string, ProbeInfo, error) {
	t, err := transportFor(target, opts)
	if err != nil {
		return nil, "", ProbeInfo{}, &StageError{Stage: "target", Err: err}
	}
	if target == LocalTarget {
		exe, err := os.Executable()
		report.Platform = "local"
		return t, exe, ProbeInfo{}, err
	}
	info, err := Probe(ctx, t, opts.AgentDir)
	if err != nil {
		return t, "", info, err
	}
	report.Platform = info.Platform()
	if job != nil {
		if err := checkSupported(job, info); err != nil {
			return t, "", info, &StageError{Stage: "preflight", Err: err}
		}
	}
	agent, err := opts.Agents.Resolve(info.Platform())
	if err != nil {
		return t, "", info, &StageError{Stage: "agent", Err: err}
	}
	remotePath, uploaded, err := EnsureAgent(ctx, t, info, opts.Agents.Version, agent)
	report.Uploaded = uploaded
	return t, remotePath, info, err
}

// checkSupported rejects playbook features remote targets can't run yet.
func checkSupported(job *PreparedJob, info ProbeInfo) error {
	if job.UsesPlugins {
		return errors.New("playbooks that declare 'plugins:' can't be applied remotely yet")
	}
	if len(job.RemoteUses) > 0 {
		return fmt.Errorf("remote 'uses:' sources can't be applied remotely yet: %s", strings.Join(job.RemoteUses, ", "))
	}
	if job.UsesBecome && info.Sudo != "nopasswd" {
		reason := "sudo requires a password (or 'requiretty' is set)"
		if info.Sudo == "missing" {
			reason = "sudo is not installed"
		}
		return fmt.Errorf("playbook uses 'become' but %s on the target; configure passwordless sudo for this user", reason)
	}
	return nil
}

// ApplyHost runs job on one target end to end and never panics on a
// per-host failure: every outcome is in the returned report.
func ApplyHost(ctx context.Context, target string, job *PreparedJob, opts HostOptions) HostReport {
	start := time.Now()
	report := HostReport{Name: target}
	defer func() { report.Duration = time.Since(start) }()

	t, agentPath, _, err := prepareAgent(ctx, target, job, opts, &report)
	if t != nil {
		defer func() { _ = t.Close() }()
	}
	if err != nil {
		report.Err = err
		report.Status = statusForError(err)
		report.Duration = time.Since(start)
		return report
	}

	result, err := RunHost(ctx, t, agentPath, job, RunOptions{OnEvent: opts.OnEvent, Cancel: opts.Cancel, AgentArgs: opts.AgentArgs})
	report.Result = result
	report.Duration = time.Since(start)
	switch {
	case err != nil:
		report.Err = err
		report.Status = StatusError
	case result.ExitCode == 0:
		report.Status = StatusOK
	case bootstrapPhases[result.FailedPhase]:
		report.Status = StatusError
		report.Err = errors.New(strings.Join(result.Errors, "; "))
	default:
		report.Status = StatusFailed
		report.Err = errors.New(strings.Join(result.Errors, "; "))
	}
	return report
}

func statusForError(err error) string {
	var stage *StageError
	if errors.As(err, &stage) && stage.Unreachable {
		return StatusUnreachable
	}
	return StatusError
}

// PingReport is the outcome of PingHost.
type PingReport struct {
	HostReport
	Probe        ProbeInfo
	AgentPath    string
	AgentVersion string
}

// PingHost connects, probes, ensures the agent and runs 'ironstate
// version' with it, without applying anything.
func PingHost(ctx context.Context, target string, opts HostOptions) PingReport {
	start := time.Now()
	report := PingReport{HostReport: HostReport{Name: target}}
	defer func() { report.Duration = time.Since(start) }()

	t, agentPath, info, err := prepareAgent(ctx, target, nil, opts, &report.HostReport)
	if t != nil {
		defer func() { _ = t.Close() }()
	}
	report.AgentPath = agentPath
	report.Probe = info
	if err != nil {
		report.Err = err
		report.Status = statusForError(err)
		return report
	}
	var out bytes.Buffer
	code, err := t.Exec(ctx, RemoteCommand{Program: agentPath, Args: []string{"version"}}, bytes.NewReader(nil), &out, &out)
	if err == nil && code != 0 {
		err = &StageError{Stage: "agent version", Unreachable: code == SSHExitUnreachable, Err: fmt.Errorf("exit %d: %s", code, strings.TrimSpace(out.String()))}
	}
	if err != nil {
		report.Err = err
		report.Status = statusForError(err)
		return report
	}
	report.AgentVersion = strings.TrimSpace(out.String())
	report.Status = StatusOK
	return report
}
