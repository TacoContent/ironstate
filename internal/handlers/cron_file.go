package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

type cronFileHandler struct{}

func cronFilePath(item map[string]any) (string, error) {
	raw := strings.TrimSpace(getString(item, "cron_file"))
	if raw == "" {
		return "", fmt.Errorf("cron_file requires 'cron_file' (absolute path or filename under /etc/cron.d)")
	}
	if filepath.IsAbs(raw) {
		return raw, nil
	}
	return filepath.Join("/etc/cron.d", raw), nil
}

func cronFileRead(path string) ([]string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path comes from playbook intent for cron file management
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}
	return lines, nil
}

func cronFileWrite(path string, lines []string) error {
	if err := ensureParentDir(path); err != nil {
		return err
	}
	content := strings.Join(lines, "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0o600)
}

func parseCronFileMode(v any) (os.FileMode, bool, error) {
	return parseFileModeValue(v, "cron_file")
}

func applyCronFileMetadata(path string, item map[string]any) error {
	if mode, ok, err := parseCronFileMode(item["mode"]); err != nil {
		return err
	} else if ok {
		if err := os.Chmod(path, mode); err != nil {
			return err
		}
	}

	owner := strings.TrimSpace(getString(item, "owner"))
	group := strings.TrimSpace(getString(item, "group"))
	return applyOwnerGroup(path, owner, group)
}

func cronFileEntryLine(item map[string]any) string {
	cmd := strings.TrimSpace(getString(item, "command"))
	if cmd == "" {
		cmd = strings.TrimSpace(getString(item, "job"))
	}
	if cmd == "" {
		return ""
	}
	user := strings.TrimSpace(getStringOr(item, "user", "root"))
	if user == "" {
		user = "root"
	}
	prefix := ""
	if isCronDisabled(item) {
		prefix = "# "
	}
	return prefix + cronSchedule(item) + " " + user + " " + cmd
}

func cronFileEntryLineSequence(item map[string]any) []string {
	seq := []string{}
	if env := getMap(item, "environment"); env != nil {
		for _, key := range sortedKeys(env) {
			if value, ok := env[key]; ok {
				seq = append(seq, fmt.Sprintf("%s=%v", key, value))
			}
		}
	}
	if line := cronFileEntryLine(item); line != "" {
		seq = append(seq, line)
	}
	return seq
}

func isCronFileCommandLine(line string) bool {
	trimmed := stripDisabledPrefix(line)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "@") {
		fields := strings.Fields(trimmed)
		return len(fields) >= 3
	}
	fields := strings.Fields(trimmed)
	return len(fields) >= 7
}

func removeNamedCronFileBlock(lines []string, name string) []string {
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
			if isCronFileCommandLine(next) {
				break
			}
		}
	}
	return out
}

func removeCronFileJobLine(lines []string, line string) []string {
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

func cronFileHasEntry(item map[string]any) (bool, error) {
	path, err := cronFilePath(item)
	if err != nil {
		return false, err
	}
	lines, err := cronFileRead(path)
	if err != nil {
		return false, err
	}
	if cronEnvMode(item) {
		name := cronEnvName(item)
		for _, l := range lines {
			if lineHasEnvName(l, name) {
				return true, nil
			}
		}
		return false, nil
	}
	if name := strings.TrimSpace(getString(item, "name")); name != "" {
		marker := cronNameMarker(name)
		for _, l := range lines {
			if strings.TrimSpace(l) == marker {
				return true, nil
			}
		}
		return false, nil
	}
	line := cronFileEntryLine(item)
	for _, l := range lines {
		if strings.TrimSpace(l) == line || strings.TrimSpace(l) == strings.TrimPrefix(line, "# ") {
			return true, nil
		}
	}
	return false, nil
}

func (cronFileHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	if itemState(item) != "absent" && hasPathMetadataDirective(item) {
		return false, nil
	}
	return cronFileHasEntry(item)
}

func (cronFileHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	path, err := cronFilePath(item)
	if err != nil {
		return "", err
	}
	if action == engine.ActionUninstall {
		return fmt.Sprintf("remove cron_file entry in %q", path), nil
	}
	return fmt.Sprintf("ensure cron_file entry in %q", path), nil
}

func (cronFileHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	path, err := cronFilePath(item)
	if err != nil {
		return engine.ExecResult{}, err
	}
	lines, err := cronFileRead(path)
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
		if err := cronFileWrite(path, filtered); err != nil {
			return engine.ExecResult{}, err
		}
		if err := applyCronFileMetadata(path, item); err != nil {
			return engine.ExecResult{}, err
		}
		return engine.ExecResult{RC: 0}, nil
	}
	line := cronFileEntryLine(item)
	if line == "" {
		return engine.ExecResult{}, fmt.Errorf("cron_file requires a command or job (unless env=true)")
	}
	var filtered []string
	if markerName := strings.TrimSpace(getString(item, "name")); markerName != "" {
		filtered = removeNamedCronFileBlock(lines, markerName)
		filtered = append(filtered, cronNameMarker(markerName))
		for _, entry := range cronFileEntryLineSequence(item) {
			if strings.TrimSpace(entry) != "" {
				filtered = append(filtered, entry)
			}
		}
	} else {
		filtered = removeCronFileJobLine(lines, line)
		for _, entry := range cronFileEntryLineSequence(item) {
			if strings.TrimSpace(entry) != "" {
				filtered = append(filtered, entry)
			}
		}
	}
	if err := cronFileWrite(path, filtered); err != nil {
		return engine.ExecResult{}, err
	}
	if err := applyCronFileMetadata(path, item); err != nil {
		return engine.ExecResult{}, err
	}
	return engine.ExecResult{RC: 0}, nil
}

func (cronFileHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	path, err := cronFilePath(item)
	if err != nil {
		return engine.ExecResult{}, err
	}
	lines, err := cronFileRead(path)
	if err != nil {
		return engine.ExecResult{}, err
	}
	if len(lines) == 0 {
		return engine.ExecResult{RC: 0}, nil
	}
	if cronEnvMode(item) {
		envName := cronEnvName(item)
		if envName == "" {
			return engine.ExecResult{}, fmt.Errorf("cron_file env mode requires 'name'")
		}
		filtered := removeEnvByName(lines, envName)
		if err := cronFileWrite(path, filtered); err != nil {
			return engine.ExecResult{}, err
		}
		return engine.ExecResult{RC: 0}, nil
	}
	if markerName := strings.TrimSpace(getString(item, "name")); markerName != "" {
		filtered := removeNamedCronFileBlock(lines, markerName)
		if err := cronFileWrite(path, filtered); err != nil {
			return engine.ExecResult{}, err
		}
		return engine.ExecResult{RC: 0}, nil
	}
	line := cronFileEntryLine(item)
	if line == "" {
		return engine.ExecResult{RC: 0}, nil
	}
	filtered := removeCronFileJobLine(lines, line)
	if err := cronFileWrite(path, filtered); err != nil {
		return engine.ExecResult{}, err
	}
	return engine.ExecResult{RC: 0}, nil
}
