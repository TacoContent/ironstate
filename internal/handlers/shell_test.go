package handlers

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/internal/engine"
	ironexec "github.com/TacoContent/ironstate/internal/exec"
	"github.com/TacoContent/ironstate/internal/filters"
	"github.com/TacoContent/ironstate/internal/tasks"
)

func TestPowerShellInlineExitStatus(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh is not installed")
	}
	original := runner
	runner = ironexec.Default
	t.Cleanup(func() { runner = original })
	for _, host := range []string{"pwsh", "powershell", pwsh} {
		t.Run(host, func(t *testing.T) {
			for _, tc := range []struct {
				name, command string
				args          []string
				rc            int
				stdout        string
			}{
				{"missing command", "Get-Command ironstate_missing_command_94d0 -ErrorAction SilentlyContinue", nil, 1, ""},
				{"existing command", "Get-Command Get-Command | Select-Object -ExpandProperty Name", nil, 0, "Get-Command\n"},
				{"explicit failure", "exit 7", nil, 7, ""},
				{"explicit success", "Get-Command ironstate_missing_command_94d0 -ErrorAction SilentlyContinue; exit 0", nil, 0, ""},
				{"terminating error", "throw 'test failure'", nil, 1, ""},
				{"native failure", "& " + pwshSingleQuote(pwsh) + " -NoProfile -Command 'exit 9'", nil, 9, ""},
				{"handled error", "Get-Command ironstate_missing_command_94d0 -ErrorAction SilentlyContinue; Write-Output recovered", nil, 0, "recovered\n"},
				{"arguments", "param([string]$Value)\nWrite-Output $Value", []string{"space ' apostrophe $value; [literal]"}, 0, "space ' apostrophe $value; [literal]\n"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					result := invokeShellItem(shellStateConfig{HostSpec: host, Command: tc.command, ItemArgs: tc.args}, tc.name)
					if result.RC != tc.rc || strings.ReplaceAll(result.Stdout, "\r\n", "\n") != tc.stdout {
						t.Fatalf("result = %#v, want rc=%d stdout=%q", result, tc.rc, tc.stdout)
					}
				})
			}
		})
	}
}

