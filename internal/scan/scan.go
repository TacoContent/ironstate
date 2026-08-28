package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/TacoContent/ironstate/internal/engine"
	"github.com/TacoContent/ironstate/internal/handlers"
)

// Scanner is a pluggable source of baseline config for a generated playbook.
type Scanner interface {
	Name() string
	// Role names the playbook role directory (e.g. "roles/system/users")
	// this scanner's items are grouped under.
	Role() string
	Scan() ([]Item, error)
}

// Item is a single configuration object emitted by a scan. It is a type
// alias for engine.ScanItem so a handlers.*Handler's Scan result (see
// engine.ScanCapable) needs no conversion on its way into GeneratePlaybook.
type Item = engine.ScanItem

// Registry holds scan implementations and keeps the system extensible.
type Registry struct {
	scanners []Scanner
}

func NewRegistry() *Registry {
	r := &Registry{}
	for _, s := range defaultScanners() {
		r.Register(s)
	}
	return r
}

func (r *Registry) Register(s Scanner) {
	if s == nil {
		return
	}
	r.scanners = append(r.scanners, s)
}

func (r *Registry) ScanAll() ([]Item, error) {
	return r.ScanAllWithProgress(nil)
}

func (r *Registry) ScanAllWithProgress(progress func(name string, index, total int)) ([]Item, error) {
	var out []Item
	total := len(r.scanners)
	for i, s := range r.scanners {
		if progress != nil && s != nil {
			progress(s.Name(), i+1, total)
		}
		items, err := s.Scan()
		if err != nil {
			continue
		}
		role := s.Role()
		for idx := range items {
			if items[idx].Role == "" {
				items[idx].Role = role
			}
		}
		out = append(out, items...)
	}
	return out, nil
}

func (r *Registry) ListNames() []string {
	out := make([]string, 0, len(r.scanners))
	for _, s := range r.scanners {
		if s != nil {
			out = append(out, s.Name())
		}
	}
	return out
}

// handlerScanner adapts a handlers.All() entry that implements
// engine.ScanCapable into a Scanner, so a module's scan logic can live
// next to its own handlers.Handler instead of this package maintaining a
// separate, hardcoded scanner per module.
type handlerScanner struct {
	name string
	cap  engine.ScanCapable
}

func (h handlerScanner) Name() string { return h.name }
func (h handlerScanner) Role() string { return h.cap.ScanRole() }
func (h handlerScanner) Scan() ([]Item, error) {
	return h.cap.Scan(engine.Context{})
}

// defaultScanners builds one Scanner per handlers.All() entry that
// implements engine.ScanCapable, dynamically discovering what can be
// scanned instead of hardcoding a scanner per module here.
func defaultScanners() []Scanner {
	all := handlers.All()
	names := make([]string, 0, len(all))
	for name := range all {
		if name == "brew" {
			// "brew" is a registered alias for "homebrew" pointing at
			// the exact same handler instance (see handlers.All) - skip
			// it here so its packages aren't scanned (and duplicated)
			// twice under two different names.
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]Scanner, 0, len(names))
	for _, name := range names {
		if sc, ok := all[name].(engine.ScanCapable); ok {
			out = append(out, handlerScanner{name: name, cap: sc})
		}
	}
	return out
}

// GeneratePlaybook writes a baseline playbook tree rooted at target.
func GeneratePlaybook(target string, known []Item) error {
	if target == "" {
		target = "."
	}
	paths := []string{
		target,
		filepath.Join(target, "roles"),
		filepath.Join(target, "roles", "system"),
		filepath.Join(target, "roles", "system", "users"),
		filepath.Join(target, "roles", "system", "groups"),
		filepath.Join(target, "roles", "system", "services"),
		filepath.Join(target, "roles", "packages"),
		filepath.Join(target, "tasks"),
		filepath.Join(target, "packages"),
		filepath.Join(target, "hosts"),
		filepath.Join(target, "variables"),
	}
	for _, p := range paths {
		if err := os.MkdirAll(p, 0o750); err != nil {
			return err
		}
	}

	itemsByRole := map[string][]Item{}
	for _, item := range known {
		role := item.Role
		if role == "" {
			// Items constructed directly (e.g. by callers/tests that
			// don't go through Registry.ScanAll, which is what stamps
			// Role from each Scanner) fall back to the same routing
			// this package has always used.
			switch item.Module {
			case "user":
				role = "roles/system/users"
			case "group":
				role = "roles/system/groups"
			case "service":
				role = "roles/system/services"
			default:
				role = "roles/packages"
			}
		}
		itemsByRole[role] = append(itemsByRole[role], item)
	}

	if err := writeYAML(filepath.Join(target, "main.yml"), map[string]any{
		"version": "1",
		"vars": map[string]any{
			"generated_by": "ironstate scan",
			"generated_at": time.Now().UTC().Format(time.RFC3339),
		},
		"tasks": []map[string]any{
			{"name": "Include baseline users", "include": map[string]any{"name": "roles/system/users"}},
			{"name": "Include baseline groups", "include": map[string]any{"name": "roles/system/groups"}},
			{"name": "Include baseline services", "include": map[string]any{"name": "roles/system/services"}},
			{"name": "Include baseline packages", "include": map[string]any{"name": "roles/packages"}},
		},
	}); err != nil {
		return err
	}

	for _, roleDir := range []string{"roles/system/users", "roles/system/groups", "roles/system/services", "roles/packages"} {
		if err := writeYAML(filepath.Join(target, roleDir, "main.yml"), map[string]any{"tasks": buildTaskList(itemsByRole[roleDir], filepath.Base(roleDir))}); err != nil {
			return err
		}
	}

	if err := writeYAML(filepath.Join(target, "hosts", "localhost.yml"), map[string]any{"tasks": []any{}}); err != nil {
		return err
	}
	if err := writeYAML(filepath.Join(target, "variables", "default.yml"), map[string]any{"vars": map[string]any{}}); err != nil {
		return err
	}
	return nil
}

func buildTaskList(items []Item, roleName string) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if item.Name == "" {
			continue
		}
		task := map[string]any{"name": fmt.Sprintf("Ensure %s %s", roleName, item.Name)}
		if len(item.Tags) > 0 {
			task["tags"] = item.Tags
		}
		cfg := map[string]any{}
		if item.Config != nil {
			for k, v := range item.Config {
				cfg[k] = v
			}
		}
		if len(cfg) == 0 {
			cfg["state"] = "present"
		}
		task[item.Module] = cfg
		out = append(out, task)
	}
	if len(out) == 0 {
		return []map[string]any{{"name": "No items discovered for this baseline", "log": map[string]any{"message": "no matching items found"}}}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i]["name"].(string) < out[j]["name"].(string)
	})
	return out
}

func writeYAML(path string, value any) error {
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
