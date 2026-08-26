package handlers

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

type cronUnixHandler struct{}

func cronNameMarker(name string) string {
	return "#Ansible: " + strings.TrimSpace(name)
}

func cronSchedule(item map[string]any) string {
	if schedule := strings.TrimSpace(getString(item, "schedule")); schedule != "" {
		return schedule
	}

	special := strings.ToLower(strings.TrimSpace(getString(item, "special_time")))
	switch special {
	case "hourly":
		return "0 * * * *"
	case "daily", "midnight":
		return "0 0 * * *"
	case "weekly":
		return "0 0 * * 0"
	case "monthly":
		return "0 0 1 * *"
	case "yearly", "annually":
		return "0 0 1 1 *"
	case "reboot":
		return "@reboot"
	}

	minute := strings.TrimSpace(getStringOr(item, "minute", "*"))
	hour := strings.TrimSpace(getStringOr(item, "hour", "*"))
	day := strings.TrimSpace(getStringOr(item, "day", "*"))
	month := strings.TrimSpace(getStringOr(item, "month", "*"))
	weekday := strings.TrimSpace(getStringOr(item, "weekday", "*"))
	if minute == "" {
		minute = "*"
	}
	if hour == "" {
		hour = "*"
	}
	if day == "" {
		day = "*"
	}
	if month == "" {
		month = "*"
	}
	if weekday == "" {
		weekday = "*"
	}
	return fmt.Sprintf("%s %s %s %s %s", minute, hour, day, month, weekday)
}

func cronEntryLine(item map[string]any) string {
	cmd := strings.TrimSpace(getString(item, "command"))
	if cmd == "" {
		cmd = strings.TrimSpace(getString(item, "job"))
	}
	if cmd == "" {
		return ""
	}
	prefix := ""
	if isCronDisabled(item) {
		prefix = "# "
	}
	return prefix + cronSchedule(item) + " " + cmd
}

func cronEnvMode(item map[string]any) bool {
	if v, ok := item["env"].(bool); ok {
		return v
	}
	if v, ok := item["env"].(string); ok {
		return strings.EqualFold(v, "true") || strings.EqualFold(v, "yes") || strings.EqualFold(v, "on")
	}
	return false
}

func cronEnvName(item map[string]any) string {
	return strings.TrimSpace(getString(item, "name"))
}

func cronEnvLine(item map[string]any) (string, error) {
	name := cronEnvName(item)
	if name == "" {
		return "", fmt.Errorf("cron env mode requires 'name'")
	}
	if strings.Contains(name, "=") || strings.ContainsAny(name, " \t\n\r") {
		return "", fmt.Errorf("cron env name %q is invalid", name)
	}
	return fmt.Sprintf("%s=%s", name, getString(item, "value")), nil
}

func cronEntryLineSequence(item map[string]any) []string {
	seq := []string{}
	if env := getMap(item, "environment"); env != nil {
		for _, key := range sortedKeys(env) {
			if value, ok := env[key]; ok {
				seq = append(seq, fmt.Sprintf("%s=%v", key, value))
			}
		}
	}
	if line := cronEntryLine(item); line != "" {
		seq = append(seq, line)
	}
	return seq
}

func stripDisabledPrefix(line string) string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "# ") {
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
	}
	if strings.HasPrefix(trimmed, "#") {
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
	}
	return trimmed
}

func isManagedMarkerLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	const prefix = "#Ansible:"
	if !strings.HasPrefix(trimmed, prefix) {
		return "", false
	}
	name := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
	if name == "" {
		return "", false
	}
	return name, true
}

func isCronCommandLine(line string) bool {
	trimmed := stripDisabledPrefix(line)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "@") {
		fields := strings.Fields(trimmed)
		return len(fields) >= 2
	}
	fields := strings.Fields(trimmed)
	return len(fields) >= 6
}

func envVarNameFromLine(line string) string {
	trimmed := stripDisabledPrefix(line)
	if strings.HasPrefix(trimmed, "#") {
		return ""
	}
	if strings.Contains(trimmed, " ") || !strings.Contains(trimmed, "=") {
		return ""
	}
	parts := strings.SplitN(trimmed, "=", 2)
	name := strings.TrimSpace(parts[0])
	if name == "" || strings.ContainsAny(name, " \t\n\r") {
		return ""
	}
	return name
}

func lineHasEnvName(line, name string) bool {
	return strings.EqualFold(envVarNameFromLine(line), strings.TrimSpace(name))
}

func removeJobLine(lines []string, line string) []string {
	out := make([]string, 0, len(lines))
	target := strings.TrimSpace(line)
	legacy := strings.TrimSpace(strings.TrimPrefix(line, "# "))
	for _, existing := range lines {
		trimmed := strings.TrimSpace(existing)
		if trimmed == target || trimmed == legacy {
			continue
		}
		out = append(out, existing)
	}
	return out
}

func removeNamedCronBlock(lines []string, name string) []string {
	if strings.TrimSpace(name) == "" {
		return lines
	}
	marker := cronNameMarker(name)
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != marker {
			out = append(out, lines[i])
			continue
		}
		for i+1 < len(lines) {
			next := strings.TrimSpace(lines[i+1])
			if _, ok := isManagedMarkerLine(next); ok {
				break
			}
			if next == "" {
				i++
				continue
			}
			i++
			if isCronCommandLine(next) {
				break
			}
		}
	}
	return out
}

func removeEnvByName(lines []string, name string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if lineHasEnvName(line, name) {
			continue
		}
		out = append(out, line)
	}
	return out
}

