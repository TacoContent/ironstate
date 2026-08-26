package scan

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Scanner is a pluggable source of baseline config for a generated playbook.
type Scanner interface {
	Name() string
	Scan() ([]Item, error)
}

// Item is a single configuration object emitted by a scan.
type Item struct {
	Module string         `yaml:"-"`
	Name   string         `yaml:"name"`
	Config map[string]any `yaml:"config"`
	Tags   []string       `yaml:"tags,omitempty"`
}

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

type userScanner struct{}

func (userScanner) Name() string { return "users" }

func (userScanner) Scan() ([]Item, error) {
	entries, err := readPasswdEntries()
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(entries))
	for _, e := range entries {
		if e.Username == "" {
			continue
		}
		if runtime.GOOS == "windows" && !isWindowsHumanAccountSID(e.SID) {
			continue
		}
		out = append(out, Item{
			Module: "user",
			Name:   e.Username,
			Config: map[string]any{"name": e.Username, "home": e.Home, "shell": e.Shell, "state": "present"},
			Tags:   []string{"system", "users"},
		})
	}
	return out, nil
}

type groupScanner struct{}

func (groupScanner) Name() string { return "groups" }

func (groupScanner) Scan() ([]Item, error) {
	entries, err := readGroupEntries()
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(entries))
	for _, e := range entries {
		if e.Name == "" {
			continue
		}
		if runtime.GOOS == "windows" && isWindowsBuiltInGroupSID(e.SID) {
			continue
		}
		out = append(out, Item{
			Module: "group",
			Name:   e.Name,
			Config: map[string]any{"name": e.Name, "gid": e.GID, "state": "present"},
			Tags:   []string{"system", "groups"},
		})
	}
	return out, nil
}

type serviceScanner struct{}

func (serviceScanner) Name() string { return "services" }

func (serviceScanner) Scan() ([]Item, error) {
	if runtime.GOOS == "windows" {
		return nil, nil
	}
	entries, err := discoverServices()
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(entries))
	for _, e := range entries {
		if e.Name == "" {
			continue
		}
		if runtime.GOOS == "windows" && !isUserManagedWindowsService(e) {
			continue
		}
		out = append(out, Item{
			Module: "service",
			Name:   e.Name,
			Config: map[string]any{"name": e.Name, "state": "started", "enabled": true},
			Tags:   []string{"system", "services"},
		})
	}
	return out, nil
}

type packageScanner struct{}

func (packageScanner) Name() string { return "packages" }

func (packageScanner) Scan() ([]Item, error) {
	entries, err := discoverPackages()
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(entries))
	for _, p := range entries {
		if p.Name == "" {
			continue
		}
		module := "winget"
		if runtime.GOOS != "windows" {
			module = "npm"
		} else if !isWingetManagedSource(p.Source) {
			continue
		}
		out = append(out, Item{
			Module: module,
			Name:   p.Name,
			Config: map[string]any{"package": p.Identifier, "source": p.Source, "state": "present"},
			Tags:   []string{"packages"},
		})
	}
	return out, nil
}

func defaultScanners() []Scanner {
	if runtime.GOOS == "windows" {
		return []Scanner{
			userScanner{},
			groupScanner{},
			packageScanner{},
		}
	}
	return []Scanner{
		userScanner{},
		groupScanner{},
		serviceScanner{},
		packageScanner{},
	}
}

type passwdEntry struct {
	Username string
	Home     string
	Shell    string
	SID      string
}

type groupEntry struct {
	Name string
	GID  string
	SID  string
}

type serviceEntry struct {
	Name      string
	PathName  string
	StartName string
}

type packageEntry struct {
	Name       string
	Identifier string
	Source     string
}

type wingetExportPackage struct {
	Identifier string
	Version    string
	Source     string
}

type wingetExportFile struct {
	Sources []struct {
		Packages []struct {
			PackageIdentifier string `json:"PackageIdentifier"`
			Version           string `json:"Version"`
		} `json:"Packages"`
	} `json:"Sources"`
}