func TestMissingPowerShellCommandSetsFalseFact(t *testing.T) {
	if _, err := exec.LookPath("pwsh"); err != nil {
		t.Skip("pwsh is not installed")
	}
	original := runner
	runner = ironexec.Default
	t.Cleanup(func() { runner = original })
	for _, tc := range []struct {
		name, command string
		want          bool
	}{
		{"missing", "ironstate_missing_command_94d0", false},
		{"existing", "Get-Command", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := engine.NewState()
			results, stopped, err := engine.RunLeaves([]tasks.Leaf{{
				Module: "fact",
				Item: map[string]any{
					"name": "scoop_installed", "value": "${{ rc == 0 }}",
					"shell": map[string]any{"type": "pwsh", "command": "Get-Command " + tc.command + " -ErrorAction SilentlyContinue"},
				},
				FailedWhen: []any{false},
			}}, engine.Options{Handlers: map[string]engine.Handler{"fact": factHandler{}}, Filters: filters.New()}, state)
			if err != nil || stopped || len(results) != 1 || results[0].Failed {
				t.Fatalf("results=%#v stopped=%v err=%v", results, stopped, err)
			}
			if got := state.UserFacts["scoop_installed"]; got != tc.want {
				t.Fatalf("scoop_installed=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestShellHandlerRunsCommandViaPwshFromShellEnv(t *testing.T) {
	t.Setenv("SHELL", "pwsh")
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0, Stdout: "hi\n", StdoutLines: []string{"hi"}}}}
	withRunner(t, rec)

	h := shellHandler{}
	item := map[string]any{"command": "Write-Output hi"}
	result, err := h.Install(item, "", testCtx())
	if err != nil {
		t.Fatal(err)
	}
	if rec.exes[0] != "pwsh" {
		t.Fatalf("exe = %q, want pwsh", rec.exes[0])
	}
	if result.Stdout != "hi\n" {
		t.Fatalf("result = %#v", result)
	}
}

func TestShellHostInvocationPreservesExecutablePathWithSpaces(t *testing.T) {
	host := filepath.Join(t.TempDir(), "Program Files", "PowerShell", "7", "pwsh.exe")
	if err := os.MkdirAll(filepath.Dir(host), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(host, []byte("test"), 0o700); err != nil {
		t.Fatal(err)
	}
	got := shellHostInvocation(host)
	if len(got) != 1 || got[0] != host {
		t.Fatalf("shellHostInvocation(%q) = %#v, want the path as one executable", host, got)
	}
}

func TestShellCustomPowerShellPathUsesPS1Extension(t *testing.T) {
	for _, host := range []string{`C:\Program Files\PowerShell\7\pwsh.exe`, `"C:\Program Files\PowerShell\7\pwsh.exe"`, `/opt/PowerShell/pwsh`} {
		if !isPowerShellHost(host) {
			t.Errorf("isPowerShellHost(%q) = false", host)
		}
		if got := shellScriptExtension(shellStateConfig{HostSpec: host}); got != ".ps1" {
			t.Errorf("shellScriptExtension(%q) = %q, want .ps1", host, got)
		}
	}
	for _, host := range []string{"bash", `C:\Program Files\Git\bin\bash.exe`} {
		if isPowerShellHost(host) {
			t.Errorf("isPowerShellHost(%q) = true", host)
		}
	}
}

func TestShellHandlerTypeResolution(t *testing.T) {
	tests := []struct {
		name     string
		item     map[string]any
		shell    string
		wantType string
		wantHost string
	}{
		{name: "type wins over host", item: map[string]any{"type": "bash", "host": "cmd"}, wantType: "bash", wantHost: "bash"},
		{name: "legacy host", item: map[string]any{"host": "cmd"}, wantHost: "cmd"},
		{name: "shell environment", item: map[string]any{}, shell: "/bin/zsh", wantType: "/bin/zsh", wantHost: "/bin/zsh"},
		{name: "os fallback", item: map[string]any{}, wantType: defaultShellType(), wantHost: defaultShellType()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SHELL", tt.shell)
			cfg := resolveShellStateConfig(tt.item, "present")
			if cfg.Type != tt.wantType || cfg.HostSpec != tt.wantHost {
				t.Fatalf("resolved type=%q host=%q, want type=%q host=%q", cfg.Type, cfg.HostSpec, tt.wantType, tt.wantHost)
			}
		})
	}
}

func TestShellHandlerPerStateFallback(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := shellHandler{}
	item := map[string]any{
		"host":    "cmd",
		"command": "echo top-level",
		"present": map[string]any{"command": "echo present-state"},
	}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	// per-state 'present' block's own 'command' wins, falls back to
	// top-level 'host' since the block doesn't set its own.
	if rec.exes[0] != "cmd.exe" {
		t.Fatalf("exe = %q, want cmd.exe (host falls back to top-level)", rec.exes[0])
	}
}

func TestShellHandlerAbsentDoesNotFallBackToTopLevelCommand(t *testing.T) {
	cfg := resolveShellStateConfig(map[string]any{"command": "echo install-only"}, "absent")
	if cfg.Command != "" {
		t.Fatalf("expected 'absent' to never fall back to the top-level command, got %q", cfg.Command)
	}
}

func TestShellHandlerCreatesGatesTest(t *testing.T) {
	dir := t.TempDir()
	marker := dir + "/done.marker"
	h := shellHandler{}
	item := map[string]any{"creates": []any{marker}}

	installed, _ := h.Test(item, "", testCtx())
	if installed {
		t.Fatal("expected not installed before marker exists")
	}
	if err := os.WriteFile(marker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	installed, _ = h.Test(item, "", testCtx())
	if !installed {
		t.Fatal("expected installed once marker exists")
	}
}
