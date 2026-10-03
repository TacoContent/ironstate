package remoteexec_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/internal/remoteexec"
)

// TestSSHIntegration runs against a real sshd (see scripts/ssh-integration.sh):
//
//	IRONSTATE_SSH_TEST_TARGET  target, e.g. an alias from the ssh config
//	IRONSTATE_SSH_TEST_CONFIG  ssh config file (ssh -F)
//	IRONSTATE_SSH_TEST_AGENT   os/arch=path of a real ironstate binary for the target
func TestSSHIntegration(t *testing.T) {
	target := os.Getenv("IRONSTATE_SSH_TEST_TARGET")
	if target == "" {
		t.Skip("IRONSTATE_SSH_TEST_TARGET not set; run scripts/ssh-integration.sh")
	}
	platform, agentPath, ok := strings.Cut(os.Getenv("IRONSTATE_SSH_TEST_AGENT"), "=")
	if !ok {
		t.Fatal("IRONSTATE_SSH_TEST_AGENT must be os/arch=path")
	}
	opts := remoteexec.HostOptions{
		SSH:    remoteexec.SSHOptions{ConfigFile: os.Getenv("IRONSTATE_SSH_TEST_CONFIG")},
		Agents: &remoteexec.AgentSource{Version: "integration", Binaries: map[string]string{platform: agentPath}},
	}
	ctx := context.Background()

	ping := remoteexec.PingHost(ctx, target, opts)
	if ping.Status != remoteexec.StatusOK {
		t.Fatalf("ping: status=%s err=%v", ping.Status, ping.Err)
	}
	if ping.Platform != platform {
		t.Fatalf("ping platform = %s, want %s", ping.Platform, platform)
	}

	dir := t.TempDir()
	writeFile(t, dir+"/site.yml", `
vars:
  greeting: hello
tasks:
  - name: greet
    log:
      message: "${{ vars.greeting }} from ${{ facts.computer_name }} token=${{ lookup('env', 'SSH_IT_SECRET') }}"
  - name: check
    assert:
      that:
        - "facts.platform == 'linux'"
`)
	writeFile(t, dir+"/.secrets", "SSH_IT_SECRET=very-secret-value\n")
	job, err := remoteexec.Prepare(remoteexec.JobSpec{Playbook: dir, Apply: true, EnvFile: dir + "/.env", SecretsFile: dir + "/.secrets"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = job.Close() }()

	report := remoteexec.ApplyHost(ctx, target, job, opts)
	if report.Status != remoteexec.StatusOK {
		var stderr string
		if report.Result != nil {
			stderr = report.Result.Stderr
		}
		t.Fatalf("apply: status=%s err=%v stderr=%s", report.Status, report.Err, stderr)
	}
	if report.Uploaded {
		t.Error("agent re-uploaded although ping just installed it")
	}
	var logs []string
	for _, ev := range report.Result.Logs {
		logs = append(logs, ev.Message)
	}
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "hello from") || strings.Contains(joined, "very-secret-value") || !strings.Contains(joined, "token=***") {
		t.Fatalf("unexpected logs:\n%s", joined)
	}
	if report.Result.Hello == nil || report.Result.Hello.RunLog == "" {
		t.Fatal("hello without run log path")
	}
}
