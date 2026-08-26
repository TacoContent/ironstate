package handlers

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// advFirewallHandler manages Windows firewall rules via
// 'netsh advfirewall firewall'.
type advFirewallHandler struct{}

func advRuleName(item map[string]any) string {
	if v := strings.TrimSpace(getString(item, "name")); v != "" {
		return v
	}
	if v := strings.TrimSpace(getString(item, "rule_name")); v != "" {
		return v
	}
	return ""
}

func advDirection(item map[string]any) string {
	d := strings.ToLower(strings.TrimSpace(getString(item, "direction")))
	if d == "out" {
		return "out"
	}
	return "in"
}

func advAction(item map[string]any) string {
	a := strings.ToLower(strings.TrimSpace(getString(item, "action")))
	if a == "deny" || a == "reject" || a == "drop" || a == "block" {
		return "block"
	}
	return "allow"
}

func advProtocol(item map[string]any) string {
	p := strings.ToUpper(strings.TrimSpace(getString(item, "protocol")))
	if p == "" || p == "ANY" || p == "ALL" {
		return "ANY"
	}
	return p
}

func (advFirewallHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	if runtime.GOOS != "windows" {
		return false, fmt.Errorf("advfirewall is only supported on Windows")
	}
	ruleName := advRuleName(item)
	if ruleName == "" {
		return false, fmt.Errorf("advfirewall requires 'name' (or alias 'rule_name')")
	}
	res, err := runner.Run("netsh", []string{"advfirewall", "firewall", "show", "rule", "name=" + ruleName})
	if err != nil {
		return false, nil //nolint:nilerr // treat probe failures as not-installed
	}
	lower := strings.ToLower(res.Stdout + "\n" + res.Stderr)
	exists := res.RC == 0 && !strings.Contains(lower, "no rules match")
	if itemState(item) == "absent" {
		return exists, nil
	}
	return exists, nil
}

func (advFirewallHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	ruleName := advRuleName(item)
	if action == engine.ActionUninstall {
		return "delete advfirewall rule " + ruleName, nil
	}
	return "ensure advfirewall rule " + ruleName, nil
}

func (advFirewallHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	if runtime.GOOS != "windows" {
		return engine.ExecResult{}, fmt.Errorf("advfirewall is only supported on Windows")
	}
	ruleName := advRuleName(item)
	if ruleName == "" {
		return engine.ExecResult{}, fmt.Errorf("advfirewall requires 'name' (or alias 'rule_name')")
	}
	args := []string{"advfirewall", "firewall", "add", "rule", "name=" + ruleName, "dir=" + advDirection(item), "action=" + advAction(item), "protocol=" + advProtocol(item)}

	if enable := strings.TrimSpace(getString(item, "enable")); enable != "" {
		args = append(args, "enable="+enable)
	} else {
		args = append(args, "enable=yes")
	}

	if localPort := strings.TrimSpace(getString(item, "local_port")); localPort != "" {
		args = append(args, "localport="+localPort)
	} else if port := strings.TrimSpace(getString(item, "port")); port != "" {
		args = append(args, "localport="+port)
	}
	if remotePort := strings.TrimSpace(getString(item, "remote_port")); remotePort != "" {
		args = append(args, "remoteport="+remotePort)
	}
	if source := strings.TrimSpace(getString(item, "source")); source != "" {
		args = append(args, "remoteip="+source)
	}
	if destination := strings.TrimSpace(getString(item, "destination")); destination != "" {
		args = append(args, "localip="+destination)
	}
	if program := strings.TrimSpace(getString(item, "program")); program != "" {
		args = append(args, "program="+program)
	}
	if profile := strings.TrimSpace(getString(item, "profile")); profile != "" {
		args = append(args, "profile="+profile)
	}

	res := runExternalCommand("netsh", args)
	if res.RC != 0 {
		engine.Warn("netsh advfirewall add rule exited with code %d", res.RC)
	}
	return res, nil
}

func (advFirewallHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	if runtime.GOOS != "windows" {
		return engine.ExecResult{}, fmt.Errorf("advfirewall is only supported on Windows")
	}
	ruleName := advRuleName(item)
	if ruleName == "" {
		return engine.ExecResult{}, fmt.Errorf("advfirewall requires 'name' (or alias 'rule_name')")
	}
	res := runExternalCommand("netsh", []string{"advfirewall", "firewall", "delete", "rule", "name=" + ruleName})
	if res.RC != 0 {
		engine.Warn("netsh advfirewall delete rule exited with code %d", res.RC)
	}
	return res, nil
}
