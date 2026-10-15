package remoteexec

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/TacoContent/ironstate/internal/remoteexec/protocol"
)

var runIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ReadRunLog fetches one target-side event log. An empty runID selects the
// newest retained run. Truncated final lines are ignored during parsing.
func ReadRunLog(ctx context.Context, t Transport, info ProbeInfo, runID string) (*HostResult, error) {
	if runID == "" {
		latest, err := latestRunID(ctx, t, info)
		if err != nil {
			return nil, err
		}
		runID = latest
	}
	if !runIDPattern.MatchString(runID) {
		return nil, fmt.Errorf("invalid run id %q", runID)
	}
	logPath := joinRemotePath(info.OS, info.StateDir, "runs", runID, "events.ndjson")
	var cmd RemoteCommand
	if info.OS == "windows" {
		encoded := base64.StdEncoding.EncodeToString([]byte(logPath))
		script := `$p=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + encoded + `')); [Console]::Out.Write([IO.File]::ReadAllText($p))`
		cmd = powerShellScript(script)
	} else {
		cmd = shCommand(`cat "$1"`, logPath)
	}
	out, err := execCapture(ctx, t, "read run log", cmd, nil)
	if err != nil {
		return nil, err
	}
	return parseRunLog(runID, out)
}

func parseRunLog(runID, out string) (*HostResult, error) {
	if !runIDPattern.MatchString(runID) {
		return nil, fmt.Errorf("invalid run id %q", runID)
	}
	result := &HostResult{RunID: runID}
	completeBytes := out
	if last := strings.LastIndexByte(completeBytes, '\n'); last >= 0 {
		completeBytes = completeBytes[:last+1]
	} else {
		completeBytes = ""
	}
	result.RawLog = completeBytes
	if err := collectEvents(bytes.NewBufferString(completeBytes), result, nil); err != nil {
		return nil, err
	}
	var clean bytes.Buffer
	for _, line := range bytes.Split([]byte(completeBytes), []byte("\n")) {
		line = bytes.TrimSuffix(line, []byte("\r"))
		if _, ok := protocol.ParseEvent(line); ok {
			clean.Write(line)
			clean.WriteByte('\n')
		}
	}
	result.RawLog = clean.String()
	return result, nil
}

// RecoverRunLog reconnects briefly to collect events written after an apply
// stream was interrupted. It never starts the playbook again.
func RecoverRunLog(ctx context.Context, t Transport, info ProbeInfo, runID string) (*HostResult, error) {
	var partial *HostResult
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		result, err := ReadRunLog(ctx, t, info, runID)
		if err == nil {
			partial = result
			if result.Completed {
				return result, nil
			}
		} else {
			lastErr = err
		}
		if attempt == 3 {
			break
		}
		timer := time.NewTimer(time.Duration(1<<attempt) * 250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return partial, ctx.Err()
		case <-timer.C:
		}
	}
	if partial != nil {
		return partial, nil
	}
	if lastErr == nil {
		lastErr = errors.New("run log unavailable")
	}
	return nil, lastErr
}

// ReadHostLogs probes a host and reads the requested run log (or latest log).
func ReadHostLogs(ctx context.Context, host Host, opts HostOptions, runID string) (*HostResult, error) {
	t, err := transportFor(host, opts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = t.Close() }()
	if host.Local {
		return nil, errors.New("remote logs is not supported for the reserved local target")
	}
	info, err := ProbeWithHint(ctx, t, opts.AgentDir, host.Platform)
	if err != nil {
		return nil, err
	}
	return ReadRunLog(ctx, t, info, runID)
}

// CleanHost removes cached agents and retained logs after confirming no
// apply currently holds the target's run lock.
func CleanHost(ctx context.Context, host Host, opts HostOptions) error {
	t, err := transportFor(host, opts)
	if err != nil {
		return err
	}
	defer func() { _ = t.Close() }()
	if host.Local {
		return errors.New("remote clean is not supported for the reserved local target")
	}
	info, err := ProbeWithHint(ctx, t, opts.AgentDir, host.Platform)
	if err != nil {
		return err
	}
	var cmd RemoteCommand
	if info.OS == "windows" {
		encode := func(value string) string { return base64.StdEncoding.EncodeToString([]byte(value)) }
		state := encode(info.StateDir)
		agents := encode(info.AgentDir)
		script := `$ErrorActionPreference='Stop'; $s=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + state + `')); ` +
			`$a=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + agents + `')); ` +
			`$lock=Join-Path $s 'apply.lock'; [IO.Directory]::CreateDirectory($s)|Out-Null; ` +
			`$held=$null; try {$held=[IO.File]::Open($lock,[IO.FileMode]::OpenOrCreate,[IO.FileAccess]::ReadWrite,[IO.FileShare]::None)} catch {throw 'an apply is running; remote clean refused'}; ` +
			`try {Get-ChildItem -LiteralPath $a -Force -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force; ` +
			`$runs=Join-Path $s 'runs'; Get-ChildItem -LiteralPath $runs -Force -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force; ` +
			`$detached=Join-Path $s 'detached'; Get-ChildItem -LiteralPath $detached -Force -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force} finally {$held.Dispose()}`
		cmd = powerShellScript(script)
	} else {
		script := `if ! command -v flock >/dev/null 2>&1; then echo 'remote clean requires flock to protect active runs' >&2; exit 2; fi; ` +
			`mkdir -p "$2"; touch "$2/apply.lock"; ` +
			`(flock -n 9 || { echo 'an apply is running; remote clean refused' >&2; exit 3; }; ` +
			`for p in "$1"/*; do [ ! -e "$p" ] || rm -rf -- "$p"; done; ` +
			`for p in "$2/runs"/* "$2/detached"/*; do [ ! -e "$p" ] || rm -rf -- "$p"; done) 9>>"$2/apply.lock"`
		cmd = shCommand(script, info.AgentDir, info.StateDir)
	}
	_, err = execCapture(ctx, t, "clean remote state", cmd, nil)
	return err
}

func latestRunID(ctx context.Context, t Transport, info ProbeInfo) (string, error) {
	runsDir := joinRemotePath(info.OS, info.StateDir, "runs")
	var cmd RemoteCommand
	if info.OS == "windows" {
		encoded := base64.StdEncoding.EncodeToString([]byte(runsDir))
		script := `$d=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + encoded + `')); $r=Get-ChildItem -LiteralPath $d -Directory -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending | Select-Object -First 1; if ($null -eq $r) {exit 4}; [Console]::Out.Write($r.Name)`
		cmd = powerShellScript(script)
	} else {
		cmd = shCommand(`latest=$(ls -1t "$1" 2>/dev/null | head -n 1); [ -n "$latest" ] || exit 4; printf '%s' "$latest"`, runsDir)
	}
	out, err := execCapture(ctx, t, "find latest run log", cmd, nil)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(out)
	if !runIDPattern.MatchString(value) {
		return "", fmt.Errorf("target returned invalid latest run id %q", value)
	}
	return value, nil
}

func joinRemotePath(goos string, parts ...string) string {
	if goos == "windows" {
		return strings.Join(parts, `\`)
	}
	return strings.Join(parts, "/")
}
