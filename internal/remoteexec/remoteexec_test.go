package remoteexec_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/internal/cli"
	"github.com/TacoContent/ironstate/internal/remoteexec"
	"github.com/TacoContent/ironstate/internal/remoteexec/protocol"
	"github.com/TacoContent/ironstate/internal/remoteexec/runstate"
)

const agentHelperEnv = "IRONSTATE_TEST_RUN_AGENT"

// TestMain lets the test binary act as the 'ironstate' agent when spawned
// by LocalTransport, so the end-to-end tests need no prebuilt binary.
func TestMain(m *testing.M) {
	if os.Getenv(agentHelperEnv) == "1" {
		if err := cli.Execute(); err != nil {
			os.Exit(cli.ExitCodeFor(err))
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runLocal(t *testing.T, spec remoteexec.JobSpec) (*remoteexec.HostResult, error) {
	t.Helper()
	job, err := remoteexec.Prepare(spec)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = job.Close() })
	return runPrepared(t, job, remoteexec.RunOptions{})
}

// runPrepared runs job through a local agent with a private state dir.
func runPrepared(t *testing.T, job *remoteexec.PreparedJob, opts remoteexec.RunOptions) (*remoteexec.HostResult, error) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if len(opts.AgentArgs) == 0 {
		opts.AgentArgs = []string{"--state-dir", t.TempDir()}
	}
	transport := remoteexec.LocalTransport{Env: []string{agentHelperEnv + "=1"}}
	return remoteexec.RunHost(context.Background(), transport, exe, job, opts)
}

func TestRunHostEndToEndOverLocalTransport(t *testing.T) {
	controllerDir := t.TempDir()
	playbook := filepath.Join(controllerDir, "pb")
	writeFile(t, filepath.Join(playbook, "site.yml"), `
vars:
  greeting: hello
tasks:
  - name: say
    log:
      message: "${{ vars.greeting }} from ${{ facts.platform }} token=${{ lookup('env', 'E2E_SECRET_TOKEN') }}"
  - name: check vars and env
    assert:
      that:
        - "greeting == 'override'"
        - "envs.E2E_PLAIN == 'plain-value'"
        - "extra == 'from-vars-file'"
  - name: included
    include:
      name: roles/r
`)
	writeFile(t, filepath.Join(playbook, "roles", "r", "main.yml"), `
tasks:
  - name: from role
    log:
      message: role ran
`)
	envFile := filepath.Join(controllerDir, ".env")
	writeFile(t, envFile, "E2E_PLAIN=plain-value\n")
	secretsFile := filepath.Join(controllerDir, ".secrets")
	writeFile(t, secretsFile, "E2E_SECRET_TOKEN=super-secret-123\n")
	varsFile := filepath.Join(controllerDir, "extra.yml")
	writeFile(t, varsFile, "vars:\n  extra: from-vars-file\n")

	result, err := runLocal(t, remoteexec.JobSpec{
		Playbook:     playbook,
		VarsFiles:    []string{varsFile},
		VarOverrides: []string{"greeting=override"},
		Apply:        true,
		EnvFile:      envFile,
		SecretsFile:  secretsFile,
	})
	if err != nil {
		t.Fatalf("RunHost: %v\nerrors: %v\nstderr: %s", err, result.Errors, result.Stderr)
	}
	if result.ExitCode != 0 || len(result.Errors) > 0 {
		t.Fatalf("exit=%d errors=%v stderr=%s", result.ExitCode, result.Errors, result.Stderr)
	}
	if result.Hello == nil || result.Hello.Version == "" || result.Hello.OS == "" {
		t.Fatalf("hello = %+v", result.Hello)
	}
	if result.Facts["platform"] == nil {
		t.Errorf("facts event missing platform: %v", result.Facts)
	}
	if result.Summary == nil || result.Summary.Stats.Total != 3 || result.Summary.Stats.Failed != 0 {
		t.Fatalf("summary = %+v\nlogs = %+v\nstderr = %s", result.Summary.Stats, result.Logs, result.Stderr)
	}
	if len(result.Results) != 3 {
		t.Fatalf("got %d leaf results, want 3", len(result.Results))
	}

	var logs []string
	for _, ev := range result.Logs {
		logs = append(logs, ev.Message)
	}
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "override from") || !strings.Contains(joined, "role ran") {
		t.Errorf("expected log messages missing:\n%s", joined)
	}
	if strings.Contains(joined, "super-secret-123") || !strings.Contains(joined, "token=***") {
		t.Errorf("secret not redacted in logs:\n%s", joined)
	}
	if _, err := os.Stat(filepath.Join(playbook, "..", "work")); err == nil {
		t.Error("agent wrote into the controller's playbook tree")
	}
}

func TestRunHostReportsFailedRun(t *testing.T) {
	playbook := filepath.Join(t.TempDir(), "site.yml")
	writeFile(t, playbook, `
tasks:
  - name: boom
    fail:
      message: nope
  - name: never
    log:
      message: unreachable
`)
	result, err := runLocal(t, remoteexec.JobSpec{Playbook: playbook, Apply: true, EnvFile: "-none-", SecretsFile: "-none-"})
	if err != nil {
		t.Fatalf("RunHost: %v (stderr: %s)", err, result.Stderr)
	}
	if !result.Completed || result.ExitCode != 1 {
		t.Fatalf("completed=%v exit=%d, want completed exit 1", result.Completed, result.ExitCode)
	}
	if len(result.Errors) == 0 {
		t.Error("expected an error event for the stopped run")
	}
	if result.Summary == nil || !result.Summary.Stopped || result.Summary.Stats.Failed != 1 {
		t.Fatalf("summary = %+v", result.Summary)
	}
}

