package handlers

import (
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
	"gopkg.in/yaml.v3"
)

// GitHub release binaries via xget.
type xgetHandler struct{}

func (xgetHandler) Emoji() string           { return "📦" }
func (xgetHandler) RequiredTools() []string { return []string{"xget"} }

var xgetToArgPattern = regexp.MustCompile(`^--to=(.+)$`)

func xgetExpandedArgs(item map[string]any) []string {
	var out []string
	for _, raw := range asList(item["args"]) {
		s, ok := raw.(string)
		if !ok {
			continue
		}
		if m := xgetToArgPattern.FindStringSubmatch(s); m != nil {
			out = append(out, "--to="+resolvePath(m[1]))
		} else {
			out = append(out, s)
		}
	}
	return out
}

func xgetTargetPath(item map[string]any) string {
	for _, arg := range xgetExpandedArgs(item) {
		if m := xgetToArgPattern.FindStringSubmatch(arg); m != nil {
			return m[1]
		}
	}
	return ""
}

func (xgetHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	target := xgetTargetPath(item)
	if target == "" {
		return false, nil
	}
	return fileExists(target), nil
}

func (xgetHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	pkg := getString(item, "package")
	if action == engine.ActionUninstall {
		return "remove " + xgetTargetPath(item), nil
	}
	desc := "xget " + pkg
	for _, a := range xgetExpandedArgs(item) {
		desc += " " + a
	}
	return desc, nil
}

func (xgetHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkg := getString(item, "package")
	args := append([]string{pkg}, xgetExpandedArgs(item)...)
	result := runExternalCommand("xget", args)
	if result.RC != 0 {
		engine.Warn("xget %s exited with code %d", pkg, result.RC)
	}
	return result, nil
}

func (xgetHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	target := xgetTargetPath(item)
	if target != "" && fileExists(target) {
		_ = os.Remove(target)
	}
	return engine.ExecResult{}, nil
}

// ScanRole implements engine.ScanCapable - discovered packages seed
// roles/packages in a generated playbook (see internal/scan).
func (xgetHandler) ScanRole() string { return "roles/packages/xget" }

// xgetInstalledFile mirrors the shape of xget's own install-record
// database at ~/.config/xget/.xget.installed.yml (see xgetInstalledPath),
// keyed by "<source>:<package>" (e.g. "github:camalot/xget"). Each entry
// is a one-element list in xget's own file; only Name and Options matter
// for reconstructing a playbook item.
type xgetInstalledFile struct {
	Packages map[string][]struct {
		Name    string         `yaml:"name"`
		Options map[string]any `yaml:"options"`
	} `yaml:"packages"`
}

// xgetInstalledPath is where xget records what it has installed.
func xgetInstalledPath() string {
	return resolvePath("~/.config/xget/.xget.installed.yml")
}

// xgetOptionArgNames maps an install record's 'options' key to its xget
// CLI flag name where the two don't just differ by kebab vs. snake case
// (e.g. the recorded 'output' option is xget's '--to' flag).
var xgetOptionArgNames = map[string]string{
	"output":        "to",
	"installed_tag": "tag",
}

// xgetOptionArgs turns one install record's 'options' map back into the
// '--flag'/'--flag=value' args xget's own CLI expects. Option keys are
// snake_case (e.g. 'upgrade_only') while the matching flags are
// kebab-case ('--upgrade-only'), unless renamed by xgetOptionArgNames; a
// bool option becomes a bare flag only when true, a list option repeats
// the flag once per entry, and anything else becomes '--flag=value'.
func xgetOptionArgs(options map[string]any) []string {
	keys := make([]string, 0, len(options))
	for k := range options {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []string
	for _, key := range keys {
		name, ok := xgetOptionArgNames[key]
		if !ok {
			name = strings.ReplaceAll(key, "_", "-")
		}
		flag := "--" + name
		switch v := options[key].(type) {
		case bool:
			if v {
				out = append(out, flag)
			}
		case []any:
			for _, raw := range v {
				if s, ok := raw.(string); ok {
					out = append(out, flag+"="+s)
				}
			}
		case string:
			out = append(out, flag+"="+v)
		default:
			// numbers and anything else: best-effort string form.
			out = append(out, flag)
		}
	}
	return out
}

// Scan implements engine.ScanCapable: discovers packages xget has
// installed by reading its own install-record file.
func (xgetHandler) Scan(ctx engine.Context) ([]engine.ScanItem, error) {
	data, err := os.ReadFile(xgetInstalledPath())
	if err != nil {
		return nil, nil //nolint:nilerr // no record file just means nothing to report
	}
	var installed xgetInstalledFile
	if err := yaml.Unmarshal(data, &installed); err != nil {
		return nil, nil //nolint:nilerr // unparseable record file just means nothing to report
	}

	keys := make([]string, 0, len(installed.Packages))
	for k := range installed.Packages {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]engine.ScanItem, 0)
	for _, key := range keys {
		for _, entry := range installed.Packages[key] {
			pkg := strings.TrimSpace(entry.Name)
			if pkg == "" {
				continue
			}
			out = append(out, engine.ScanItem{
				Module: "xget",
				Name:   pkg,
				Config: map[string]any{
					"package": pkg,
					"state":   "present",
					"args":    xgetOptionArgs(entry.Options),
				},
				Tags: []string{"packages"},
			})
		}
	}
	return out, nil
}
