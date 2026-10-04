package remoteexec_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TacoContent/ironstate/internal/remoteexec"
)

// fakeTarget simulates a POSIX target: it answers the probe/check/upload
// scripts itself and runs the agent locally via the test binary.
type fakeTarget struct {
	t        *testing.T
	probeOut string
	exitCode int // forced exit code for every command (e.g. 255)

	mu       sync.Mutex
	files    map[string]string // remote path -> sha256
	uploads  int
	stateDir string
	platform string
	logData  string
	cleaned  bool
}

func newFakeTarget(t *testing.T, sudo string) *fakeTarget {
	goos, arch := runtime.GOOS, runtime.GOARCH
	unameOS := map[string]string{"linux": "Linux", "darwin": "Darwin"}[goos]
	platformOS := goos
	if unameOS == "" {
		unameOS, platformOS = "Linux", "linux"
	}
	unameArch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[arch]
	return &fakeTarget{
		t: t,
		probeOut: fmt.Sprintf("motd noise\n__IRONSTATE_BEGIN__\nos=%s\narch=%s\nuid=1000\nagent_dir=/home/u/.cache/ironstate/agent\nsha=sha256sum\nsudo=%s\nexec=ok\n__IRONSTATE_END__\n",
			unameOS, unameArch, sudo),
		files:    map[string]string{},
		stateDir: t.TempDir(),
		platform: platformOS + "/" + arch,
	}
}

func (f *fakeTarget) Exec(ctx context.Context, cmd remoteexec.RemoteCommand, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	if f.exitCode != 0 {
		_, _ = io.WriteString(stderr, "ssh: connect to host x port 22: Connection refused")
		return f.exitCode, nil
	}
	if cmd.Program != "sh" {
		exe, err := os.Executable()
		if err != nil {
			return -1, err
		}
		args := cmd.Args
		if len(args) > 0 && args[0] == "agent" {
			args = append(args, "--state-dir", f.stateDir)
		}
		return remoteexec.LocalTransport{Env: []string{agentHelperEnv + "=1"}}.Exec(ctx, remoteexec.RemoteCommand{Program: exe, Args: args}, stdin, stdout, stderr)
	}
	script := cmd.Args[1]
	params := cmd.Args[3:]
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case strings.HasPrefix(script, ": ironstate-probe;"):
		_, _ = io.WriteString(stdout, f.probeOut)
	case strings.HasPrefix(script, ": ironstate-check;"):
		if sum, ok := f.files[params[0]]; ok {
			_, _ = fmt.Fprintf(stdout, "sha256=%s\n", sum)
		}
	case strings.HasPrefix(script, ": ironstate-upload;"):
		h := sha256.New()
		if _, err := io.Copy(h, stdin); err != nil {
			return 1, err
		}
		sum := hex.EncodeToString(h.Sum(nil))
		if sum != params[3] {
			_, _ = io.WriteString(stderr, "mismatch")
			return 3, nil
		}
		f.files[params[1]] = sum
		f.uploads++
		_, _ = fmt.Fprintf(stdout, "sha256=%s\n", sum)
	case strings.HasPrefix(script, ": ironstate-upload-unverified;"):
		h := sha256.New()
		if _, err := io.Copy(h, stdin); err != nil {
			return 1, err
		}
		f.files[params[1]] = hex.EncodeToString(h.Sum(nil))
		f.uploads++
		_, _ = io.WriteString(stdout, "uploaded=1\n")
	case strings.HasPrefix(script, `cat "$1"`):
		_, _ = io.WriteString(stdout, f.logData)
	case strings.HasPrefix(script, "latest="):
		_, _ = io.WriteString(stdout, "run-123")
	case strings.HasPrefix(script, "if ! command -v flock"):
		f.cleaned = true
	default:
		f.t.Fatalf("unexpected script %q", script)
	}
	return 0, nil
}

func (f *fakeTarget) Close() error { return nil }

