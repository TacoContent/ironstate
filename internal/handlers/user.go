package handlers

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// userHandler manages local users across Windows/Linux/macOS.
type userHandler struct{}

func userName(item map[string]any) string {
	if name := strings.TrimSpace(getString(item, "name")); name != "" {
		return name
	}
	return strings.TrimSpace(getString(item, "user"))
}

func userPassword(item map[string]any) string {
	if envName := strings.TrimSpace(getString(item, "password_env")); envName != "" {
		if v := os.Getenv(envName); v != "" {
			return v
		}
	}
	return getString(item, "password")
}

func userGroupsList(item map[string]any) []string {
	out := []string{}
	for _, raw := range asList(item["groups"]) {
		s := strings.TrimSpace(fmt.Sprintf("%v", raw))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func userExists(name string) (bool, error) {
	if name == "" {
		return false, fmt.Errorf("user requires 'name'")
	}
	switch runtime.GOOS {
	case "windows":
		res, err := runner.Run("net", []string{"user", name})
		if err != nil {
			return false, nil
		}
		return res.RC == 0, nil
	case "darwin":
		res, err := runner.Run("dscl", []string{".", "-read", "/Users/" + name})
		if err != nil {
			return false, nil
		}
		return res.RC == 0, nil
	default:
		res, err := runner.Run("id", []string{"-u", name})
		if err != nil {
			return false, nil
		}
		return res.RC == 0, nil
	}
}

func installUser(item map[string]any) (engine.ExecResult, error) {
	name := userName(item)
	exists, err := userExists(name)
	if err != nil {
		return engine.ExecResult{}, err
	}
	if exists {
		return engine.ExecResult{RC: 0}, nil
	}

	password := userPassword(item)
	groups := userGroupsList(item)
	primaryGroup := strings.TrimSpace(getString(item, "group"))
	if primaryGroup == "" {
		primaryGroup = strings.TrimSpace(getString(item, "gid"))
	}
	shell := strings.TrimSpace(getString(item, "shell"))
	home := strings.TrimSpace(getString(item, "home"))
	comment := strings.TrimSpace(getString(item, "comment"))
	system := getBool(item, "system", false)

	switch runtime.GOOS {
	case "windows":
		args := []string{"user", name, password, "/add"}
		res := runExternalCommand("net", args)
		if res.RC != 0 {
			return res, nil
		}
		if comment != "" {
			_ = runExternalCommand("net", []string{"user", name, "/fullname:" + comment})
		}
		for _, g := range groups {
			_ = runExternalCommand("net", []string{"localgroup", g, name, "/add"})
		}
		if primaryGroup != "" {
			_ = runExternalCommand("net", []string{"localgroup", primaryGroup, name, "/add"})
		}
		if shell != "" || home != "" || system {
			engine.Warn("user shell/home/system are ignored on Windows")
		}
		return res, nil
	case "darwin":
		if password == "" {
			password = "*"
		}
		args := []string{"-addUser", name, "-password", password}
		if shell != "" {
			args = append(args, "-shell", shell)
		}
		if home != "" {
			args = append(args, "-home", home)
		}
		if comment != "" {
			args = append(args, "-fullName", comment)
		}
		if uid := strings.TrimSpace(getString(item, "uid")); uid != "" {
			args = append(args, "-UID", uid)
		}
		res := runExternalCommand("sysadminctl", args)
		if res.RC != 0 {
			return res, nil
		}
		if primaryGroup != "" {
			_ = runExternalCommand("dseditgroup", []string{"-o", "edit", "-a", name, "-t", "user", primaryGroup})
		}
		for _, g := range groups {
			_ = runExternalCommand("dseditgroup", []string{"-o", "edit", "-a", name, "-t", "user", g})
		}
		if system {
			engine.Warn("user system is ignored on macOS")
		}
		return res, nil
	default:
		args := []string{}
		if system {
			args = append(args, "--system")
		}
		if uid := strings.TrimSpace(getString(item, "uid")); uid != "" {
			args = append(args, "--uid", uid)
		}
		if primaryGroup != "" {
			args = append(args, "--gid", primaryGroup)
		}
		if len(groups) > 0 {
			args = append(args, "--groups", strings.Join(groups, ","))
		}
		if shell != "" {
			args = append(args, "--shell", shell)
		}
		if home != "" {
			args = append(args, "--home-dir", home)
		}
		if comment != "" {
			args = append(args, "--comment", comment)
		}
		if password != "" {
			args = append(args, "--password", password)
		}
		if getBool(item, "create_home", true) {
			args = append(args, "--create-home")
		} else {
			args = append(args, "--no-create-home")
		}
		args = append(args, name)
		res := runExternalCommand("useradd", args)
		return res, nil
	}
}

func uninstallUser(item map[string]any) (engine.ExecResult, error) {
	name := userName(item)
	exists, err := userExists(name)
	if err != nil {
		return engine.ExecResult{}, err
	}
	if !exists {
		return engine.ExecResult{RC: 0}, nil
	}

	switch runtime.GOOS {
	case "windows":
		res := runExternalCommand("net", []string{"user", name, "/delete"})
		return res, nil
	case "darwin":
		res := runExternalCommand("sysadminctl", []string{"-deleteUser", name})
		return res, nil
	default:
		args := []string{}
		if getBool(item, "remove_home", false) {
			args = append(args, "-r")
		}
		args = append(args, name)
		res := runExternalCommand("userdel", args)
		return res, nil
	}
}

func (userHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	exists, err := userExists(userName(item))
	if err != nil {
		return false, err
	}
	if itemState(item) == "absent" {
		return exists, nil
	}
	return exists, nil
}

func (userHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	name := userName(item)
	if name == "" {
		return "", fmt.Errorf("user requires 'name'")
	}
	if action == engine.ActionUninstall {
		return "remove user " + name, nil
	}
	return "ensure user " + name, nil
}

func (userHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return installUser(item)
}

func (userHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return uninstallUser(item)
}

// ScanRole implements engine.ScanCapable - discovered users seed
// roles/system/users in a generated playbook (see internal/scan).
func (userHandler) ScanRole() string { return "roles/system/users" }

// Scan implements engine.ScanCapable: discovers the host's local user
// accounts, filtering out OS builtin/service accounts so only
// user-managed accounts show up in a generated playbook - ports the
// scanning logic that used to live in internal/scan's userScanner.
func (userHandler) Scan(ctx engine.Context) ([]engine.ScanItem, error) {
	entries, err := readPasswdUsers()
	if err != nil {
		return nil, err
	}
	out := make([]engine.ScanItem, 0, len(entries))
	for _, e := range entries {
		if e.Username == "" {
			continue
		}
		if runtime.GOOS == "windows" && !isWindowsHumanAccountSID(e.SID) {
			continue
		}
		if runtime.GOOS == "darwin" && isMacOSBuiltinUser(e.Username) {
			continue
		}
		if runtime.GOOS == "linux" && isLinuxBuiltinUser(e.Username) {
			continue
		}
		out = append(out, engine.ScanItem{
			Module: "user",
			Name:   e.Username,
			Config: map[string]any{"name": e.Username, "home": e.Home, "shell": e.Shell, "state": "present"},
			Tags:   []string{"system", "users"},
		})
	}
	return out, nil
}

// passwdUser is one row read from /etc/passwd (or Get-LocalUser on
// Windows) - see readPasswdUsers.
type passwdUser struct {
	Username string
	Home     string
	Shell    string
	SID      string
}

type windowsLocalUser struct {
	Name string `json:"Name"`
	SID  string `json:"SID"`
}

func readPasswdUsers() ([]passwdUser, error) {
	if runtime.GOOS == "windows" {
		return parseWindowsUsers()
	}
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return nil, err
	}

	entries := make([]passwdUser, 0)
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
		entries = append(entries, passwdUser{Username: parts[0], Home: parts[5], Shell: parts[6]})
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

func parseWindowsUsers() ([]passwdUser, error) {
	result, err := runner.Run("pwsh", []string{"-NoProfile", "-NonInteractive", "-Command", "Get-LocalUser | Select-Object Name,SID | ConvertTo-Json -Depth 4 -Compress"})
	if err != nil {
		return nil, err
	}
	records, err := parseJSONList[windowsLocalUser](result.Stdout)
	if err != nil {
		return nil, err
	}
	entries := make([]passwdUser, 0, len(records))
	for _, record := range records {
		if record.Name == "" || !isWindowsHumanAccountSID(record.SID) {
			continue
		}
		entries = append(entries, passwdUser{Username: record.Name, SID: record.SID})
	}
	return entries, nil
}

// isWindowsHumanAccountSID reports whether sid is a real, human local
// account SID (S-1-5-21-...-<RID> with RID >= 1000), excluding Windows'
// built-in/service accounts (Administrator, Guest, DefaultAccount, ...).
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

// macOSBuiltinUsers lists macOS system accounts kept out of a 'users'
// scan even though they don't use the '_'-prefixed service-account
// naming convention (see isMacOSBuiltinUser).
var macOSBuiltinUsers = map[string]bool{
	"daemon": true,
	"nobody": true,
	"root":   true,
}

// isMacOSBuiltinUser reports whether name is a macOS system account that
// should never show up as a user-managed 'user' scan item: every
// '_'-prefixed service account (ports Apple's own convention for daemon
// accounts, e.g. '_www', '_spotlight') plus the small set of legacy
// unprefixed system accounts in macOSBuiltinUsers.
func isMacOSBuiltinUser(name string) bool {
	if strings.HasPrefix(name, "_") {
		return true
	}
	return macOSBuiltinUsers[name]
}

// linuxBuiltinUsers lists Debian/Ubuntu-style system accounts kept out of
// a 'users' scan even though they don't use the '_'-prefixed
// service-account naming convention (see isLinuxBuiltinUser). Distro-
// specific and not exhaustive - a starting point, per-host overrides may
// become configurable later.
var linuxBuiltinUsers = map[string]bool{
	"backup":          true,
	"bin":             true,
	"daemon":          true,
	"dhcpcd":          true,
	"games":           true,
	"irc":             true,
	"list":            true,
	"lp":              true,
	"mail":            true,
	"man":             true,
	"messagebus":      true,
	"news":            true,
	"nobody":          true,
	"proxy":           true,
	"root":            true,
	"sync":            true,
	"sys":             true,
	"systemd-network": true,
	"uucp":            true,
	"uuidd":           true,
	"www-data":        true,
}

// isLinuxBuiltinUser reports whether name is a Linux system account that
// should never show up as a user-managed 'user' scan item: every
// '_'-prefixed service account (e.g. '_apt') plus the named system
// accounts in linuxBuiltinUsers.
func isLinuxBuiltinUser(name string) bool {
	if strings.HasPrefix(name, "_") {
		return true
	}
	return linuxBuiltinUsers[name]
}
