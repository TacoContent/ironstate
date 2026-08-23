//go:build windows

package handlers

import (
	"strings"
	"testing"

	ironexec "github.com/TacoContent/ironstate/internal/exec"
)

func TestScheduledTaskBuildDefinitionXML(t *testing.T) {
	item := map[string]any{
		"name":        "smartctl_exporter",
		"description": "Run on logon",
		"actions": []any{
			map[string]any{"execute": "C:\\tool.exe", "arguments": "--flag=1"},
		},
		"triggers": []any{
			map[string]any{"type": "logon"},
			map[string]any{"type": "startup"},
		},
		"principal": map[string]any{"user_id": "S-1-5-20", "run_level": "Highest"},
		"settings": map[string]any{
			"multiple_instances":            "IgnoreNew",
			"start_when_available":          true,
			"run_only_if_network_available": true,
		},
	}

	xmlDoc, err := buildTaskDefinitionXML(item)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<LogonTrigger>", "<BootTrigger>", "<UserId>S-1-5-20</UserId>",
		"<RunLevel>HighestAvailable</RunLevel>", "<Command>C:\\tool.exe</Command>",
		"<Arguments>--flag=1</Arguments>", "<MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>",
		"<StartWhenAvailable>true</StartWhenAvailable>",
	} {
		if !strings.Contains(xmlDoc, want) {
			t.Fatalf("expected xml to contain %q, got:\n%s", want, xmlDoc)
		}
	}
}

func TestScheduledTaskInstallInvokesSchtasksCreate(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := scheduledTaskHandler{}
	item := map[string]any{
		"name":    "mytask",
		"actions": []any{map[string]any{"execute": "notepad.exe"}},
	}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if rec.exes[0] != "schtasks.exe" || rec.calls[0][0] != "/Create" {
		t.Fatalf("exe=%q args=%v", rec.exes[0], rec.calls[0])
	}
}

func TestScheduledTaskUninstallInvokesSchtasksDelete(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := scheduledTaskHandler{}
	item := map[string]any{"name": "mytask"}
	if _, err := h.Uninstall(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if rec.exes[0] != "schtasks.exe" || rec.calls[0][0] != "/Delete" {
		t.Fatalf("exe=%q args=%v", rec.exes[0], rec.calls[0])
	}
}

func TestScheduledTaskTestReportsNotInstalledWhenQueryFails(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 1}}}
	withRunner(t, rec)

	h := scheduledTaskHandler{}
	installed, err := h.Test(map[string]any{"name": "missingtask"}, "", testCtx())
	if err != nil || installed {
		t.Fatalf("installed=%v err=%v, want false", installed, err)
	}
}

func TestParseAndFormatISO8601Duration(t *testing.T) {
	d, err := parseTaskDuration("PT1H30M")
	if err != nil {
		t.Fatal(err)
	}
	if d.Hours() != 1.5 {
		t.Fatalf("d = %v", d)
	}
	if got := formatISO8601Duration(d); got != "PT1H30M" {
		t.Fatalf("formatted = %q", got)
	}
}

func TestParseDotNetTimeSpanDuration(t *testing.T) {
	d, err := parseTaskDuration("1.00:00:00")
	if err != nil {
		t.Fatal(err)
	}
	if d.Hours() != 24 {
		t.Fatalf("d = %v", d)
	}
}
