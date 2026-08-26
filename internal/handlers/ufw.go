package handlers

import (
	"fmt"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// ufwHandler manages uncomplicated firewall rules through the ufw CLI.
// UFW does not expose a stable machine-readable exact-match query for all
// rule variants, so Test uses conservative behavior:
// - present/latest: always run (ufw itself deduplicates common repeats)
// - absent: always run delete (non-existent delete is normalized to RC 0)
type ufwHandler struct{}

func ufwRuleAction(item map[string]any) string {
	rule := strings.ToLower(strings.TrimSpace(getString(item, "rule")))
	if rule != "" {
		return rule
	}
	switch strings.ToLower(strings.TrimSpace(getString(item, "action"))) {
	case "deny", "drop", "block":
		return "deny"
	case "reject":
		return "reject"
	case "limit":
		return "limit"
	default:
		return "allow"
	}
}

func ufwRuleArgs(item map[string]any) []string {
	args := []string{"--force", ufwRuleAction(item)}

	if direction := strings.ToLower(strings.TrimSpace(getString(item, "direction"))); direction == "in" || direction == "out" {
		args = append(args, direction)
	}

	if iface := strings.TrimSpace(getString(item, "interface")); iface != "" {
		args = append(args, "on", iface)
	}

	source := strings.TrimSpace(getString(item, "source"))
	if source == "" {
		source = "any"
	}
	args = append(args, "from", source)

	destination := strings.TrimSpace(getString(item, "destination"))
	if destination == "" {
		destination = "any"
	}
	args = append(args, "to", destination)

	port := strings.TrimSpace(getString(item, "port"))
	if port != "" {
		args = append(args, "port", port)
	}

	protocol := strings.TrimSpace(getString(item, "protocol"))
	if protocol != "" && !strings.EqualFold(protocol, "any") && !strings.EqualFold(protocol, "all") {
		args = append(args, "proto", protocol)
	}

	if comment := strings.TrimSpace(getString(item, "comment")); comment != "" {
		args = append(args, "comment", comment)
	}

	return args
}

func ufwExec(args []string) engine.ExecResult {
	r, err := runner.Run("ufw", args)
	if err != nil {
		engine.Warn("ufw: %v", err)
		return engine.ExecResult{RC: 1, Stderr: err.Error(), StderrLines: []string{err.Error()}}
	}
	for _, line := range r.StdoutLines {
		engine.Info("%s", line)
	}
	for _, line := range r.StderrLines {
		engine.Warn("%s", line)
	}
	return engine.ExecResult{RC: r.RC, Stdout: r.Stdout, StdoutLines: r.StdoutLines, Stderr: r.Stderr, StderrLines: r.StderrLines}
}

func (ufwHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	if itemState(item) == "absent" {
		return true, nil
	}
	return false, nil
}

func (ufwHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	rule := ufwRuleAction(item)
	if action == engine.ActionUninstall {
		return fmt.Sprintf("delete ufw rule (%s)", rule), nil
	}
	return fmt.Sprintf("ensure ufw rule (%s)", rule), nil
}

func (ufwHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	res := ufwExec(ufwRuleArgs(item))
	if res.RC != 0 {
		engine.Warn("ufw apply rule exited with code %d", res.RC)
	}
	return res, nil
}

func (ufwHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	args := append([]string{"--force", "delete"}, ufwRuleArgs(item)[1:]...)
	res := ufwExec(args)
	if res.RC != 0 {
		lower := strings.ToLower(res.Stderr)
		if strings.Contains(lower, "could not delete non-existent") || strings.Contains(lower, "could not find") {
			return engine.ExecResult{RC: 0, Stdout: res.Stdout, StdoutLines: res.StdoutLines, Stderr: res.Stderr, StderrLines: res.StderrLines}, nil
		}
		engine.Warn("ufw delete rule exited with code %d", res.RC)
	}
	return res, nil
}
