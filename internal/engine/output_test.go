package engine

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/TacoContent/ironstate/internal/secrets"
	"github.com/TacoContent/ironstate/internal/ui"
)

func withColorDisabled(t *testing.T) {
	t.Helper()
	prev := ui.Enabled
	ui.Enabled = false
	t.Cleanup(func() { ui.Enabled = prev })
}

func TestComputeStats(t *testing.T) {
	results := []Result{
		{Action: ActionInstall, Apply: true},
		{Action: ActionUninstall, Apply: true},
		{Action: ActionSkip},
		{Action: ActionSkip},
		{Action: ActionInstall, Apply: true, Failed: true},
	}
	stats := ComputeStats(results)
	if stats.Total != 5 || stats.Installed != 1 || stats.Uninstalled != 1 || stats.Skipped != 2 || stats.Failed != 1 {
		t.Fatalf("ComputeStats = %+v, want {Total:5 Installed:1 Uninstalled:1 Skipped:2 Failed:1}", stats)
	}
}

func TestPrintSummary(t *testing.T) {
	withColorDisabled(t)
	var buf bytes.Buffer
	stats := Stats{Total: 3, Installed: 1, Uninstalled: 1, Skipped: 1, Failed: 0}
	if err := PrintSummary(&buf, stats, 250*time.Millisecond); err != nil {
		t.Fatalf("PrintSummary error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"Summary", "Installed", "Uninstalled", "Skipped", "Failed", "Total: 3 task(s)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("PrintSummary output missing %q; got:\n%s", want, out)
		}
	}
}

func TestPrintTableAlignsWithoutColor(t *testing.T) {
	withColorDisabled(t)
	var buf bytes.Buffer
	results := []Result{
		{Module: "winget", Package: "Git.Git", State: "present", Action: ActionInstall, Apply: true},
		{Module: "fact", Package: "computer_name", State: "present", Action: ActionSkip},
		{Module: "eget", Package: "grype", State: "present", Action: ActionInstall, Apply: true, Failed: true},
	}
	if err := PrintTable(&buf, results); err != nil {
		t.Fatalf("PrintTable error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"MODULE", "Git.Git", "installed", "skip", "failed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("PrintTable output missing %q; got:\n%s", want, out)
		}
	}
}

func withColorEnabled(t *testing.T) {
	t.Helper()
	prev := ui.Enabled
	ui.Enabled = true
	t.Cleanup(func() { ui.Enabled = prev })
}

// TestPrintTableUsesCRLFWhenEnabled guards against a real bug: on at
// least one Windows terminal, plain "\n" only moves the cursor down a
// row without returning to column 0, so every row after the first drifts
// further right (a "staircase") - single-line engine.Info/Warn prints
// never showed this because the progress spinner's own '\r' erase/redraw
// happens to bracket each of them, but PrintTable's back-to-back rows
// have no such bracketing. See ui.Newline's doc comment.
func TestPrintTableUsesCRLFWhenEnabled(t *testing.T) {
	withColorEnabled(t)
	var buf bytes.Buffer
	results := []Result{
		{Module: "winget", Package: "Git.Git", State: "present", Action: ActionInstall, Apply: true},
		{Module: "fact", Package: "computer_name", State: "present", Action: ActionSkip},
	}
	if err := PrintTable(&buf, results); err != nil {
		t.Fatalf("PrintTable error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "\r\n") {
		t.Fatalf("PrintTable with Enabled=true should use \\r\\n line endings, got %q", out)
	}
	if strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n") {
		t.Fatalf("PrintTable with Enabled=true should never emit a bare \\n, got %q", out)
	}
}

func TestPrintSummaryUsesCRLFWhenEnabled(t *testing.T) {
	withColorEnabled(t)
	var buf bytes.Buffer
	stats := Stats{Total: 3, Installed: 1, Uninstalled: 1, Skipped: 1, Failed: 0}
	if err := PrintSummary(&buf, stats, 250*time.Millisecond); err != nil {
		t.Fatalf("PrintSummary error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "\r\n") {
		t.Fatalf("PrintSummary with Enabled=true should use \\r\\n line endings, got %q", out)
	}
	if strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n") {
		t.Fatalf("PrintSummary with Enabled=true should never emit a bare \\n, got %q", out)
	}
}

func TestPrintJSONRedactsRegisteredSecrets(t *testing.T) {
	secrets.Register("super-secret-value")
	var buf bytes.Buffer
	results := []Result{{
		Module:  "log",
		Package: "show-token",
		State:   "present",
		Action:  ActionInstall,
		Apply:   true,
		Exec: ExecResult{
			RC:     0,
			Stdout: "super-secret-value\n",
			Stderr: "super-secret-value",
		},
	}}
	if err := PrintJSON(&buf, results); err != nil {
		t.Fatalf("PrintJSON error: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "super-secret-value") {
		t.Fatalf("PrintJSON leaked secret in output:\n%s", out)
	}
	if !strings.Contains(out, "***") {
		t.Fatalf("PrintJSON should redact registered secrets, got:\n%s", out)
	}
}
