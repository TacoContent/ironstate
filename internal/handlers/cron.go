package handlers

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// cronHandler is a cross-platform wrapper that maps a unified cron task to the
// platform-appropriate backend: a Unix crontab job on Linux/macOS, or the
// existing scheduled_task handler on Windows.
type cronHandler struct{}

func cronBackend(item map[string]any, ctx engine.Context) (string, error) {
	if explicit := strings.ToLower(strings.TrimSpace(getString(item, "backend"))); explicit != "" && explicit != "auto" {
		switch explicit {
		case "cron":
			if runtime.GOOS == "windows" {
				return "", fmt.Errorf("cron backend is only supported on Linux/macOS")
			}
			return "cron", nil
		case "scheduled_task":
			if runtime.GOOS != "windows" {
				return "", fmt.Errorf("scheduled_task backend is only supported on Windows")
			}
			return "scheduled_task", nil
		default:
			return "", fmt.Errorf("unknown cron backend %q (expected auto|cron|scheduled_task)", explicit)
		}
	}

	platform := runtime.GOOS
	if v, ok := ctx.Flat["platform"].(string); ok {
		platform = strings.ToLower(strings.TrimSpace(v))
	}

	switch platform {
	case "windows":
		return "scheduled_task", nil
	case "linux", "darwin":
		return "cron", nil
	default:
		return "", fmt.Errorf("cron backend auto-selection found no supported platform (windows/linux/darwin)")
	}
}

func cronToUnix(item map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"state", "name", "command", "job", "user", "schedule", "special_time", "minute", "hour", "day", "month", "weekday", "disabled", "environment", "env", "value", "insertbefore", "insertafter"} {
		if v, ok := item[key]; ok {
			out[key] = v
		}
	}
	if _, ok := out["command"]; !ok {
		if cmd := strings.TrimSpace(getString(item, "job")); cmd != "" {
			out["command"] = cmd
		}
	}
	if _, ok := out["schedule"]; !ok {
		if schedule := cronSchedule(item); schedule != "" {
			out["schedule"] = schedule
		}
	}
	return out
}

func cronToScheduledTask(item map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"state", "name", "command", "job", "description", "enabled", "actions", "triggers", "principal", "settings", "owner", "group"} {
		if v, ok := item[key]; ok {
			out[key] = v
		}
	}
	if _, ok := out["name"]; !ok {
		if name := strings.TrimSpace(getString(item, "name")); name != "" {
			out["name"] = name
		}
	}
	if _, ok := out["actions"]; !ok {
		cmd := strings.TrimSpace(getString(item, "command"))
		if cmd == "" {
			cmd = strings.TrimSpace(getString(item, "job"))
		}
		if cmd != "" {
			out["actions"] = []any{map[string]any{"execute": cmd}}
		}
	}
	return out
}

func cronDispatch(item map[string]any, ctx engine.Context) (engine.Handler, map[string]any, string, error) {
	backend, err := cronBackend(item, ctx)
	if err != nil {
		return nil, nil, "", err
	}
	switch backend {
	case "cron":
		return cronUnixHandler{}, cronToUnix(item), backend, nil
	case "scheduled_task":
		return scheduledTaskHandler{}, cronToScheduledTask(item), backend, nil
	default:
		return nil, nil, "", fmt.Errorf("unsupported cron backend %q", backend)
	}
}

func (cronHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	h, translated, _, err := cronDispatch(item, ctx)
	if err != nil {
		return false, err
	}
	return h.Test(translated, name, ctx)
}

func (cronHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	h, translated, backend, err := cronDispatch(item, ctx)
	if err != nil {
		return "", err
	}
	d, err := h.Describe(translated, action, ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("cron[%s]: %s", backend, d), nil
}

func (cronHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	h, translated, _, err := cronDispatch(item, ctx)
	if err != nil {
		return engine.ExecResult{}, err
	}
	return h.Install(translated, name, ctx)
}

func (cronHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	h, translated, _, err := cronDispatch(item, ctx)
	if err != nil {
		return engine.ExecResult{}, err
	}
	return h.Uninstall(translated, name, ctx)
}