type windowsLocalUser struct {
	Name string `json:"Name"`
	SID  string `json:"SID"`
}

type windowsLocalGroup struct {
	Name string `json:"Name"`
	SID  string `json:"SID"`
}

type windowsService struct {
	Name      string `json:"Name"`
	PathName  string `json:"PathName"`
	StartName string `json:"StartName"`
}

func parseJSONList[T any](out string) ([]T, error) {
	var items []T
	if err := json.Unmarshal([]byte(out), &items); err == nil {
		return items, nil
	}
	var item T
	if err := json.Unmarshal([]byte(out), &item); err == nil {
		return []T{item}, nil
	}
	return nil, fmt.Errorf("unable to parse JSON list")
}

func readPasswdEntries() ([]passwdEntry, error) {
	if runtime.GOOS == "windows" {
		return parseWindowsUsers()
	}
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return nil, err
	}

	entries := make([]passwdEntry, 0)
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 7 {
			continue
		}
		entries = append(entries, passwdEntry{Username: parts[0], Home: parts[5], Shell: parts[6]})
	}
	if scanErr := s.Err(); scanErr != nil {
		if closeErr := f.Close(); closeErr != nil {
			return entries, closeErr
		}
		return entries, scanErr
	}
	if closeErr := f.Close(); closeErr != nil {
		return entries, closeErr
	}
	return entries, nil
}

func parseWindowsUsers() ([]passwdEntry, error) {
	out, err := runCommand("powershell", "-NoProfile", "-NonInteractive", "-Command", "Get-LocalUser | Select-Object Name,SID | ConvertTo-Json -Depth 4 -Compress")
	if err != nil {
		return nil, err
	}
	records, err := parseJSONList[windowsLocalUser](out)
	if err != nil {
		return nil, err
	}
	entries := make([]passwdEntry, 0, len(records))
	for _, record := range records {
		if record.Name == "" || !isWindowsHumanAccountSID(record.SID) {
			continue
		}
		entries = append(entries, passwdEntry{Username: record.Name, SID: record.SID})
	}
	return entries, nil
}

func readGroupEntries() ([]groupEntry, error) {
	if runtime.GOOS == "windows" {
		out, err := runCommand("powershell", "-NoProfile", "-NonInteractive", "-Command", "Get-LocalGroup | Select-Object Name,SID | ConvertTo-Json -Depth 4 -Compress")
		if err != nil {
			return nil, err
		}
		records, err := parseJSONList[windowsLocalGroup](out)
		if err != nil {
			return nil, err
		}
		entries := make([]groupEntry, 0, len(records))
		for _, record := range records {
			if record.Name == "" || isWindowsBuiltInGroupSID(record.SID) {
				continue
			}
			entries = append(entries, groupEntry{Name: record.Name, SID: record.SID})
		}
		return entries, nil
	}
	f, err := os.Open("/etc/group")
	if err != nil {
		return nil, err
	}
	entries := make([]groupEntry, 0)
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 4 {
			continue
		}
		entries = append(entries, groupEntry{Name: parts[0], GID: parts[2]})
	}
	if scanErr := s.Err(); scanErr != nil {
		if closeErr := f.Close(); closeErr != nil {
			return entries, closeErr
		}
		return entries, scanErr
	}
	if closeErr := f.Close(); closeErr != nil {
		return entries, closeErr
	}
	return entries, nil
}

