package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/TacoContent/ironstate/internal/engine"
	"github.com/TacoContent/ironstate/internal/handlers"
	"github.com/TacoContent/ironstate/internal/model"
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
	return NewRegistryWithHandlers(nil)
}

// NewRegistryWithHandlers returns a registry for built-in scanners plus any
// externally supplied handlers that implement engine.ScanCapable.
func NewRegistryWithHandlers(external map[string]engine.Handler) *Registry {
	r := &Registry{}
	for _, s := range scannersForHandlers(handlers.All()) {
		r.Register(s)
	}
	for _, s := range scannersForHandlers(external) {
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
	items, err := h.cap.Scan(engine.Context{})
	if err != nil {
		return nil, err
	}
	if strings.Count(h.name, ".") == 2 {
		for index := range items {
			if !strings.Contains(items[index].Module, ".") {
				items[index].Module = h.name
			}
		}
	}
	return items, nil
}

func scannersForHandlers(all map[string]engine.Handler) []Scanner {
	names := make([]string, 0, len(all))
	for name := range all {
		if name == "brew" || strings.HasPrefix(name, "ironstate.builtin.") {
			// Aliases point to the same implementation as their base handler.
			// Scan each built-in once under its canonical module name.
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]Scanner, 0, len(names))
	for _, name := range names {
		if sc, ok := all[name].(engine.ScanCapable); ok {
			if sc.ScanRole() != "" {
				out = append(out, handlerScanner{name: name, cap: sc})
			}
		}
	}
	return out
}

// GeneratePlaybook writes a baseline playbook tree rooted at target.
// installedVersions resolves a scanned plugin namespace ("organization.
// plugin") to its latest installed version, used to register any plugin
// whose qualified handler contributed scanned items - see mergedPluginUses.
func GeneratePlaybook(target string, known []Item, installedVersions map[string]string) error {
	if target == "" {
		target = "."
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
	roleDirs := orderedRoleDirs(itemsByRole)
	paths := []string{
		target,
		filepath.Join(target, "roles"),
		filepath.Join(target, "tasks"),
		filepath.Join(target, "packages"),
		filepath.Join(target, "hosts"),
		filepath.Join(target, "variables"),
	}
	for _, roleDir := range roleDirs {
		paths = append(paths, filepath.Join(target, roleDir))
	}
	for _, p := range paths {
		if err := os.MkdirAll(p, 0o750); err != nil {
			return err
		}
	}

	includeTasks := make([]map[string]any, 0, len(roleDirs))
	for _, roleDir := range roleDirs {
		includeTasks = append(includeTasks, map[string]any{
			"name":    "Include baseline " + filepath.Base(roleDir),
			"include": map[string]any{"name": roleDir},
		})
	}

	mainDoc := map[string]any{
		"version": "1",
		"vars": map[string]any{
			"generated_by": "ironstate scan",
			"generated_at": time.Now().UTC().Format(time.RFC3339),
		},
		"tasks": includeTasks,
	}
	if pluginUses := mergedPluginUses(target, known, installedVersions); len(pluginUses) > 0 {
		mainDoc["plugins"] = pluginUses
	}
	if err := writeYAML(filepath.Join(target, "main.yml"), mainDoc); err != nil {
		return err
	}

	for _, roleDir := range roleDirs {
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

func orderedRoleDirs(itemsByRole map[string][]Item) []string {
	standard := []string{"roles/system/users", "roles/system/groups", "roles/system/services", "roles/packages"}
	known := make(map[string]bool, len(standard))
	for _, role := range standard {
		known[role] = true
	}
	extra := make([]string, 0, len(itemsByRole))
	for role := range itemsByRole {
		if !known[role] {
			extra = append(extra, role)
		}
	}
	sort.Strings(extra)
	return append(standard, extra...)
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

// mergedPluginUses builds the 'plugins:' list for target's main.yml: every
// plugin namespace already declared in an existing main.yml (if any) is
// preserved verbatim, in its original order, and never removed - even if
// this scan didn't itself require it. Any additional namespace required by
// known's own qualified ("organization.plugin.handler") items but not
// already declared is appended, pinned to installedVersions' resolved
// latest-installed version, in sorted order. A required namespace with no
// entry in installedVersions is skipped rather than failing the whole
// generation - matches ScanAllWithProgress's own best-effort error handling.
func mergedPluginUses(target string, known []Item, installedVersions map[string]string) []map[string]any {
	existingNamespaces := map[string]bool{}
	var uses []map[string]any
	if doc, err := readExistingDocument(filepath.Join(target, "main.yml")); err == nil {
		if plugins, err := model.Plugins(doc); err == nil {
			for _, p := range plugins {
				existingNamespaces[p.Namespace] = true
				uses = append(uses, map[string]any{"use": p.Namespace + "@" + p.Version})
			}
		}
	}

	required := map[string]bool{}
	for _, item := range known {
		if ns, ok := pluginNamespace(item.Module); ok {
			required[ns] = true
		}
	}
	additions := make([]string, 0, len(required))
	for ns := range required {
		if !existingNamespaces[ns] {
			additions = append(additions, ns)
		}
	}
	sort.Strings(additions)
	for _, ns := range additions {
		version, ok := installedVersions[ns]
		if !ok {
			continue
		}
		uses = append(uses, map[string]any{"use": ns + "@" + version})
	}
	return uses
}

// pluginNamespace extracts "organization.plugin" from a scanned item's
// qualified module name ("organization.plugin.handler" - see
// handlerScanner.Scan's own qualification), or reports ok=false for a
// built-in (unqualified) module.
func pluginNamespace(module string) (string, bool) {
	if strings.Count(module, ".") != 2 {
		return "", false
	}
	return module[:strings.LastIndex(module, ".")], true
}

func readExistingDocument(path string) (map[string]any, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is this package's own generated playbook main.yml
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc == nil {
		doc = map[string]any{}
	}
	return doc, nil
}
