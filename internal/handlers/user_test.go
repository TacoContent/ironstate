package handlers

import (
	"strings"
	"testing"

	ironexec "github.com/TacoContent/ironstate/internal/exec"
)

func TestIsWindowsHumanAccountSID(t *testing.T) {
	tests := []struct {
		sid  string
		want bool
	}{
		{sid: "S-1-5-21-100-200-300-1001", want: true},
		{sid: "S-1-5-21-100-200-300-500", want: false},
		{sid: "S-1-5-32-544", want: false},
		{sid: "", want: false},
	}

	for _, tt := range tests {
		if got := isWindowsHumanAccountSID(tt.sid); got != tt.want {
			t.Fatalf("isWindowsHumanAccountSID(%q) = %v, want %v", tt.sid, got, tt.want)
		}
	}
}

func TestIsMacOSBuiltinUser(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "_spotlight", want: true},
		{name: "_www", want: true},
		{name: "daemon", want: true},
		{name: "nobody", want: true},
		{name: "root", want: true},
		{name: "ryan", want: false},
		{name: "", want: false},
	}
	for _, tt := range tests {
		if got := isMacOSBuiltinUser(tt.name); got != tt.want {
			t.Fatalf("isMacOSBuiltinUser(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIsLinuxBuiltinUser(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "_apt", want: true},
		{name: "daemon", want: true},
		{name: "www-data", want: true},
		{name: "root", want: true},
		{name: "systemd-network", want: true},
		{name: "ryan", want: false},
		{name: "", want: false},
	}
	for _, tt := range tests {
		if got := isLinuxBuiltinUser(tt.name); got != tt.want {
			t.Fatalf("isLinuxBuiltinUser(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestUserHandlerScanFiltersBuiltinAccountsOnWindows(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0, Stdout: `[{"Name":"Administrator","SID":"S-1-5-21-1-2-3-500"},{"Name":"ryan","SID":"S-1-5-21-1-2-3-1001"}]`}}}
	withRunner(t, rec)

	items, err := userHandler{}.Scan(testCtx())
	if err != nil {
		t.Fatal(err)
	}
	if rec.exes[0] != "pwsh" {
		t.Fatalf("exe = %q, want pwsh", rec.exes[0])
	}
	if len(items) != 1 || items[0].Name != "ryan" {
		t.Fatalf("items = %+v, want just the human account 'ryan'", items)
	}
	if items[0].Module != "user" {
		t.Fatalf("module = %q, want user", items[0].Module)
	}
}

func TestUserHandlerScanRoleIsSystemUsers(t *testing.T) {
	h := userHandler{}
	if got := h.ScanRole(); got != "roles/system/users" {
		t.Fatalf("ScanRole() = %q, want roles/system/users", got)
	}
}

func TestParseWindowsUsersHandlesSingleBareObject(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0, Stdout: `{"Name":"ryan","SID":"S-1-5-21-1-2-3-1001"}`}}}
	withRunner(t, rec)

	entries, err := parseWindowsUsers()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Username != "ryan" {
		t.Fatalf("entries = %+v", entries)
	}
	if !strings.Contains(rec.calls[0][len(rec.calls[0])-1], "Get-LocalUser") {
		t.Fatalf("command = %v, want it to invoke Get-LocalUser", rec.calls[0])
	}
}
