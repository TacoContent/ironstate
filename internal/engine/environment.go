package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/TacoContent/ironstate/internal/expr"
	"github.com/TacoContent/ironstate/internal/model"
	"github.com/TacoContent/ironstate/internal/pathutil"
	"github.com/TacoContent/ironstate/internal/template"
)

// ResolveEnvironment evaluates one declaration layer against the incoming
// environment. cache, when provided, holds valueFrom results for one scope.
func ResolveEnvironment(values map[string]any, ctx Context, shell Handler, cache map[string]any) (map[string]any, error) {
	resolved := make(map[string]any, len(values))
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if key == "" || strings.ContainsAny(key, "=\x00") {
			return nil, fmt.Errorf("envs: invalid environment variable name %q", key)
		}
		raw := model.DeepCopy(values[key])
		value, err := resolveEnvironmentValue(raw, ctx, shell, cache, key)
		if err != nil {
			return nil, fmt.Errorf("envs.%s: %w", key, err)
		}
		if err := environmentPrimitive(value); err != nil {
			return nil, fmt.Errorf("envs.%s: %w", key, err)
		}
		if strings.ContainsRune(expr.DisplayString(value), '\x00') {
			return nil, fmt.Errorf("envs.%s: value contains a NUL byte", key)
		}
		resolved[key] = value
	}
	return resolved, nil
}

func environmentPrimitive(value any) error {
	switch value.(type) {
	case nil, string, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return nil
	default:
		return fmt.Errorf("expected a primitive value, got %T", value)
	}
}

func expandEnvironmentValue(raw any, ctx Context) (any, error) {
	value, err := template.ExpandNode(raw, ctx.Flat, ctx.Filters, "environment", false, nil)
	if err != nil {
		return nil, err
	}
	if template.IsOmit(value) {
		return nil, fmt.Errorf("environment expression could not be resolved")
	}
	return value, nil
}

func resolveEnvironmentValue(raw any, ctx Context, shell Handler, cache map[string]any, key string) (any, error) {
	spec, advanced := raw.(map[string]any)
	if !advanced {
		return expandEnvironmentValue(raw, ctx)
	}
	source, ok := spec["valueFrom"].(map[string]any)
	if !ok || len(spec) != 1 || len(source) != 1 {
		return nil, fmt.Errorf("expected valueFrom with exactly one source: file or task")
	}
	if _, file := source["file"]; !file {
		if _, task := source["task"]; !task {
			return nil, fmt.Errorf("unknown valueFrom source (supported: file, task)")
		}
	}
	if value, cached := cache[key]; cached {
		return value, nil
	}
	var value any
	if rawPath, file := source["file"]; file {
		path, err := expandEnvironmentValue(rawPath, ctx)
		if err != nil {
			return nil, err
		}
		text, ok := path.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("valueFrom.file must resolve to a non-empty path string")
		}
		data, err := os.ReadFile(environmentPath(text, ctx.BaseDir)) //nolint:gosec // explicit playbook-configured environment source
		if err != nil {
			return nil, fmt.Errorf("valueFrom.file: %w", err)
		}
		value = string(data)
	} else {
		task, ok := source["task"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("valueFrom.task must be a mapping")
		}
		var err error
		value, err = resolveEnvironmentTask(task, ctx, shell)
		if err != nil {
			return nil, err
		}
	}
	if err := environmentPrimitive(value); err != nil {
		return nil, err
	}
	if cache != nil {
		cache[key] = value
	}
	return value, nil
}

func resolveEnvironmentTask(task map[string]any, ctx Context, shell Handler) (any, error) {
	value, present := task["value"]
	if !present {
		return nil, fmt.Errorf("valueFrom.task requires value")
	}
	for key := range task {
		if key != "value" && key != "shell" {
			return nil, fmt.Errorf("unknown valueFrom.task field %q", key)
		}
	}
	valueContext := ctx
	valueContext.Flat = cloneFlat(ctx.Flat)
	if rawShell, present := task["shell"]; present {
		if _, ok := rawShell.(map[string]any); !ok {
			return nil, fmt.Errorf("valueFrom.task.shell must be a mapping")
		}
		expanded, err := expandEnvironmentValue(rawShell, ctx)
		if err != nil {
			return nil, err
		}
		shellSpec := model.AsMap(expanded)
		for key, value := range shellSpec {
			switch key {
			case "command", "script", "type", "host", "extension":
				if _, ok := value.(string); !ok {
					return nil, fmt.Errorf("valueFrom.task.shell.%s must resolve to a string", key)
				}
			case "args":
				args, ok := value.([]any)
				if !ok {
					return nil, fmt.Errorf("valueFrom.task.shell.args must resolve to a list of strings")
				}
				for _, arg := range args {
					if _, ok := arg.(string); !ok {
						return nil, fmt.Errorf("valueFrom.task.shell.args must resolve to a list of strings")
					}
				}
			default:
				return nil, fmt.Errorf("unsupported valueFrom.task.shell field %q", key)
			}
		}
		command, _ := shellSpec["command"].(string)
		script, _ := shellSpec["script"].(string)
		if script != "" {
			shellSpec["script"] = environmentPath(script, ctx.BaseDir)
		}
		if strings.TrimSpace(command) == "" && strings.TrimSpace(script) == "" {
			return nil, fmt.Errorf("valueFrom.task.shell requires command or script")
		}
		if command != "" && script != "" {
			return nil, fmt.Errorf("valueFrom.task.shell requires only one of command or script")
		}
		if shell == nil {
			return nil, fmt.Errorf("valueFrom.task.shell: shell handler is not registered")
		}
		shellContext := ctx
		shellContext.Apply = true
		result, err := shell.Install(shellSpec, "environment source", shellContext)
		if err != nil {
			return nil, fmt.Errorf("valueFrom.task.shell: %w", err)
		}
		if result.RC != 0 {
			return nil, fmt.Errorf("valueFrom.task.shell exited with code %d: %s", result.RC, strings.TrimSpace(result.Stderr))
		}
		mergeExecInto(valueContext.Flat, result)
	}
	return expandEnvironmentValue(value, valueContext)
}

func environmentPath(path, baseDir string) string {
	path = pathutil.ResolveUserPath(path)
	if baseDir != "" && !filepath.IsAbs(path) {
		return filepath.Join(baseDir, path)
	}
	return path
}
