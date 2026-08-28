package handlers

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// groupHandler manages local groups across Windows/Linux/macOS.
type groupHandler struct{}

func groupName(item map[string]any) string {
	if name := strings.TrimSpace(getString(item, "name")); name != "" {
		return name
	}
	return strings.TrimSpace(getString(item, "group"))
}

func groupExists(name string) (bool, error) {
	if name == "" {
		return false, fmt.Errorf("group requires 'name'")
	}
	switch runtime.GOOS {
	case "windows":
		res, err := runner.Run("net", []string{"localgroup", name})
		if err != nil {
			return false, nil
		}
		return res.RC == 0, nil
	case "darwin":
		res, err := runner.Run("dscl", []string{".", "-read", "/Groups/" + name})
		if err != nil {
			return false, nil
		}
		return res.RC == 0, nil
	default:
		res, err := runner.Run("getent", []string{"group", name})
		if err != nil {
			return false, nil
		}
		return res.RC == 0, nil
	}
}

func installGroup(item map[string]any) (engine.ExecResult, error) {
	name := groupName(item)
	exists, err := groupExists(name)
	if err != nil {
		return engine.ExecResult{}, err
	}
	if exists {
		return engine.ExecResult{RC: 0}, nil
	}

	gid := strings.TrimSpace(getString(item, "gid"))
	system := getBool(item, "system", false)

	switch runtime.GOOS {
	case "windows":
		if gid != "" || system {
			engine.Warn("group gid/system are ignored on Windows")
		}
		res := runExternalCommand("net", []string{"localgroup", name, "/add"})
		return res, nil
	case "darwin":
		args := []string{"-o", "create"}
		if gid != "" {
			args = append(args, "-i", gid)
		}
		args = append(args, name)
		res := runExternalCommand("dseditgroup", args)
		return res, nil
	default:
		args := []string{}
		if system {
			args = append(args, "--system")
		}
		if gid != "" {
			args = append(args, "--gid", gid)
		}
		args = append(args, name)
		res := runExternalCommand("groupadd", args)
		return res, nil
	}
}

func uninstallGroup(item map[string]any) (engine.ExecResult, error) {
	name := groupName(item)
	exists, err := groupExists(name)
	if err != nil {
		return engine.ExecResult{}, err
	}
	if !exists {
		return engine.ExecResult{RC: 0}, nil
	}

	switch runtime.GOOS {
	case "windows":
		res := runExternalCommand("net", []string{"localgroup", name, "/delete"})
		return res, nil
	case "darwin":
		res := runExternalCommand("dseditgroup", []string{"-o", "delete", name})
		return res, nil
	default:
		res := runExternalCommand("groupdel", []string{name})
		return res, nil
	}
}

func (groupHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	exists, err := groupExists(groupName(item))
	if err != nil {
		return false, err
	}
	if itemState(item) == "absent" {
		return exists, nil
	}
	return exists, nil
}

func (groupHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	name := groupName(item)
	if name == "" {
		return "", fmt.Errorf("group requires 'name'")
	}
	if action == engine.ActionUninstall {
		return "remove group " + name, nil
	}
	return "ensure group " + name, nil
}

func (groupHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return installGroup(item)
}

func (groupHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return uninstallGroup(item)
}

// ScanRole implements engine.ScanCapable - discovered groups seed
// roles/system/groups in a generated playbook (see internal/scan).
func (groupHandler) ScanRole() string { return "roles/system/groups" }

// Scan implements engine.ScanCapable: discovers the host's local groups,
// filtering out OS builtin/service groups so only user-managed groups
// show up in a generated playbook - ports the scanning logic that used
// to live in internal/scan's groupScanner.
func (groupHandler) Scan(ctx engine.Context) ([]engine.ScanItem, error) {
	entries, err := readGroupEntries()
	if err != nil {
		return nil, err
	}
	out := make([]engine.ScanItem, 0, len(entries))
	for _, e := range entries {
		if e.Name == "" {
			continue
		}
		if runtime.GOOS == "windows" && isWindowsBuiltInGroupSID(e.SID) {
			continue
		}
		if runtime.GOOS == "darwin" && isMacOSBuiltinGroup(e.Name) {
			continue
		}
		if runtime.GOOS == "linux" && isLinuxBuiltinGroup(e.Name) {
			continue
		}
		out = append(out, engine.ScanItem{
			Module: "group",
			Name:   e.Name,
			Config: map[string]any{"name": e.Name, "gid": e.GID, "state": "present"},
			Tags:   []string{"system", "groups"},
		})
	}
	return out, nil
}