type flakyTarget struct {
	target   *fakeTarget
	failures int
	failed   int
}

func (f *flakyTarget) Exec(ctx context.Context, cmd remoteexec.RemoteCommand, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	if f.failed < f.failures {
		f.failed++
		_, _ = io.WriteString(stderr, "ssh: connect to host x port 22: Connection refused")
		return remoteexec.SSHExitUnreachable, nil
	}
	return f.target.Exec(ctx, cmd, stdin, stdout, stderr)
}

func (f *flakyTarget) Close() error { return nil }

type blockingTarget struct{}

func (blockingTarget) Exec(ctx context.Context, _ remoteexec.RemoteCommand, _ io.Reader, _, _ io.Writer) (int, error) {
	<-ctx.Done()
	return -1, ctx.Err()
}

func (blockingTarget) Close() error { return nil }

var box = remoteexec.Host{Name: "box", SSH: remoteexec.SSHTarget{User: "u", Host: "box"}}

// hostOptions ships the test binary itself as the agent for the fake's
// platform; the fake runs it locally.
func hostOptions(f *fakeTarget) remoteexec.HostOptions {
	exe, err := os.Executable()
	if err != nil {
		f.t.Fatal(err)
	}
	return remoteexec.HostOptions{
		Agents: &remoteexec.AgentSource{Version: "test", Binaries: map[string]string{f.platform: exe}},
		Dial:   func(remoteexec.Host) (remoteexec.Transport, error) { return f, nil },
	}
}

func prepareSimple(t *testing.T, content string) *remoteexec.PreparedJob {
	t.Helper()
	playbook := filepath.Join(t.TempDir(), "site.yml")
	writeFile(t, playbook, content)
	job, err := remoteexec.Prepare(remoteexec.JobSpec{Playbook: playbook, Apply: true, EnvFile: "-none-", SecretsFile: "-none-"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = job.Close() })
	return job
}

func TestApplyHostUploadsOnceThenReusesCachedAgent(t *testing.T) {
	target := newFakeTarget(t, "nopasswd")
	job := prepareSimple(t, "tasks:\n  - name: hi\n    log:\n      message: hello\n")

	first := remoteexec.ApplyHost(context.Background(), box, job, hostOptions(target))
	if first.Status != remoteexec.StatusOK || !first.Uploaded {
		t.Fatalf("first run: status=%s uploaded=%v err=%v", first.Status, first.Uploaded, first.Err)
	}
	second := remoteexec.ApplyHost(context.Background(), box, job, hostOptions(target))
	if second.Status != remoteexec.StatusOK || second.Uploaded {
		t.Fatalf("second run: status=%s uploaded=%v err=%v", second.Status, second.Uploaded, second.Err)
	}
	if target.uploads != 1 {
		t.Fatalf("uploads = %d, want 1", target.uploads)
	}
}

func TestApplyHostUnreachable(t *testing.T) {
	target := newFakeTarget(t, "nopasswd")
	target.exitCode = remoteexec.SSHExitUnreachable
	report := remoteexec.ApplyHost(context.Background(), box, prepareSimple(t, "tasks: []\n"), hostOptions(target))
	if report.Status != remoteexec.StatusUnreachable || !strings.Contains(report.Err.Error(), "Connection refused") {
		t.Fatalf("status=%s err=%v", report.Status, report.Err)
	}
	if code := remoteexec.ExitCode([]remoteexec.HostReport{report}); code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}
}

func TestPingHostRetriesTransientBootstrapFailures(t *testing.T) {
	target := newFakeTarget(t, "nopasswd")
	flaky := &flakyTarget{target: target, failures: 2}
	opts := hostOptions(target)
	opts.Dial = func(remoteexec.Host) (remoteexec.Transport, error) { return flaky, nil }
	report := remoteexec.PingHost(context.Background(), box, opts)
	if report.Status != remoteexec.StatusOK || flaky.failed != 2 {
		t.Fatalf("status=%s failed attempts=%d err=%v", report.Status, flaky.failed, report.Err)
	}
}

