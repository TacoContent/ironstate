package handlers

import (
	"fmt"
	"os"
	"runtime"
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
