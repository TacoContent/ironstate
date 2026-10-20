package handlers

import (
	"os/exec"
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
