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
	default:
		f.t.Fatalf("unexpected script %q", script)
	}
	return 0, nil
}

func (f *fakeTarget) Close() error { return nil }

// hostOptions ships the test binary itself as the agent for the fake's
// platform; the fake runs it locally.
func hostOptions(f *fakeTarget) remoteexec.HostOptions {
	exe, err := os.Executable()
	if err != nil {
		f.t.Fatal(err)
	}
	return remoteexec.HostOptions{
		Agents: &remoteexec.AgentSource{Version: "test", Binaries: map[string]string{f.platform: exe}},
		Dial:   func(string) (remoteexec.Transport, error) { return f, nil },
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

	first := remoteexec.ApplyHost(context.Background(), "u@box", job, hostOptions(target))
	if first.Status != remoteexec.StatusOK || !first.Uploaded {
		t.Fatalf("first run: status=%s uploaded=%v err=%v", first.Status, first.Uploaded, first.Err)
	}
	second := remoteexec.ApplyHost(context.Background(), "u@box", job, hostOptions(target))
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
	report := remoteexec.ApplyHost(context.Background(), "box", prepareSimple(t, "tasks: []\n"), hostOptions(target))
	if report.Status != remoteexec.StatusUnreachable || !strings.Contains(report.Err.Error(), "Connection refused") {
		t.Fatalf("status=%s err=%v", report.Status, report.Err)
	}
	if code := remoteexec.ExitCode([]remoteexec.HostReport{report}); code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}
}

func TestApplyHostRejectsBecomeWithoutPasswordlessSudo(t *testing.T) {
	target := newFakeTarget(t, "password")
	job := prepareSimple(t, "tasks:\n  - name: root\n    become: true\n    log:\n      message: hi\n")
	report := remoteexec.ApplyHost(context.Background(), "box", job, hostOptions(target))
	if report.Status != remoteexec.StatusError || !strings.Contains(report.Err.Error(), "become") {
		t.Fatalf("status=%s err=%v", report.Status, report.Err)
	}
	if target.uploads != 0 {
		t.Error("agent uploaded despite failed preflight")
	}
}

func TestApplyHostRejectsUnsupportedFeatures(t *testing.T) {
	cases := map[string]string{
		"plugins":     "plugins:\n  - acme.tools@1.0.0\ntasks: []\n",
		"remote uses": "tasks:\n  - name: r\n    uses:\n      remote: https://github.com/acme/roles.git\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			report := remoteexec.ApplyHost(context.Background(), "box", prepareSimple(t, content), hostOptions(newFakeTarget(t, "nopasswd")))
			if report.Status != remoteexec.StatusError || !strings.Contains(report.Err.Error(), "preflight") {
				t.Fatalf("status=%s err=%v", report.Status, report.Err)
			}
		})
	}
}

func TestApplyHostNeedsAgentForOtherPlatform(t *testing.T) {
	target := newFakeTarget(t, "nopasswd")
	target.probeOut = strings.Replace(target.probeOut, "arch="+map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[runtime.GOARCH], "arch=riscv64", 1)
	report := remoteexec.ApplyHost(context.Background(), "box", prepareSimple(t, "tasks: []\n"), hostOptions(target))
	if report.Status != remoteexec.StatusError || !strings.Contains(report.Err.Error(), "unsupported target architecture") {
		t.Fatalf("status=%s err=%v", report.Status, report.Err)
	}

	other := "linux/arm64"
	if runtime.GOOS == "linux" && runtime.GOARCH == "arm64" {
		other = "linux/amd64"
	}
	_, err := (&remoteexec.AgentSource{Version: "test"}).Resolve(other)
	if err == nil || !strings.Contains(err.Error(), "--agent-binary "+other) {
		t.Fatalf("Resolve(%s) err = %v, want --agent-binary hint", other, err)
	}
}

func TestApplyHostRejectsNonPOSIXTarget(t *testing.T) {
	target := newFakeTarget(t, "nopasswd")
	target.probeOut = "'sh' is not recognized as an internal or external command\r\n"
	report := remoteexec.ApplyHost(context.Background(), "box", prepareSimple(t, "tasks: []\n"), hostOptions(target))
	if report.Status != remoteexec.StatusError || !strings.Contains(report.Err.Error(), "POSIX") {
		t.Fatalf("status=%s err=%v", report.Status, report.Err)
	}
}

func TestPingHostRunsAgentVersion(t *testing.T) {
	target := newFakeTarget(t, "nopasswd")
	report := remoteexec.PingHost(context.Background(), "box", hostOptions(target))
	if report.Status != remoteexec.StatusOK || !strings.Contains(report.AgentVersion, "ironstate") {
		t.Fatalf("status=%s version=%q err=%v", report.Status, report.AgentVersion, report.Err)
	}
	if report.Probe.Sudo != "nopasswd" {
		t.Errorf("probe sudo = %q", report.Probe.Sudo)
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
