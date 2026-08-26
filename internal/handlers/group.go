package handlers

import (
	"fmt"
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