func discoverServices() ([]serviceEntry, error) {
	if runtime.GOOS == "windows" {
		out, err := runCommand("powershell", "-NoProfile", "-NonInteractive", "-Command", "Get-CimInstance Win32_Service | Select-Object Name,PathName,StartName | ConvertTo-Json -Depth 4 -Compress")
		if err != nil {
			return nil, err
		}
		records, err := parseJSONList[windowsService](out)
		if err != nil {
			return nil, err
		}
		entries := make([]serviceEntry, 0, len(records))
		for _, record := range records {
			service := serviceEntry(record)
			if !isUserManagedWindowsService(service) {
				continue
			}
			entries = append(entries, service)
		}
		return entries, nil
	}
	out, err := runCommand("systemctl", "list-unit-files", "--type=service", "--no-pager", "--plain")
	if err != nil {
		return nil, err
	}
	entries := make([]serviceEntry, 0)
	for _, line := range splitLines(out) {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] == "UNIT" || strings.Contains(line, "UNIT FILE") {
			continue
		}
		entries = append(entries, serviceEntry{Name: fields[0]})
	}
	return entries, nil
}

func discoverPackages() ([]packageEntry, error) {
	if runtime.GOOS == "windows" {
		wingetPackages, err := discoverWingetPackagesFromExport()
		if err != nil {
			return nil, err
		}
		if len(wingetPackages) > 0 {
			return wingetPackagesToEntries(wingetPackages), nil
		}
		if _, err := exec.LookPath("choco"); err == nil {
			out, err := runCommand("choco", "list", "--local-only", "--limit-output")
			if err == nil {
				return parseChocoList(out), nil
			}
		}
		return nil, nil
	}
	if _, err := exec.LookPath("npm"); err == nil {
		out, err := runCommand("npm", "list", "-g", "--depth=0", "--json")
		if err == nil {
			return parseNPMList(out), nil
		}
	}
	return nil, nil
}

func discoverWingetPackagesFromExport() ([]wingetExportPackage, error) {
	if runtime.GOOS != "windows" {
		return nil, nil
	}
	entries := make([]wingetExportPackage, 0)
	for _, source := range []string{"winget", "msstore"} {
		packages, err := exportWingetPackages(source)
		if err != nil {
			continue
		}
		entries = append(entries, packages...)
	}
	return entries, nil
}

func exportWingetPackages(source string) ([]wingetExportPackage, error) {
	tempFile, err := os.CreateTemp("", "ironstate-winget-export-*.json")
	if err != nil {
		return nil, err
	}
	path := tempFile.Name()
	_ = tempFile.Close()
	defer func() { _ = os.Remove(path) }()

	out, err := runCommand("winget", "export", "--source", source, "--output", path, "--include-versions", "--accept-source-agreements")
	if err != nil {
		return nil, fmt.Errorf("winget export %s: %w: %s", source, err, strings.TrimSpace(out))
	}
	data, err := os.ReadFile(path) // #nosec G304 -- winget export writes to a temp file we created ourselves
	if err != nil {
		return nil, err
	}
	var exported wingetExportFile
	if err := json.Unmarshal(data, &exported); err != nil {
		return nil, err
	}
	items := make([]wingetExportPackage, 0)
	for _, sourceBlock := range exported.Sources {
		for _, pkg := range sourceBlock.Packages {
			if pkg.PackageIdentifier == "" {
				continue
			}
			items = append(items, wingetExportPackage{Identifier: pkg.PackageIdentifier, Version: pkg.Version, Source: source})
		}
	}
	return items, nil
}

func wingetPackagesToEntries(packages []wingetExportPackage) []packageEntry {
	entries := make([]packageEntry, 0, len(packages))
	msstoreNames := map[string]string{}
	for _, pkg := range packages {
		name := pkg.Identifier
		if strings.EqualFold(pkg.Source, "msstore") {
			if cached, ok := msstoreNames[pkg.Identifier]; ok {
				name = cached
			} else if displayName := wingetDisplayNameLookup(pkg.Identifier); displayName != "" {
				name = displayName
				msstoreNames[pkg.Identifier] = displayName
			}
		}
		entries = append(entries, packageEntry{Name: name, Identifier: pkg.Identifier, Source: pkg.Source})
	}
	return entries
}

var wingetDisplayNameLookup = wingetDisplayName

