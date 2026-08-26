package scan

import "testing"

type stubScanner struct {
	name  string
	items []Item
}

func (s stubScanner) Name() string { return s.name }

func (s stubScanner) Scan() ([]Item, error) { return s.items, nil }

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

func TestIsUserManagedWindowsService(t *testing.T) {
	tests := []struct {
		service serviceEntry
		want    bool
	}{
		{service: serviceEntry{Name: "Contoso Service", PathName: `"C:\Program Files\Contoso\contoso.exe" --service`}, want: true},
		{service: serviceEntry{Name: "Windows Service", PathName: `C:\Windows\System32\svchost.exe -k netsvcs`}, want: false},
		{service: serviceEntry{Name: "Empty", PathName: ""}, want: false},
	}

	for _, tt := range tests {
		if got := isUserManagedWindowsService(tt.service); got != tt.want {
			t.Fatalf("isUserManagedWindowsService(%+v) = %v, want %v", tt.service, got, tt.want)
		}
	}
}

func TestIsWingetManagedSource(t *testing.T) {
	tests := []struct {
		source string
		want   bool
	}{
		{source: "winget", want: true},
		{source: "msstore", want: true},
		{source: "", want: false},
		{source: "manual", want: false},
	}

	for _, tt := range tests {
		if got := isWingetManagedSource(tt.source); got != tt.want {
			t.Fatalf("isWingetManagedSource(%q) = %v, want %v", tt.source, got, tt.want)
		}
	}
}

func TestRegistryScanAllWithProgress(t *testing.T) {
	reg := &Registry{}
	reg.Register(stubScanner{name: "users", items: []Item{{Name: "alice"}}})
	reg.Register(stubScanner{name: "groups", items: []Item{{Name: "devs"}}})

	var seen []string
	items, err := reg.ScanAllWithProgress(func(name string, index, total int) {
		seen = append(seen, name)
		if total != 2 {
			t.Fatalf("total = %d, want 2", total)
		}
		if index < 1 || index > total {
			t.Fatalf("index = %d, total = %d", index, total)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if len(seen) != 2 || seen[0] != "users" || seen[1] != "groups" {
		t.Fatalf("seen = %v, want [users groups]", seen)
	}
}

func TestWingetPackagesToEntriesKeepsIdsAndSources(t *testing.T) {
	previousLookup := wingetDisplayNameLookup
	wingetDisplayNameLookup = func(identifier string) string {
		if identifier == "9P8LTPGCBZXD" {
			return "Wintoys"
		}
		return ""
	}
	defer func() {
		wingetDisplayNameLookup = previousLookup
	}()

	items := wingetPackagesToEntries([]wingetExportPackage{
		{Identifier: "Microsoft.VCRedist.2010.x64", Source: "winget"},
		{Identifier: "Microsoft.VCRedist.2010.x86", Source: "winget"},
		{Identifier: "9P8LTPGCBZXD", Source: "msstore"},
		{Identifier: "7zip.7zip", Source: "winget"},
	})
	if len(items) != 4 {
		t.Fatalf("items = %d, want 4", len(items))
	}
	for _, want := range []packageEntry{
		{Name: "Microsoft.VCRedist.2010.x64", Identifier: "Microsoft.VCRedist.2010.x64", Source: "winget"},
		{Name: "Microsoft.VCRedist.2010.x86", Identifier: "Microsoft.VCRedist.2010.x86", Source: "winget"},
		{Name: "Wintoys", Identifier: "9P8LTPGCBZXD", Source: "msstore"},
		{Name: "7zip.7zip", Identifier: "7zip.7zip", Source: "winget"},
	} {
		matched := false
		for _, got := range items {
			if got.Identifier == want.Identifier {
				matched = true
				if got != want {
					t.Fatalf("package %+v, want %+v", got, want)
				}
			}
		}
		if !matched {
			t.Fatalf("missing package identifier %q", want.Identifier)
		}
	}
}

func TestBuildTaskListUsesLogForEmptyScan(t *testing.T) {
	tasks := buildTaskList(nil, "system/groups")
	if len(tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(tasks))
	}
	if got := tasks[0]["name"]; got != "No items discovered for this baseline" {
		t.Fatalf("name = %v, want %q", got, "No items discovered for this baseline")
	}
	logTask, ok := tasks[0]["log"].(map[string]any)
	if !ok {
		t.Fatalf("log task missing or wrong type: %#v", tasks[0]["log"])
	}
	if got := logTask["message"]; got != "no matching items found" {
		t.Fatalf("log.message = %v, want %q", got, "no matching items found")
	}
	if _, exists := tasks[0]["debug"]; exists {
		t.Fatalf("debug task should not be present: %#v", tasks[0])
	}
}