func TestPingHostTimeoutCancelsProbe(t *testing.T) {
	opts := remoteexec.HostOptions{
		HostTimeout: 20 * time.Millisecond,
		Dial:        func(remoteexec.Host) (remoteexec.Transport, error) { return blockingTarget{}, nil },
	}
	started := time.Now()
	report := remoteexec.PingHost(context.Background(), box, opts)
	if report.Err == nil || !strings.Contains(report.Err.Error(), "deadline exceeded") {
		t.Fatalf("status=%s err=%v", report.Status, report.Err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("host timeout took %s", elapsed)
	}
}

func TestApplyHostRejectsBecomeWithoutPasswordlessSudo(t *testing.T) {
	target := newFakeTarget(t, "password")
	job := prepareSimple(t, "tasks:\n  - name: root\n    become: true\n    log:\n      message: hi\n")
	report := remoteexec.ApplyHost(context.Background(), box, job, hostOptions(target))
	if report.Status != remoteexec.StatusError || !strings.Contains(report.Err.Error(), "become") {
		t.Fatalf("status=%s err=%v", report.Status, report.Err)
	}
	if target.uploads != 0 {
		t.Error("agent uploaded despite failed preflight")
	}
}

func TestApplyHostDisablesBecomeWhenInventorySaysFalse(t *testing.T) {
	target := newFakeTarget(t, "missing")
	host := box
	host.DisableBecome = true
	job := prepareSimple(t, "tasks:\n  - name: current user\n    become: true\n    log:\n      message: runs without sudo\n")
	report := remoteexec.ApplyHost(context.Background(), host, job, hostOptions(target))
	if report.Status != remoteexec.StatusOK {
		t.Fatalf("status=%s err=%v", report.Status, report.Err)
	}
}

func TestApplyHostSkipsAgentVerificationForTrustedHost(t *testing.T) {
	target := newFakeTarget(t, "missing")
	target.probeOut = strings.Replace(target.probeOut, "sha=sha256sum\n", "", 1)
	host := box
	host.DisableBecome = true
	host.SkipAgentVerification = true
	job := prepareSimple(t, "tasks:\n  - name: current user\n    become: true\n    log:\n      message: runs without sudo\n")
	report := remoteexec.ApplyHost(context.Background(), host, job, hostOptions(target))
	if report.Status != remoteexec.StatusOK || !report.VerificationSkipped || !report.Uploaded {
		t.Fatalf("status=%s verificationSkipped=%t uploaded=%t err=%v", report.Status, report.VerificationSkipped, report.Uploaded, report.Err)
	}
}

func TestApplyHostRejectsUnsupportedFeatures(t *testing.T) {
	cases := map[string]string{
		"plugins":     "plugins:\n  - acme.tools@1.0.0\ntasks: []\n",
		"remote uses": "tasks:\n  - name: r\n    uses:\n      remote: https://github.com/acme/roles.git\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			report := remoteexec.ApplyHost(context.Background(), box, prepareSimple(t, content), hostOptions(newFakeTarget(t, "nopasswd")))
			if report.Status != remoteexec.StatusError || !strings.Contains(report.Err.Error(), "preflight") {
				t.Fatalf("status=%s err=%v", report.Status, report.Err)
			}
		})
	}
}

