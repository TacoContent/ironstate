package remoteexec

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Host is one target: a display name plus how to reach it.
type Host struct {
	Name                  string
	Local                 bool
	SSH                   SSHTarget
	Platform              string // optional hint: linux | darwin | windows
	AgentDir              string // overrides --remote-agent-dir for this host
	DisableBecome         bool   // inventory become:false runs playbook tasks as the SSH user
	SkipAgentVerification bool
}

// Address renders how the host is reached.
func (h Host) Address() string {
	if h.Local {
		return LocalTarget
	}
	return h.SSH.String()
}

// HostFromTarget turns a --target value into a Host named after it.
func HostFromTarget(target string) (Host, error) {
	if target == LocalTarget {
		return Host{Name: LocalTarget, Local: true}, nil
	}
	t, err := ParseSSHTarget(target)
	if err != nil {
		return Host{}, err
	}
	return Host{Name: target, SSH: t}, nil
}

// Inventory is a parsed inventory file (connection/elevation settings only;
// per-host configuration belongs in the playbook's hosts/ and variables/ overlays).
type Inventory struct {
	Defaults HostSpec            `yaml:"defaults"`
	Hosts    map[string]HostSpec `yaml:"hosts"`
	Groups   map[string][]string `yaml:"groups"`

	hosts map[string]Host
}

// HostSpec is one inventory entry; empty fields fall back to defaults.
type HostSpec struct {
	Address     string `yaml:"address"`
	User        string `yaml:"user"`
	Port        int    `yaml:"port"`
	Platform    string `yaml:"platform"`
	AgentDir    string `yaml:"agent_dir"`
	Become      *bool  `yaml:"become"`
	VerifyAgent *bool  `yaml:"verify_agent"`
}

// AllGroup selects every inventory host.
const AllGroup = "all"

var inventoryNamePattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*$`)

// LoadInventory reads and validates an inventory file or directory. Unknown
// keys are errors so typos don't silently drop settings.
func LoadInventory(path string) (*Inventory, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return loadInventoryDirectory(path)
	}
	return loadInventoryFile(path)
}

func loadInventoryFile(path string) (*Inventory, error) {
	data, err := os.ReadFile(path) //nolint:gosec // operator-chosen inventory file
	if err != nil {
		return nil, err
	}
	var inv Inventory
	if err := decodeInventory(data, &inv); err != nil {
		return nil, fmt.Errorf("inventory %s: %w", path, err)
	}
	if err := inv.build(); err != nil {
		return nil, fmt.Errorf("inventory %s: %w", path, err)
	}
	return &inv, nil
}

func loadInventoryDirectory(path string) (*Inventory, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".yml" || ext == ".yaml" {
			files = append(files, filepath.Join(path, entry.Name()))
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("inventory directory %s contains no .yml or .yaml files", path)
	}

	primary := ""
	for _, file := range files {
		switch filepath.Base(file) {
		case "main.yml":
			primary = file
		case "main.yaml":
			if primary == "" {
				primary = file
			}
		}
	}
	sort.Slice(files, func(i, j int) bool {
		if primary != "" && files[i] == primary {
			return true
		}
		if primary != "" && files[j] == primary {
			return false
		}
		return filepath.Base(files[i]) < filepath.Base(files[j])
	})

	var merged yaml.Node
	for i, file := range files {
		data, err := os.ReadFile(file) //nolint:gosec // operator-chosen inventory directory
		if err != nil {
			return nil, fmt.Errorf("inventory %s: %w", file, err)
		}

		// Validate each fragment before merging, so an invalid key cannot be
		// hidden by a later fragment that overrides it.
		var fragment Inventory
		if err := decodeInventory(data, &fragment); err != nil {
			return nil, fmt.Errorf("inventory %s: %w", file, err)
		}

		var document yaml.Node
		dec := yaml.NewDecoder(bytes.NewReader(data))
		if err := dec.Decode(&document); err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("inventory %s: %w", file, err)
		}
		root := documentRoot(&document)
		if i == 0 {
			merged = *root
		} else {
			mergeYAMLNode(&merged, root)
		}
	}

	var inv Inventory
	if err := merged.Decode(&inv); err != nil {
		return nil, fmt.Errorf("inventory directory %s: %w", path, err)
	}
	if err := inv.build(); err != nil {
		return nil, fmt.Errorf("inventory directory %s: %w", path, err)
	}
	return &inv, nil
}

func decodeInventory(data []byte, inv *Inventory) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(inv); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func documentRoot(document *yaml.Node) *yaml.Node {
	if document.Kind == yaml.DocumentNode && len(document.Content) > 0 {
		return document.Content[0]
	}
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
}

func mergeYAMLNode(dst, src *yaml.Node) {
	if src.Tag == "!!null" {
		return
	}
	if dst.Kind == yaml.SequenceNode && src.Kind == yaml.SequenceNode {
		dst.Content = append(dst.Content, src.Content...)
		return
	}
	if dst.Kind != yaml.MappingNode || src.Kind != yaml.MappingNode {
		*dst = *src
		return
	}
	for i := 0; i+1 < len(src.Content); i += 2 {
		key, value := src.Content[i], src.Content[i+1]
		var existing *yaml.Node
		for j := 0; j+1 < len(dst.Content); j += 2 {
			if dst.Content[j].Value == key.Value {
				existing = dst.Content[j+1]
				break
			}
		}
		if existing == nil {
			dst.Content = append(dst.Content, key, value)
			continue
		}
		mergeYAMLNode(existing, value)
	}
}

func (inv *Inventory) build() error {
	inv.hosts = map[string]Host{}
	for name, spec := range inv.Hosts {
		if !inventoryNamePattern.MatchString(name) || name == AllGroup {
			return fmt.Errorf("invalid host name %q", name)
		}
		host, err := inv.resolve(name, spec)
		if err != nil {
			return fmt.Errorf("host %q: %w", name, err)
		}
		inv.hosts[name] = host
	}
	for group, members := range inv.Groups {
		if !inventoryNamePattern.MatchString(group) || group == AllGroup {
			return fmt.Errorf("invalid group name %q", group)
		}
		if _, clash := inv.hosts[group]; clash {
			return fmt.Errorf("group %q has the same name as a host", group)
		}
		for _, member := range members {
			if _, ok := inv.hosts[member]; !ok {
				return fmt.Errorf("group %q lists unknown host %q", group, member)
			}
		}
	}
	return nil
}

func (inv *Inventory) resolve(name string, spec HostSpec) (Host, error) {
	pick := func(v, def string) string {
		if v != "" {
			return v
		}
		return def
	}
	address := pick(spec.Address, inv.Defaults.Address)
	if address == "" {
		address = name
	}
	host := Host{
		Name:          name,
		Platform:      pick(spec.Platform, inv.Defaults.Platform),
		AgentDir:      pick(spec.AgentDir, inv.Defaults.AgentDir),
		DisableBecome: false,
	}
	become := true
	if inv.Defaults.Become != nil {
		become = *inv.Defaults.Become
	}
	if spec.Become != nil {
		become = *spec.Become
	}
	host.DisableBecome = !become
	verifyAgent := true
	if inv.Defaults.VerifyAgent != nil {
		verifyAgent = *inv.Defaults.VerifyAgent
	}
	if spec.VerifyAgent != nil {
		verifyAgent = *spec.VerifyAgent
	}
	host.SkipAgentVerification = !verifyAgent
	switch host.Platform {
	case "", "linux", "darwin", "windows":
	default:
		return host, fmt.Errorf("unknown platform %q (want linux, darwin or windows)", host.Platform)
	}
	if address == LocalTarget {
		host.Local = true
		return host, nil
	}
	target := address
	if isIPv6(address) {
		target = "[" + address + "]"
	}
	if user := pick(spec.User, inv.Defaults.User); user != "" {
		target = user + "@" + target
	}
	port := spec.Port
	if port == 0 {
		port = inv.Defaults.Port
	}
	if port != 0 {
		target += ":" + strconv.Itoa(port)
	}
	parsed, err := ParseSSHTarget(target)
	if err != nil {
		return host, err
	}
	host.SSH = parsed
	return host, nil
}

func isIPv6(address string) bool {
	return sshIPv6Pattern.MatchString("[" + address + "]")
}

// Select returns the hosts named by limit (host or group names, or "all"),
// in the order given and without duplicates. An empty limit selects every
// host, sorted by name.
func (inv *Inventory) Select(limit []string) ([]Host, error) {
	if len(limit) == 0 {
		limit = []string{AllGroup}
	}
	var out []Host
	seen := map[string]bool{}
	add := func(name string) {
		if !seen[name] {
			seen[name] = true
			out = append(out, inv.hosts[name])
		}
	}
	for _, name := range limit {
		switch {
		case name == AllGroup:
			names := make([]string, 0, len(inv.hosts))
			for n := range inv.hosts {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				add(n)
			}
		case inv.Groups[name] != nil:
			for _, member := range inv.Groups[name] {
				add(member)
			}
		default:
			if _, ok := inv.hosts[name]; !ok {
				return nil, fmt.Errorf("--limit %q is not a host or group in the inventory", name)
			}
			add(name)
		}
	}
	return out, nil
}
