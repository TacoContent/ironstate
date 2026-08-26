package handlers

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// firewallHandler is a cross-platform wrapper that translates one unified
// firewall rule intent into a platform/backend-specific handler.
type firewallHandler struct{}

func firewallBackend(item map[string]any, ctx engine.Context) (string, error) {
	if explicit := strings.ToLower(strings.TrimSpace(getString(item, "backend"))); explicit != "" && explicit != "auto" {
		switch explicit {
		case "iptables", "ufw", "advfirewall":
			return explicit, nil
		default:
			return "", fmt.Errorf("unknown firewall backend '%s' (expected auto|iptables|ufw|advfirewall)", explicit)
		}
	}

	platform := runtime.GOOS
	if v, ok := ctx.Flat["platform"].(string); ok && strings.TrimSpace(v) != "" {
		platform = strings.ToLower(strings.TrimSpace(v))
	}

	if platform == "windows" {
		return "advfirewall", nil
	}

	if _, err := exec.LookPath("ufw"); err == nil {
		return "ufw", nil
	}
	if _, err := exec.LookPath("iptables"); err == nil {
		return "iptables", nil
	}

	if platform == "darwin" {
		return "", fmt.Errorf("firewall backend auto-selection found no supported CLI on macOS (looked for ufw, iptables)")
	}
	return "", fmt.Errorf("firewall backend auto-selection found no supported CLI (looked for ufw, iptables, netsh)")
}

func firewallToIPTables(item map[string]any) map[string]any {
	out := map[string]any{}
	for _, k := range []string{"state", "table", "chain", "jump", "protocol", "source", "destination", "source_port", "destination_port", "port", "in_interface", "out_interface", "comment", "rule_num", "ipv6", "matches", "direction", "action"} {
		if v, ok := item[k]; ok {
			out[k] = v
		}
	}
	if _, ok := out["destination_port"]; !ok {
		if port := strings.TrimSpace(getString(item, "port")); port != "" {
			out["destination_port"] = port
		}
	}
	if _, ok := out["jump"]; !ok {
		out["jump"] = iptablesTarget(item)
	}
	return out
}

func firewallToUFW(item map[string]any) map[string]any {
	out := map[string]any{}
	for _, k := range []string{"state", "rule", "action", "direction", "interface", "source", "destination", "port", "protocol", "comment"} {
		if v, ok := item[k]; ok {
			out[k] = v
		}
	}
	return out
}

func firewallToAdv(item map[string]any) map[string]any {
	out := map[string]any{}
	for _, k := range []string{"state", "name", "rule_name", "direction", "action", "protocol", "local_port", "remote_port", "port", "source", "destination", "program", "profile"} {
		if v, ok := item[k]; ok {
			out[k] = v
		}
	}
	if _, ok := out["name"]; !ok {
		if n := strings.TrimSpace(getString(item, "rule_name")); n != "" {
			out["name"] = n
		}
	}
	if _, ok := out["name"]; !ok {
		if n := strings.TrimSpace(getString(item, "name")); n != "" {
			out["name"] = n
		}
	}
	if _, ok := out["name"]; !ok {
		if n := strings.TrimSpace(getString(item, "id")); n != "" {
			out["name"] = n
		}
	}
	return out
}

func firewallDispatch(item map[string]any, ctx engine.Context) (engine.Handler, map[string]any, string, error) {
	backend, err := firewallBackend(item, ctx)
	if err != nil {
		return nil, nil, "", err
	}
	switch backend {
	case "iptables":
		return iptablesHandler{}, firewallToIPTables(item), backend, nil
	case "ufw":
		return ufwHandler{}, firewallToUFW(item), backend, nil
	case "advfirewall":
		return advFirewallHandler{}, firewallToAdv(item), backend, nil
	default:
		return nil, nil, "", fmt.Errorf("unsupported firewall backend '%s'", backend)
	}
}

func (firewallHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	h, translated, _, err := firewallDispatch(item, ctx)
	if err != nil {
		return false, err
	}
	return h.Test(translated, name, ctx)
}

func (firewallHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	h, translated, backend, err := firewallDispatch(item, ctx)
	if err != nil {
		return "", err
	}
	d, err := h.Describe(translated, action, ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("firewall[%s]: %s", backend, d), nil
}

func (firewallHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	h, translated, _, err := firewallDispatch(item, ctx)
	if err != nil {
		return engine.ExecResult{}, err
	}
	return h.Install(translated, name, ctx)
}

func (firewallHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	h, translated, _, err := firewallDispatch(item, ctx)
	if err != nil {
		return engine.ExecResult{}, err
	}
	return h.Uninstall(translated, name, ctx)
}