// groupEntry is one row read from /etc/group (or Get-LocalGroup on
// Windows) - see readGroupEntries.
type groupEntry struct {
	Name string
	GID  string
	SID  string
}

type windowsLocalGroup struct {
	Name string `json:"Name"`
	SID  string `json:"SID"`
}

func readGroupEntries() ([]groupEntry, error) {
	if runtime.GOOS == "windows" {
		result, err := runner.Run("pwsh", []string{"-NoProfile", "-NonInteractive", "-Command", "Get-LocalGroup | Select-Object Name,SID | ConvertTo-Json -Depth 4 -Compress"})
		if err != nil {
			return nil, err
		}
		records, err := parseJSONList[windowsLocalGroup](result.Stdout)
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

// isWindowsBuiltInGroupSID reports whether sid is one of Windows' own
// built-in local groups (S-1-5-32-*, e.g. Administrators, Users).
func isWindowsBuiltInGroupSID(sid string) bool {
	return strings.HasPrefix(strings.TrimSpace(sid), "S-1-5-32-")
}

// macOSBuiltinGroups lists macOS system/service groups kept out of a
// 'groups' scan. Every 'com.apple.*' directory-service group
// (access_ssh, access_screensharing, ...) is covered by the prefix check
// in isMacOSBuiltinGroup instead of being enumerated here.
var macOSBuiltinGroups = map[string]bool{
	"accessibility": true,
	"admin":         true,
	"authedusers":   true,
	"bin":           true,
	"certusers":     true,
	"consoleusers":  true,
	"daemon":        true,
	"dialer":        true,
	"everyone":      true,
	"group":         true,
	"interactusers": true,
	"kmem":          true,
	"localaccounts": true,
	"mail":          true,
	"netaccounts":   true,
	"netusers":      true,
	"network":       true,
	"nobody":        true,
	"nogroup":       true,
	"operator":      true,
	"owner":         true,
	"procmod":       true,
	"procview":      true,
	"staff":         true,
	"sys":           true,
	"tty":           true,
	"utmp":          true,
	"wheel":         true,
}

// isMacOSBuiltinGroup reports whether name is a macOS system/service
// group that should never show up as a user-managed 'group' scan item:
// every '_'-prefixed service group, every 'com.apple.*' directory-service
// group, plus the legacy unprefixed system groups in macOSBuiltinGroups.
func isMacOSBuiltinGroup(name string) bool {
	if strings.HasPrefix(name, "_") || strings.HasPrefix(name, "com.apple.") {
		return true
	}
	return macOSBuiltinGroups[name]
}

// linuxBuiltinGroups lists Debian/Ubuntu-style system/service groups kept
// out of a 'groups' scan. Same caveats as linuxBuiltinUsers.
var linuxBuiltinGroups = map[string]bool{
	"adm":             true,
	"audio":           true,
	"backup":          true,
	"bin":             true,
	"cdrom":           true,
	"clock":           true,
	"crontab":         true,
	"daemon":          true,
	"dialout":         true,
	"dip":             true,
	"disk":            true,
	"docker":          true,
	"fax":             true,
	"floppy":          true,
	"games":           true,
	"input":           true,
	"irc":             true,
	"kmem":            true,
	"kvm":             true,
	"list":            true,
	"lp":              true,
	"mail":            true,
	"man":             true,
	"messagebus":      true,
	"netdev":          true,
	"news":            true,
	"nogroup":         true,
	"operator":        true,
	"plugdev":         true,
	"proxy":           true,
	"render":          true,
	"root":            true,
	"sasl":            true,
	"sgx":             true,
	"shadow":          true,
	"src":             true,
	"staff":           true,
	"sudo":            true,
	"sys":             true,
	"systemd-journal": true,
	"systemd-network": true,
	"tape":            true,
	"tty":             true,
	"users":           true,
	"utmp":            true,
	"uucp":            true,
	"uuidd":           true,
	"video":           true,
	"voice":           true,
	"www-data":        true,
}

// isLinuxBuiltinGroup reports whether name is a Linux system/service
// group that should never show up as a user-managed 'group' scan item:
// every '_'-prefixed service group (e.g. '_ssh') plus the named system
// groups in linuxBuiltinGroups.
func isLinuxBuiltinGroup(name string) bool {
	if strings.HasPrefix(name, "_") {
		return true
	}
	return linuxBuiltinGroups[name]
}
