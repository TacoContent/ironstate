package handlers

import (
	"testing"

	ironexec "github.com/TacoContent/ironstate/internal/exec"
)

func TestIsWindowsBuiltInGroupSID(t *testing.T) {
	tests := []struct {
		sid  string
		want bool
	}{
		{sid: "S-1-5-32-544", want: true},
		{sid: "S-1-5-21-100-200-300-1001", want: false},
		{sid: "", want: false},
	}

	for _, tt := range tests {
		if got := isWindowsBuiltInGroupSID(tt.sid); got != tt.want {
			t.Fatalf("isWindowsBuiltInGroupSID(%q) = %v, want %v", tt.sid, got, tt.want)
		}
	}
}

func TestIsMacOSBuiltinGroup(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "_developer", want: true},
		{name: "com.apple.access_ssh", want: true},
		{name: "com.apple.anything", want: true},
		{name: "admin", want: true},
		{name: "wheel", want: true},
		{name: "staff", want: true},
		{name: "developers", want: false},
		{name: "", want: false},
	}
	for _, tt := range tests {
		if got := isMacOSBuiltinGroup(tt.name); got != tt.want {
			t.Fatalf("isMacOSBuiltinGroup(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIsLinuxBuiltinGroup(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "_ssh", want: true},
		{name: "sudo", want: true},
		{name: "docker", want: true},
		{name: "users", want: true},
		{name: "root", want: true},
		{name: "developers", want: false},
		{name: "", want: false},
	}
	for _, tt := range tests {
		if got := isLinuxBuiltinGroup(tt.name); got != tt.want {
			t.Fatalf("isLinuxBuiltinGroup(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestGroupHandlerScanFiltersBuiltinGroupsOnWindows(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0, Stdout: `[{"Name":"Administrators","SID":"S-1-5-32-544"},{"Name":"devs","SID":"S-1-5-21-1-2-3-1001"}]`}}}
	withRunner(t, rec)

	items, err := groupHandler{}.Scan(testCtx())
	if err != nil {
		t.Fatal(err)
	}
	if rec.exes[0] != "pwsh" {
		t.Fatalf("exe = %q, want pwsh", rec.exes[0])
	}
	if len(items) != 1 || items[0].Name != "devs" {
		t.Fatalf("items = %+v, want just the human group 'devs'", items)
	}
	if items[0].Module != "group" {
		t.Fatalf("module = %q, want group", items[0].Module)
	}
}

func TestGroupHandlerScanRoleIsSystemGroups(t *testing.T) {
	h := groupHandler{}
	if got := h.ScanRole(); got != "roles/system/groups" {
		t.Fatalf("ScanRole() = %q, want roles/system/groups", got)
	}
}
