package handlers

import (
	"fmt"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// iptablesHandler manages Linux-style iptables/ip6tables rules. It is
// intentionally close to Ansible's iptables module for common rule
// operations, while fitting ironstate's present/absent/latest state model.
type iptablesHandler struct{}

func iptablesBinary(item map[string]any) string {
	if getBool(item, "ipv6", false) {
		return "ip6tables"
	}
	return "iptables"
}

func iptablesChain(item map[string]any) string {
	if v := strings.TrimSpace(getString(item, "chain")); v != "" {
		return v
	}
	if strings.EqualFold(strings.TrimSpace(getString(item, "direction")), "out") {
		return "OUTPUT"
	}
	return "INPUT"
}

func iptablesTarget(item map[string]any) string {
	if jump := strings.TrimSpace(getString(item, "jump")); jump != "" {
		return jump
	}
	switch strings.ToLower(strings.TrimSpace(getString(item, "action"))) {
	case "deny", "drop":
		return "DROP"
	case "reject":
		return "REJECT"
	default:
		return "ACCEPT"
	}
}

func iptablesRuleArgs(item map[string]any) []string {
	var args []string

	table := strings.TrimSpace(getStringOr(item, "table", "filter"))
	if table != "" {
		args = append(args, "-t", table)
	}

	protocol := strings.TrimSpace(getString(item, "protocol"))
	if protocol != "" && !strings.EqualFold(protocol, "all") {
		args = append(args, "-p", protocol)
	}
	if source := strings.TrimSpace(getString(item, "source")); source != "" {
		args = append(args, "-s", source)
	}
	if destination := strings.TrimSpace(getString(item, "destination")); destination != "" {
		args = append(args, "-d", destination)
	}
	if inIf := strings.TrimSpace(getString(item, "in_interface")); inIf != "" {
		args = append(args, "-i", inIf)
	}
	if outIf := strings.TrimSpace(getString(item, "out_interface")); outIf != "" {
		args = append(args, "-o", outIf)
	}

	sourcePort := strings.TrimSpace(getString(item, "source_port"))
	destinationPort := strings.TrimSpace(getString(item, "destination_port"))
	if destinationPort == "" {
		destinationPort = strings.TrimSpace(getString(item, "port"))
	}
	if sourcePort != "" {
		args = append(args, "--sport", sourcePort)
	}
	if destinationPort != "" {
		args = append(args, "--dport", destinationPort)
	}

	for _, raw := range asList(item["matches"]) {
		if m, ok := raw.(string); ok && strings.TrimSpace(m) != "" {
			args = append(args, "-m", strings.TrimSpace(m))
		}
	}

	if comment := strings.TrimSpace(getString(item, "comment")); comment != "" {
		args = append(args, "-m", "comment", "--comment", comment)
	}

	args = append(args, "-j", iptablesTarget(item))
	return args
}

func iptablesCheck(item map[string]any) (bool, error) {
	chain := iptablesChain(item)
	args := append([]string{"-C", chain}, iptablesRuleArgs(item)...)
	res, err := runner.Run(iptablesBinary(item), args)
	if err != nil {
		return false, nil //nolint:nilerr // treat probe failures as not-converged state
	}
	return res.RC == 0, nil
}

func (iptablesHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	rulePresent, err := iptablesCheck(item)
	if err != nil {
		return false, err
	}
	if itemState(item) == "absent" {
		return rulePresent, nil
	}
	return rulePresent, nil
}

func (iptablesHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	chain := iptablesChain(item)
	target := iptablesTarget(item)
	if action == engine.ActionUninstall {
		return fmt.Sprintf("remove %s rule from %s (%s)", iptablesBinary(item), chain, target), nil
	}
	return fmt.Sprintf("ensure %s rule in %s (%s)", iptablesBinary(item), chain, target), nil
}

func (iptablesHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	chain := iptablesChain(item)
	op := "-A"
	if rawNum := strings.TrimSpace(getString(item, "rule_num")); rawNum != "" {
		op = "-I"
	}
	args := []string{op, chain}
	if op == "-I" {
		args = append(args, strings.TrimSpace(getString(item, "rule_num")))
	}
	args = append(args, iptablesRuleArgs(item)...)
	res := runExternalCommand(iptablesBinary(item), args)
	if res.RC != 0 {
		engine.Warn("%s add/insert rule exited with code %d", iptablesBinary(item), res.RC)
	}
	return res, nil
}

func (iptablesHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	chain := iptablesChain(item)
	args := append([]string{"-D", chain}, iptablesRuleArgs(item)...)
	res := runExternalCommand(iptablesBinary(item), args)
	if res.RC != 0 {
		engine.Warn("%s delete rule exited with code %d", iptablesBinary(item), res.RC)
	}
	return res, nil
}
