package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/internal/engine"
	"github.com/TacoContent/ironstate/internal/remoteexec/protocol"
)

func silentShellCommand() (string, string) {
	if runtime.GOOS == "windows" {
		return "cmd", "echo quiet-stdout-marker & echo quiet-stderr-marker 1>&2"
	}
	return "sh", "printf 'quiet-stdout-marker\\n'; printf 'quiet-stderr-marker\\n' >&2"
}

func TestSilentShellOnlySuppressesExecutionLogsAcrossOutputFormats(t *testing.T) {
	for _, format := range []string{"table", "json", "ndjson"} {
		t.Run(format, func(t *testing.T) {
			var logs bytes.Buffer
			oldInfo, oldWarn, oldDanger := engine.Info, engine.Warn, engine.Danger
			logger := func(format string, args ...any) { fmt.Fprintf(&logs, format+"\n", args...) }
			engine.Info, engine.Warn, engine.Danger = logger, logger, logger
			t.Cleanup(func() { engine.Info, engine.Warn, engine.Danger = oldInfo, oldWarn, oldDanger })
			host, command := silentShellCommand()
			probeCommand := command + "; exit 1"
			if runtime.GOOS == "windows" {
				probeCommand = command + " & exit /b 1"
			}
			path := filepath.Join(t.TempDir(), "main.yml")
			content := fmt.Sprintf(`
envs:
  IRONSTATE_QUIET_SOURCE_TEST:
    valueFrom:
      task:
        shell:
          type: %s
          command: %q
          silent: true
        value: "${{ stdout | trim }}"
tasks:
  - name: quiet-shell-label
    id: quiet_result
    shell:
      type: %s
      command: %q
      silent: true
  - name: quiet-fact-label
    fact:
      name: quiet_fact
      shell:
        type: %s
        command: %q
        silent: true
      value: "${{ stdout | trim }}"
  - name: quiet-probe-label
    failed_when: false
    fact:
      name: quiet_probe
      shell:
        type: %s
        command: %q
        silent: true
      value: "${{ rc == 0 }}"
  - name: visible-consumer
    assert:
      that:
        - quiet_result.rc == 0
        - (quiet_result.stdout | trim) == facts.quiet_fact
        - envs.IRONSTATE_QUIET_SOURCE_TEST == facts.quiet_fact
        - facts.quiet_probe == false
`, host, command, host, command, host, command, host, probeCommand)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			cmd, err := newRootCommand()
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetArgs([]string{"--playbook", path, "--apply", "--verbose", "--output", format})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute: %v\n%s\n%s", err, logs.String(), output.String())
			}
			if format == "ndjson" {
				starts, results := 0, 0
				factDisplayed := false
				for _, line := range bytes.Split(output.Bytes(), []byte("\n")) {
					if len(line) == 0 {
						continue
					}
					event, ok := protocol.ParseEvent(line)
					if !ok {
						t.Fatalf("invalid NDJSON event: %s", line)
					}
					switch event.Type {
					case protocol.TypeLog:
						logs.WriteString(event.Message + "\n")
					case protocol.TypeLeafStart:
						starts++
					case protocol.TypeLeafResult:
						results++
					case protocol.TypeFacts:
						factDisplayed = event.Facts["quiet_fact"] == "quiet-stdout-marker"
					}
				}
				if starts != 4 || results != 4 || !factDisplayed {
					t.Fatalf("silent tasks lost progress, results or facts: starts=%d results=%d fact=%v", starts, results, factDisplayed)
				}
			}
			for _, line := range strings.Split(logs.String(), "\n") {
				for _, hidden := range []string{"quiet-stdout-marker", "quiet-stderr-marker"} {
					if strings.TrimSpace(line) == hidden {
						t.Fatalf("silent execution output logged %q:\n%s", hidden, logs.String())
					}
				}
			}
			if strings.Contains(logs.String(), "exited with code") {
				t.Fatalf("handled probe failure logged an exit-code warning:\n%s", logs.String())
			}
			for _, label := range []string{"quiet-shell-label", "quiet-fact-label", "visible-consumer"} {
				completed := false
				for _, line := range strings.Split(logs.String(), "\n") {
					if strings.Contains(line, "installed") && strings.Contains(line, label) {
						completed = true
					}
				}
				if !completed {
					t.Fatalf("task lifecycle log missing for %q:\n%s", label, logs.String())
				}
				if !strings.Contains(output.String(), label) {
					t.Fatalf("result was hidden for %q:\n%s", label, output.String())
				}
			}
			if !strings.Contains(output.String(), "quiet-probe-label") {
				t.Fatalf("handled probe result was hidden:\n%s", output.String())
			}
			if format == "table" && !strings.Contains(output.String(), "quiet_fact") {
				t.Fatalf("silent fact was hidden from facts table:\n%s", output.String())
			}
			if format != "table" && (!strings.Contains(output.String(), "quiet-stdout-marker") || !strings.Contains(output.String(), "quiet-stderr-marker")) {
				t.Fatalf("structured results lost captured output:\n%s", output.String())
			}
		})
	}
}

func TestSilentShellFailuresStillLogCapturedOutput(t *testing.T) {
	for _, failedWhen := range []bool{false, true} {
		t.Run(fmt.Sprintf("failed_when_override=%v", failedWhen), func(t *testing.T) {
			var logs bytes.Buffer
			oldInfo, oldWarn, oldDanger := engine.Info, engine.Warn, engine.Danger
			logger := func(format string, args ...any) { fmt.Fprintf(&logs, format+"\n", args...) }
			engine.Info, engine.Warn, engine.Danger = logger, logger, logger
			t.Cleanup(func() { engine.Info, engine.Warn, engine.Danger = oldInfo, oldWarn, oldDanger })
			host, command := silentShellCommand()
			condition := ""
			if failedWhen {
				condition = "failed_when: true"
			} else if runtime.GOOS == "windows" {
				command += " & exit /b 7"
			} else {
				command += "; exit 7"
			}
			path := filepath.Join(t.TempDir(), "main.yml")
			content := fmt.Sprintf(`
tasks:
  - name: visible-failure
    %s
    shell:
      type: %s
      command: %q
      silent: true
`, condition, host, command)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			cmd, err := newRootCommand()
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetArgs([]string{"--playbook", path, "--apply", "--output", "json"})
			if err := cmd.Execute(); err == nil {
				t.Fatal("expected the shell failure to stop the run")
			}
			for _, diagnostic := range []string{"quiet-stdout-marker", "quiet-stderr-marker", "visible-failure"} {
				if !strings.Contains(logs.String(), diagnostic) {
					t.Fatalf("failure log lost %q: %s", diagnostic, logs.String())
				}
			}
			if !strings.Contains(output.String(), `"failed": true`) || !strings.Contains(output.String(), "visible-failure") {
				t.Fatalf("failed result was hidden: %s", output.String())
			}
		})
	}
}