func TestApplyHostNeedsAgentForOtherPlatform(t *testing.T) {
	target := newFakeTarget(t, "nopasswd")
	target.probeOut = strings.Replace(target.probeOut, "arch="+map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[runtime.GOARCH], "arch=riscv64", 1)
	report := remoteexec.ApplyHost(context.Background(), box, prepareSimple(t, "tasks: []\n"), hostOptions(target))
	if report.Status != remoteexec.StatusError || !strings.Contains(report.Err.Error(), "unsupported target architecture") {
		t.Fatalf("status=%s err=%v", report.Status, report.Err)
	}

	other := "linux/arm64"
	if runtime.GOOS == "linux" && runtime.GOARCH == "arm64" {
		other = "linux/amd64"
	}
	_, err := (&remoteexec.AgentSource{Version: "test"}).Resolve(context.Background(), other)
	if err == nil || !strings.Contains(err.Error(), "--agent-binary "+other) {
		t.Fatalf("Resolve(%s) err = %v, want --agent-binary hint", other, err)
	}
}

type windowsProbeTransport struct{}

func (windowsProbeTransport) Exec(_ context.Context, cmd remoteexec.RemoteCommand, _ io.Reader, stdout, _ io.Writer) (int, error) {
	if !cmd.Windows || cmd.Script == "" {
		return 1, fmt.Errorf("expected encoded PowerShell command")
	}
	_, _ = io.WriteString(stdout, "__IRONSTATE_BEGIN__\nos=windows\narch=AMD64\nuid=deploy\nagent_dir=C:\\Users\\deploy\\AppData\\Local\\ironstate\\agent\nsha=powershell\nsudo=na\nexec=ok\nadmin=yes\n__IRONSTATE_END__\n")
	return 0, nil
}

func (windowsProbeTransport) Close() error { return nil }

func TestProbeWindowsHintUsesPowerShell(t *testing.T) {
	info, err := remoteexec.ProbeWithHint(context.Background(), windowsProbeTransport{}, "", "windows")
	if err != nil {
		t.Fatal(err)
	}
	if info.Platform() != "windows/amd64" || !info.Admin {
		t.Fatalf("probe info = %+v", info)
	}
}

func TestPingHostRunsAgentVersion(t *testing.T) {
	target := newFakeTarget(t, "nopasswd")
	report := remoteexec.PingHost(context.Background(), box, hostOptions(target))
	if report.Status != remoteexec.StatusOK || !strings.Contains(report.AgentVersion, "ironstate") {
		t.Fatalf("status=%s version=%q err=%v", report.Status, report.AgentVersion, report.Err)
	}
	if report.Probe.Sudo != "nopasswd" {
		t.Errorf("probe sudo = %q", report.Probe.Sudo)
	}
}

func TestReadHostLogsRestoresEvents(t *testing.T) {
	target := newFakeTarget(t, "nopasswd")
	target.logData = "{\"v\":1,\"type\":\"hello\",\"run_id\":\"run-123\"}\n{\"v\":1,\"type\":\"done\",\"exit_code\":0}\n"
	result, err := remoteexec.ReadHostLogs(context.Background(), box, hostOptions(target), "run-123")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Completed || result.Hello == nil || !strings.Contains(result.RawLog, `"type":"done"`) {
		t.Fatalf("recovered result = %+v", result)
	}
}

func TestCleanHostRemovesStateUnderLock(t *testing.T) {
	target := newFakeTarget(t, "nopasswd")
	if err := remoteexec.CleanHost(context.Background(), box, hostOptions(target)); err != nil {
		t.Fatal(err)
	}
	if !target.cleaned {
		t.Fatal("remote cleanup script was not run")
	}
}

func TestExitCodePrecedence(t *testing.T) {
	r := func(s string) remoteexec.HostReport { return remoteexec.HostReport{Status: s} }
	cases := []struct {
		reports []remoteexec.HostReport
		want    int
	}{
		{[]remoteexec.HostReport{r("ok"), r("ok")}, 0},
		{[]remoteexec.HostReport{r("ok"), r("unreachable")}, 3},
		{[]remoteexec.HostReport{r("error"), r("failed"), r("unreachable")}, 1},
	}
	for _, c := range cases {
		if got := remoteexec.ExitCode(c.reports); got != c.want {
			t.Errorf("ExitCode(%v) = %d, want %d", c.reports, got, c.want)
		}
	}
}
