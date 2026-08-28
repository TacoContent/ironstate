package scan

import "testing"

type stubScanner struct {
	name  string
	role  string
	items []Item
}

func (s stubScanner) Name() string { return s.name }

func (s stubScanner) Role() string { return s.role }

func (s stubScanner) Scan() ([]Item, error) { return s.items, nil }

func TestRegistryScanAllWithProgress(t *testing.T) {
	reg := &Registry{}
	reg.Register(stubScanner{name: "users", role: "roles/system/users", items: []Item{{Name: "alice"}}})
	reg.Register(stubScanner{name: "groups", role: "roles/system/groups", items: []Item{{Name: "devs"}}})

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
	if items[0].Role != "roles/system/users" || items[1].Role != "roles/system/groups" {
		t.Fatalf("roles = %q, %q, want stamped from each scanner", items[0].Role, items[1].Role)
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
