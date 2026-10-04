package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/internal/remoteexec/protocol"
)

const agentHelperEnv = "IRONSTATE_TEST_RUN_AGENT"

// TestMain lets this test binary serve as the agent for '--target local'.
func TestMain(m *testing.M) {
	if os.Getenv(agentHelperEnv) == "1" {
		if err := Execute(); err != nil {
			os.Exit(ExitCodeFor(err))
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// localTargetEnv makes a '--target local' child act as the agent, with its
// state under a temp dir instead of the real user cache.
func localTargetEnv(t *testing.T) {
	t.Helper()
	t.Setenv(agentHelperEnv, "1")
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("LOCALAPPDATA", cache)
	t.Setenv("HOME", cache)
}

func runRemoteCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd, err := newRootCommand()
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd.SetArgs(args)
	cmd.SetOut(&out)
	cmd.SetErr(new(bytes.Buffer))
	err = cmd.Execute()
	return out.String(), err
}

func writePlaybook(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "site.yml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRemoteApplyLocalTargetJSON(t *testing.T) {
	localTargetEnv(t)
	playbook := writePlaybook(t, "tasks:\n  - name: hi\n    log:\n      message: hello\n")
	out, err := runRemoteCLI(t, "--playbook", playbook, "--target", "local", "--output", "json", "--apply")
	if err != nil {
		t.Fatalf("Execute: %v\n%s", err, out)
	}
	var doc struct {
		RunID string `json:"run_id"`
		Hosts []struct {
			Name    string            `json:"name"`
			Status  string            `json:"status"`
			Results []json.RawMessage `json:"results"`
			Facts   map[string]any    `json:"facts"`
		} `json:"hosts"`
		Stats struct {
			Hosts, OK int
		} `json:"stats"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out)
	}
	if doc.RunID == "" || len(doc.Hosts) != 1 || doc.Hosts[0].Status != "ok" || len(doc.Hosts[0].Results) != 1 || doc.Hosts[0].Facts["platform"] == nil {
		t.Fatalf("doc = %+v", doc)
	}
}

func TestRemoteApplyFailedHostExitsOne(t *testing.T) {
	localTargetEnv(t)
	playbook := writePlaybook(t, "tasks:\n  - name: boom\n    fail:\n      message: nope\n")
	out, err := runRemoteCLI(t, "--playbook", playbook, "--target", "local", "--apply")
	if ExitCodeFor(err) != 1 {
		t.Fatalf("exit = %d (%v), want 1\n%s", ExitCodeFor(err), err, out)
	}
	if !strings.Contains(out, "HOST") || !strings.Contains(out, "failed") {
		t.Fatalf("table output missing host summary:\n%s", out)
	}
}

func TestRemoteApplyNDJSONTagsEventsWithHost(t *testing.T) {
	localTargetEnv(t)
	playbook := writePlaybook(t, "tasks:\n  - name: hi\n    log:\n      message: hello\n")
	out, err := runRemoteCLI(t, "--playbook", playbook, "--target", "local", "--output", "ndjson", "--apply")
	if err != nil {
		t.Fatalf("Execute: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines[:len(lines)-1] {
		ev, ok := protocol.ParseEvent([]byte(line))
		if !ok || ev.Host != "local" {
			t.Fatalf("event without host: %s", line)
		}
	}
	last, ok := protocol.ParseEvent([]byte(lines[len(lines)-1]))
	if !ok || last.Type != protocol.TypeDone || last.Host != "" || *last.ExitCode != 0 {
		t.Fatalf("final line = %s, want run-level done(0)", lines[len(lines)-1])
	}
}

func TestRemoteApplyRejectsInvalidTarget(t *testing.T) {
	playbook := writePlaybook(t, "tasks: []\n")
	for _, args := range [][]string{
		{"--target", "-oProxyCommand=evil"},
		{"--target", "local", "--limit", "web"},
		{"--inventory", filepath.Join(t.TempDir(), "missing.yml")},
	} {
		_, err := runRemoteCLI(t, append([]string{"--playbook", playbook}, args...)...)
		if ExitCodeFor(err) != 2 {
			t.Errorf("%v: exit = %d (%v), want 2", args, ExitCodeFor(err), err)
		}
	}
}

func TestRemoteApplyInventoryWithLimit(t *testing.T) {
	localTargetEnv(t)
	inventory := filepath.Join(t.TempDir(), "inventory.yml")
	content := "hosts:\n  one: { address: local }\n  two: { address: local }\n  skipped: { address: local }\ngroups:\n  pair: [one, two]\n"
	if err := os.WriteFile(inventory, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	playbook := writePlaybook(t, "tasks:\n  - name: hi\n    log:\n      message: hello\n")
	// forks 1: every host here is this machine, which allows one apply at a time.
	out, err := runRemoteCLI(t, "--playbook", playbook, "--inventory", inventory, "--limit", "pair", "--forks", "1", "--output", "json", "--apply")
	if err != nil {
		t.Fatalf("Execute: %v\n%s", err, out)
	}
	var doc struct {
		Hosts []struct {
			Name, Address, Status string
		} `json:"hosts"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Hosts) != 2 || doc.Hosts[0].Name != "one" || doc.Hosts[1].Name != "two" || doc.Hosts[0].Status != "ok" || doc.Hosts[1].Status != "ok" || doc.Hosts[0].Address != "local" {
		t.Fatalf("hosts = %+v", doc.Hosts)
	}
}

func TestRemotePingLocal(t *testing.T) {
	localTargetEnv(t)
	out, err := runRemoteCLI(t, "remote", "ping", "--target", "local")
	if err != nil || !strings.Contains(out, "ok") || !strings.Contains(out, "ironstate") {
		t.Fatalf("ping: %v\n%s", err, out)
	}
}