func wingetDisplayName(identifier string) string {
	out, err := runCommand("winget", "list", identifier, "--accept-source-agreements")
	if err != nil {
		return ""
	}
	return parseWingetDisplayName(out, identifier)
}

func parseWingetDisplayName(out, identifier string) string {
	for _, line := range splitLines(out) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "Name") || strings.Trim(trimmed, "-") == "" {
			continue
		}
		idx := strings.Index(line, identifier)
		if idx <= 0 {
			continue
		}
		name := strings.TrimSpace(line[:idx])
		if name != "" {
			return name
		}
	}
	return ""
}

func parseChocoList(out string) []packageEntry {
	items := make([]packageEntry, 0)
	for _, line := range splitLines(out) {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			items = append(items, packageEntry{Name: fields[0]})
		}
	}
	return items
}

func parseNPMList(out string) []packageEntry {
	items := make([]packageEntry, 0)
	var data map[string]any
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		return items
	}
	if deps, ok := data["dependencies"].(map[string]any); ok {
		for name := range deps {
			if strings.TrimSpace(name) != "" && name != "npm" {
				items = append(items, packageEntry{Name: name})
			}
		}
	}
	return items
}

func splitLines(s string) []string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

func runCommand(name string, args ...string) (string, error) {
	var cmd *exec.Cmd
	switch name {
	case "powershell":
		cmd = exec.Command("powershell", args...) // #nosec G204 - command is fixed and arguments are internal scan inputs
	case "systemctl":
		cmd = exec.Command("systemctl", args...) // #nosec G204 - command is fixed and arguments are internal scan inputs
	case "winget":
		cmd = exec.Command("winget", args...) // #nosec G204 - command is fixed and arguments are internal scan inputs
	case "choco":
		cmd = exec.Command("choco", args...) // #nosec G204 - command is fixed and arguments are internal scan inputs
	case "npm":
		cmd = exec.Command("npm", args...) // #nosec G204 - command is fixed and arguments are internal scan inputs
	default:
		return "", fmt.Errorf("unsupported command: %s", name)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}

func isWindowsHumanAccountSID(sid string) bool {
	trimmed := strings.TrimSpace(sid)
	if !strings.HasPrefix(trimmed, "S-1-5-21-") {
		return false
	}
	parts := strings.Split(trimmed, "-")
	if len(parts) < 8 {
		return false
	}
	last := parts[len(parts)-1]
	value, err := strconv.Atoi(last)
	if err != nil {
		return false
	}
	return value >= 1000
}

func isWindowsBuiltInGroupSID(sid string) bool {
	return strings.HasPrefix(strings.TrimSpace(sid), "S-1-5-32-")
}

func serviceExecutablePath(pathName string) string {
	trimmed := strings.TrimSpace(pathName)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "\"") {
		end := strings.Index(trimmed[1:], "\"")
		if end >= 0 {
			return trimmed[1 : 1+end]
		}
	}
	fields := strings.Fields(trimmed)
	if len(fields) > 0 {
		return fields[0]
	}
	return trimmed
}

func isUserManagedWindowsService(service serviceEntry) bool {
	exe := strings.ToLower(serviceExecutablePath(service.PathName))
	if exe == "" {
		return false
	}
	if strings.Contains(exe, `\windows\`) || strings.Contains(exe, `\windowsapps\`) {
		return false
	}
	return true
}

func isWingetManagedSource(source string) bool {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "winget", "msstore":
		return true
	default:
		return false
	}
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
		switch item.Module {
		case "user":
			itemsByRole["roles/system/users"] = append(itemsByRole["roles/system/users"], item)
		case "group":
			itemsByRole["roles/system/groups"] = append(itemsByRole["roles/system/groups"], item)
		case "service":
			itemsByRole["roles/system/services"] = append(itemsByRole["roles/system/services"], item)
		default:
			itemsByRole["roles/packages"] = append(itemsByRole["roles/packages"], item)
		}
	}

	if err := writeYAML(filepath.Join(target, "site.yml"), map[string]any{
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