func insertEnvLine(lines []string, envLine string, beforeName string, afterName string) []string {
	beforeName = strings.TrimSpace(beforeName)
	afterName = strings.TrimSpace(afterName)
	if beforeName != "" {
		for i, line := range lines {
			if lineHasEnvName(line, beforeName) {
				out := append([]string{}, lines[:i]...)
				out = append(out, envLine)
				out = append(out, lines[i:]...)
				return out
			}
		}
	}
	if afterName != "" {
		for i, line := range lines {
			if lineHasEnvName(line, afterName) {
				out := append([]string{}, lines[:i+1]...)
				out = append(out, envLine)
				out = append(out, lines[i+1:]...)
				return out
			}
		}
	}
	return append(lines, envLine)
}

func isCronDisabled(item map[string]any) bool {
	if v, ok := item["disabled"].(bool); ok {
		return v
	}
	if v, ok := item["disabled"].(string); ok {
		return strings.EqualFold(v, "true") || strings.EqualFold(v, "yes") || strings.EqualFold(v, "on")
	}
	return false
}

func cronReadArgs(user string) []string {
	if strings.TrimSpace(user) == "" {
		return []string{"-l"}
	}
	return []string{"-u", user, "-l"}
}

func cronWriteArgs(user string) []string {
	if strings.TrimSpace(user) == "" {
		return []string{"-"}
	}
	return []string{"-u", user, "-"}
}

func cronRead(user string) ([]string, error) {
	cmd := exec.Command("crontab", cronReadArgs(user)...) //nolint:gosec // fixed executable; args intentionally derived from declarative module input
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}
	return lines, nil
}

func cronWrite(user string, lines []string) error {
	content := strings.Join(lines, "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
		content += "\n"
	}
	cmd := exec.Command("crontab", cronWriteArgs(user)...) //nolint:gosec // fixed executable; args intentionally derived from declarative module input
	cmd.Stdin = strings.NewReader(content)
	return cmd.Run()
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func cronHasEntry(item map[string]any) bool {
	user := strings.TrimSpace(getString(item, "user"))
	lines, err := cronRead(user)
	if err != nil {
		return false
	}
	if cronEnvMode(item) {
		name := cronEnvName(item)
		for _, l := range lines {
			if lineHasEnvName(l, name) {
				return true
			}
		}
		return false
	}
	if name := strings.TrimSpace(getString(item, "name")); name != "" {
		marker := cronNameMarker(name)
		for _, l := range lines {
			if strings.TrimSpace(l) == marker {
				return true
			}
		}
		return false
	}
	line := cronEntryLine(item)
	for _, l := range lines {
		if strings.TrimSpace(l) == line || strings.TrimSpace(l) == strings.TrimPrefix(line, "# ") {
			return true
		}
	}
	return false
}

func (cronUnixHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	if itemState(item) == "absent" {
		return cronHasEntry(item), nil
	}
	return cronHasEntry(item), nil
}

func (cronUnixHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	if action == engine.ActionUninstall {
		return fmt.Sprintf("remove cron job %q", cronEntryLine(item)), nil
	}
	return fmt.Sprintf("ensure cron job %q", cronEntryLine(item)), nil
}

func (cronUnixHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	user := strings.TrimSpace(getString(item, "user"))
	lines, err := cronRead(user)
	if err != nil {
		return engine.ExecResult{}, err
	}
	if cronEnvMode(item) {
		envLine, err := cronEnvLine(item)
		if err != nil {
			return engine.ExecResult{}, err
		}
		filtered := removeEnvByName(lines, cronEnvName(item))
		filtered = insertEnvLine(filtered, envLine, getString(item, "insertbefore"), getString(item, "insertafter"))
		if err := cronWrite(user, filtered); err != nil {
			return engine.ExecResult{}, err
		}
		return engine.ExecResult{RC: 0}, nil
	}
	line := cronEntryLine(item)
	if line == "" {
		return engine.ExecResult{}, fmt.Errorf("cron requires a command or job")
	}
	var filtered []string
	if markerName := strings.TrimSpace(getString(item, "name")); markerName != "" {
		filtered = removeNamedCronBlock(lines, markerName)
		filtered = append(filtered, cronNameMarker(markerName))
		for _, entry := range cronEntryLineSequence(item) {
			if strings.TrimSpace(entry) != "" {
				filtered = append(filtered, entry)
			}
		}
	} else {
		filtered = removeJobLine(lines, line)
		for _, entry := range cronEntryLineSequence(item) {
			if strings.TrimSpace(entry) != "" {
				filtered = append(filtered, entry)
			}
		}
	}
	if err := cronWrite(user, filtered); err != nil {
		return engine.ExecResult{}, err
	}
	return engine.ExecResult{RC: 0}, nil
}

func (cronUnixHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	user := strings.TrimSpace(getString(item, "user"))
	lines, err := cronRead(user)
	if err != nil {
		return engine.ExecResult{}, err
	}
	if cronEnvMode(item) {
		envName := cronEnvName(item)
		if envName == "" {
			return engine.ExecResult{}, fmt.Errorf("cron env mode requires 'name'")
		}
		filtered := removeEnvByName(lines, envName)
		if err := cronWrite(user, filtered); err != nil {
			return engine.ExecResult{}, err
		}
		return engine.ExecResult{RC: 0}, nil
	}
	if markerName := strings.TrimSpace(getString(item, "name")); markerName != "" {
		filtered := removeNamedCronBlock(lines, markerName)
		if err := cronWrite(user, filtered); err != nil {
			return engine.ExecResult{}, err
		}
		return engine.ExecResult{RC: 0}, nil
	}
	line := cronEntryLine(item)
	if line == "" {
		return engine.ExecResult{RC: 0}, nil
	}
	filtered := removeJobLine(lines, line)
	if err := cronWrite(user, filtered); err != nil {
		return engine.ExecResult{}, err
	}
	return engine.ExecResult{RC: 0}, nil
}