func TestRunHostReportsLoadError(t *testing.T) {
	playbook := filepath.Join(t.TempDir(), "site.yml")
	writeFile(t, playbook, "tasks: [ this is: not valid yaml\n")
	result, err := runLocal(t, remoteexec.JobSpec{Playbook: playbook, EnvFile: "-none-", SecretsFile: "-none-"})
	if err != nil {
		t.Fatalf("RunHost: %v", err)
	}
	if result.ExitCode != 2 || len(result.Errors) == 0 {
		t.Fatalf("exit=%d errors=%v, want load error exit 2", result.ExitCode, result.Errors)
	}
}

func TestRunHostDetectsTamperedBundle(t *testing.T) {
	playbook := filepath.Join(t.TempDir(), "site.yml")
	writeFile(t, playbook, "tasks: []\n")
	job, err := remoteexec.Prepare(remoteexec.JobSpec{Playbook: playbook, EnvFile: "-none-", SecretsFile: "-none-"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = job.Close() }()
	job.Job.Bundle.SHA256 = strings.Repeat("0", 64)
	result, err := runPrepared(t, job, remoteexec.RunOptions{})
	if err != nil {
		t.Fatalf("RunHost: %v", err)
	}
	if result.ExitCode != 2 || len(result.Errors) == 0 || !strings.Contains(result.Errors[0], "sha256 mismatch") || result.FailedPhase != "bundle" {
		t.Fatalf("exit=%d phase=%q errors=%v, want bundle sha256 mismatch", result.ExitCode, result.FailedPhase, result.Errors)
	}
}

func TestRunHostStreamsEventsInOrder(t *testing.T) {
	playbook := filepath.Join(t.TempDir(), "site.yml")
	writeFile(t, playbook, "tasks:\n  - name: one\n    log:\n      message: hi\n")
	job, err := remoteexec.Prepare(remoteexec.JobSpec{Playbook: playbook, Apply: true, EnvFile: "-none-", SecretsFile: "-none-"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = job.Close() }()
	var types []string
	_, err = runPrepared(t, job, remoteexec.RunOptions{OnEvent: func(ev protocol.Event) {
		types = append(types, ev.Type)
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(types) < 4 || types[0] != protocol.TypeHello || types[len(types)-1] != protocol.TypeDone || types[len(types)-2] != protocol.TypeSummary {
		t.Fatalf("event order = %v", types)
	}
}

func TestRunHostCancelStopsBeforeNextLeafAndWritesRunLog(t *testing.T) {
	playbook := filepath.Join(t.TempDir(), "site.yml")
	writeFile(t, playbook, "tasks:\n  - name: one\n    log:\n      message: hi\n  - name: two\n    log:\n      message: there\n")
	job, err := remoteexec.Prepare(remoteexec.JobSpec{Playbook: playbook, Apply: true, EnvFile: "-none-", SecretsFile: "-none-"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = job.Close() }()
	cancel := make(chan struct{})
	close(cancel)
	stateDir := t.TempDir()
	result, err := runPrepared(t, job, remoteexec.RunOptions{Cancel: cancel, AgentArgs: []string{"--state-dir", stateDir}})
	if err != nil {
		t.Fatalf("RunHost: %v (stderr %s)", err, result.Stderr)
	}
	if result.ExitCode != 1 || len(result.Results) != 0 || result.Summary == nil {
		t.Fatalf("exit=%d results=%d summary=%v errors=%v, want cancelled before any leaf", result.ExitCode, len(result.Results), result.Summary, result.Errors)
	}
	if len(result.Errors) == 0 || !strings.Contains(result.Errors[0], "cancelled by controller") {
		t.Errorf("errors = %v, want cancellation reason", result.Errors)
	}
	if result.Hello == nil || result.Hello.RunLog == "" {
		t.Fatalf("hello missing run log: %+v", result.Hello)
	}
	data, err := os.ReadFile(result.Hello.RunLog)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if last, ok := protocol.ParseEvent([]byte(lines[len(lines)-1])); !ok || last.Type != protocol.TypeDone {
		t.Fatalf("run log does not end with done: %q", lines[len(lines)-1])
	}
}

func TestRunHostRefusesConcurrentRunOnSameTarget(t *testing.T) {
	stateDir := t.TempDir()
	lock, err := runstate.Acquire(stateDir, "other-run")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Release() }()

	playbook := filepath.Join(t.TempDir(), "site.yml")
	writeFile(t, playbook, "tasks: []\n")
	job, err := remoteexec.Prepare(remoteexec.JobSpec{Playbook: playbook, EnvFile: "-none-", SecretsFile: "-none-"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = job.Close() }()
	result, err := runPrepared(t, job, remoteexec.RunOptions{AgentArgs: []string{"--state-dir", stateDir}})
	if err != nil {
		t.Fatalf("RunHost: %v", err)
	}
	if result.FailedPhase != "lock" || result.ExitCode != 2 {
		t.Fatalf("phase=%q exit=%d errors=%v, want lock failure", result.FailedPhase, result.ExitCode, result.Errors)
	}
	if runtime.GOOS != "windows" && !strings.Contains(result.Errors[0], "other-run") {
		t.Errorf("lock error should name the holding run: %v", result.Errors)
	}
}
