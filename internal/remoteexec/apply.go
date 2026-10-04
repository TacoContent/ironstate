package remoteexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
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
	SSH                   SSHOptions
	Agents                *AgentSource
	AgentDir              string
	AgentArgs             []string
	HostTimeout           time.Duration
	Detach                bool
	SkipAgentVerification bool
	OnEvent               func(protocol.Event)
	Cancel                <-chan struct{}
	// Dial, if set, replaces the SSH transport for non-local hosts.
	Dial func(host Host) (Transport, error)
}

// HostReport is one target's outcome.
type HostReport struct {
	Name     string
	Address  string
	Status   string
	Platform string
	// Uploaded is true when the agent binary had to be (re)sent.
	Uploaded bool
	// PluginsShipped lists plugins copied from the controller's store.
	PluginsShipped      []string
	Detached            bool
	VerificationSkipped bool
	Result              *HostResult
	Err                 error
	Duration            time.Duration
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

// transportFor returns the transport for a host.
func transportFor(host Host, opts HostOptions) (Transport, error) {
	if host.Local {
		return LocalTransport{}, nil
	}
	if opts.Dial != nil {
		return opts.Dial(host)
	}
	return NewSSHTransport(host.SSH, opts.SSH), nil
}

// prepareAgent connects, probes, checks preconditions and ensures the
// agent is present, returning the transport and the agent path to run.
func prepareAgent(ctx context.Context, host Host, job *PreparedJob, opts HostOptions, report *HostReport) (Transport, string, ProbeInfo, error) {
	t, err := transportFor(host, opts)
	if err != nil {
		return nil, "", ProbeInfo{}, &StageError{Stage: "target", Err: err}
	}
	if host.Local {
		exe, err := os.Executable()
		report.Platform = "local"
		return t, exe, ProbeInfo{}, err
	}
	agentDir := opts.AgentDir
	if host.AgentDir != "" {
		agentDir = host.AgentDir
	}
	info, err := ProbeWithHint(ctx, t, agentDir, host.Platform)
	if err != nil {
		return t, "", info, err
	}
	report.Platform = info.Platform()
	info.SkipAgentVerification = host.SkipAgentVerification || opts.SkipAgentVerification
	report.VerificationSkipped = info.SkipAgentVerification
	if job != nil {
		if err := checkSupported(job, info); err != nil {
			return t, "", info, &StageError{Stage: "preflight", Err: err}
		}
	}
	agent, err := opts.Agents.Resolve(ctx, info.Platform())
	if err != nil {
		return t, "", info, &StageError{Stage: "agent", Err: err}
	}
	remotePath, uploaded, err := EnsureAgent(ctx, t, info, opts.Agents.Version, agent)
	report.Uploaded = uploaded
	if err == nil && job != nil && len(job.Plugins) > 0 && info.Platform() == runtime.GOOS+"/"+runtime.GOARCH {
		report.PluginsShipped, err = EnsurePlugins(ctx, t, info, job)
	}
	return t, remotePath, info, err
}

const bootstrapAttempts = 3

func prepareAgentWithRetry(ctx context.Context, host Host, job *PreparedJob, opts HostOptions, report *HostReport) (Transport, string, ProbeInfo, error) {
	for attempt := 0; attempt < bootstrapAttempts; attempt++ {
		transport, path, info, err := prepareAgent(ctx, host, job, opts, report)
		if err == nil || attempt == bootstrapAttempts-1 || !retryableBootstrapError(err) {
			return transport, path, info, err
		}
		if transport != nil {
			_ = transport.Close()
		}
		delay := time.Duration(1<<attempt) * 250 * time.Millisecond
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, "", info, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, "", ProbeInfo{}, errors.New("bootstrap retries exhausted")
}

func retryableBootstrapError(err error) bool {
	var stage *StageError
	if errors.As(err, &stage) && stage.Unreachable {
		return true
	}
	return strings.Contains(err.Error(), "sftp exited 255:")
}

func hostContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, timeout)
}

