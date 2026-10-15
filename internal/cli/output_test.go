package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/internal/remoteexec/protocol"
)

func TestRunApplyNDJSONOutputIsPureEventStream(t *testing.T) {
	sitePath := filepath.Join(t.TempDir(), "main.yml")
	content := "tasks:\n  - name: hello\n    log:\n      message: hi there\n"
	if err := os.WriteFile(sitePath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd, err := newRootCommand()
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd.SetArgs([]string{"--playbook", sitePath, "--output", "ndjson", "--apply"})
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	var types []string
	var sawMessage bool
	for _, line := range lines {
		ev, ok := protocol.ParseEvent([]byte(line))
		if !ok {
			t.Fatalf("non-event line on stdout: %q", line)
		}
		types = append(types, ev.Type)
		if ev.Type == protocol.TypeLog && ev.Message == "hi there" {
			sawMessage = true
		}
		if strings.Contains(line, "\x1b[") {
			t.Errorf("ANSI escape in event: %q", line)
		}
	}
	if !sawMessage {
		t.Errorf("log message event missing: %v", types)
	}
	if types[len(types)-1] != protocol.TypeDone || types[len(types)-2] != protocol.TypeSummary {
		t.Fatalf("event order = %v, want ... summary, done", types)
	}
}

func TestRunApplyRejectsUnknownOutput(t *testing.T) {
	cmd, err := newRootCommand()
	if err != nil {
		t.Fatal(err)
	}
	cmd.SetArgs([]string{"--playbook", t.TempDir(), "--output", "xml"})
	cmd.SetOut(new(bytes.Buffer))
	if err := cmd.Execute(); ExitCodeFor(err) != 2 {
		t.Fatalf("err = %v, want load error", err)
	}
}