// checkSupported rejects runs a target can't perform.
func checkSupported(job *PreparedJob, info ProbeInfo) error {
	if job.UsesBecome && !job.Job.Options.DisableBecome && info.OS != "windows" && info.Sudo != "nopasswd" && job.Job.BecomePassword == "" {
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
func ApplyHost(ctx context.Context, host Host, job *PreparedJob, opts HostOptions) HostReport {
	start := time.Now()
	report := HostReport{Name: host.Name, Address: host.Address()}
	ctx, cancel := hostContext(ctx, opts.HostTimeout)
	defer cancel()

	hostJob := *job
	hostJob.Job = job.Job
	hostJob.Job.Options = job.Job.Options
	hostJob.Job.Options.DisableBecome = host.DisableBecome
	t, agentPath, info, err := prepareAgentWithRetry(ctx, host, &hostJob, opts, &report)
	if t != nil {
		defer func() { _ = t.Close() }()
	}
	if err != nil {
		report.Err = err
		report.Status = statusForError(err)
		report.Duration = time.Since(start)
		return report
	}
	if opts.Detach {
		err = StartDetached(ctx, t, agentPath, info, &hostJob)
		report.Duration = time.Since(start)
		report.Detached = err == nil
		report.Result = &HostResult{RunID: hostJob.Job.RunID}
		if err != nil {
			report.Err = err
			report.Status = StatusError
		} else {
			report.Status = StatusOK
		}
		return report
	}

	result, err := RunHost(ctx, t, agentPath, &hostJob, RunOptions{OnEvent: opts.OnEvent, Cancel: opts.Cancel, AgentArgs: opts.AgentArgs})
	if err != nil && result != nil && !result.Completed && !host.Local && ctx.Err() == nil {
		if recovered, recoveryErr := RecoverRunLog(ctx, t, info, hostJob.Job.RunID); recovered != nil {
			result = recovered
			if recoveryErr == nil && recovered.Completed {
				err = nil
			}
		}
	}
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
func PingHost(ctx context.Context, host Host, opts HostOptions) PingReport {
	start := time.Now()
	report := PingReport{HostReport: HostReport{Name: host.Name, Address: host.Address()}}
	ctx, cancel := hostContext(ctx, opts.HostTimeout)
	defer cancel()
	done := func(err error) PingReport {
		report.Duration = time.Since(start)
		if err != nil {
			report.Err = err
			report.Status = statusForError(err)
		} else {
			report.Status = StatusOK
		}
		return report
	}

	t, agentPath, info, err := prepareAgentWithRetry(ctx, host, nil, opts, &report.HostReport)
	if t != nil {
		defer func() { _ = t.Close() }()
	}
	report.AgentPath = agentPath
	report.Probe = info
	if err != nil {
		return done(err)
	}
	var out, stderr bytes.Buffer
	code, err := t.Exec(ctx, RemoteCommand{Program: agentPath, Args: []string{"version"}, Windows: strings.HasSuffix(strings.ToLower(agentPath), ".exe")}, bytes.NewReader(nil), &out, &stderr)
	if err == nil && code != 0 {
		detail := strings.TrimSpace(strings.Join([]string{out.String(), stderr.String()}, "\n"))
		err = &StageError{Stage: "agent version", Unreachable: code == SSHExitUnreachable, Err: fmt.Errorf("exit %d: %s", code, detail)}
	}
	if err != nil {
		return done(err)
	}
	report.AgentVersion = agentVersionText(out.String(), stderr.String())
	return done(nil)
}

func agentVersionText(outputs ...string) string {
	for _, output := range outputs {
		index := strings.Index(output, "ironstate ")
		if index < 0 {
			continue
		}
		version := output[index:]
		if end := strings.IndexAny(version, "\r\n<"); end >= 0 {
			version = version[:end]
		}
		return strings.TrimSpace(version)
	}
	if len(outputs) == 0 {
		return ""
	}
	return strings.TrimSpace(outputs[0])
}

// ForEachHost runs fn for every host with at most forks running at once
// and returns the reports in host order.
func ForEachHost(hosts []Host, forks int, fn func(Host) HostReport) []HostReport {
	if forks < 1 {
		forks = 1
	}
	reports := make([]HostReport, len(hosts))
	sem := make(chan struct{}, forks)
	var wg sync.WaitGroup
	for i, host := range hosts {
		sem <- struct{}{}
		wg.Add(1)
		go func(i int, host Host) {
			defer func() { <-sem; wg.Done() }()
			reports[i] = fn(host)
		}(i, host)
	}
	wg.Wait()
	return reports
}

// NotStarted is the report for a host skipped because the run was cancelled.
func NotStarted(host Host) HostReport {
	return HostReport{Name: host.Name, Address: host.Address(), Status: StatusError, Err: errors.New("not started: run cancelled")}
}

// Cancelled reports whether ch is closed.
func Cancelled(ch <-chan struct{}) bool {
	if ch == nil {
		return false
	}
	select {
	case <-ch:
		return true
	default:
		return false
	}
}
