package handlers

import (
	"os"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// shellHandler ports Handlers/Shell.psm1: runs an inline command or a
// script file through a configurable interpreter ('host'). 'creates'
// gates idempotency (shared primitive with 'zip', see creates.go).
//
// Deviation from Handlers/Shell.psm1 (documented, not silent): the
// original's default 'pwsh' host runs the script *in-process* (no
// subprocess), which is what lets it capture a real .NET pipeline object
// and merge its properties into the registered result (e.g.
// '${{ pf.ProgramFilesDir }}', see Merge-ShellNativeResult). Go has no
// equivalent to "run PowerShell script content in-process" - every host,
// including 'pwsh', is always a subprocess here. Per
// docs/plans/go-rewrite.md §4.8/§8/§11, that native-object merge is
// already audited as unused anywhere in this repo's real YAML today, so
// this gap is accepted rather than worked around.
type shellHandler struct{}

var shellHostPresets = map[string][]string{
	"powershell": {"powershell.exe"},
	"cmd":        {"cmd.exe", "/d", "/c"},
	"bash":       {"bash.exe"},
	"sh":         {"sh.exe"},
	"node":       {"node.exe"},
	"python":     {"python.exe"},
}

var shellHostExtensions = map[string]string{
	"powershell": ".ps1",
	"cmd":        ".cmd",
	"bash":       ".sh",
	"sh":         ".sh",
	"node":       ".js",
	"python":     ".py",
}

// shellStateConfig is the effective command/script/args/host/extension for
// a given state - ports Resolve-ShellStateConfig's per-field fallback
// rules ('absent' has no fallback to the top-level, present-oriented
// block, to avoid ever accidentally re-running an install command on
// uninstall).
type shellStateConfig struct {
	Command   string
	Script    string
	ItemArgs  []string
	HostSpec  string
	Extension string
}

func resolveShellStateConfig(item map[string]any, state string) shellStateConfig {
	block := getMap(item, state)
	var fallback map[string]any
	if state != "absent" {
		fallback = item
	}

	pick := func(key string) any {
		if block != nil {
			if v, ok := block[key]; ok {
				return v
			}
		}
		if fallback != nil {
			return fallback[key]
		}
		return nil
	}

	command, _ := pick("command").(string)
	script, _ := pick("script").(string)
	hostSpec, _ := pick("host").(string)
	if hostSpec == "" {
		hostSpec = "pwsh"
	}
	extension, _ := pick("extension").(string)

	var itemArgs []string
	for _, raw := range asList(pick("args")) {
		if s, ok := raw.(string); ok {
			itemArgs = append(itemArgs, s)
		}
	}

	return shellStateConfig{
		Command:   command,
		Script:    script,
		ItemArgs:  itemArgs,
		HostSpec:  hostSpec,
		Extension: extension,
	}
}

// shellItemLabel ports Get-ShellItemLabel: itemLabel falls back to a
// bare top-level 'command'/'script', which a shell item defined entirely
// through per-state blocks doesn't have - check those blocks too.
func shellItemLabel(item map[string]any, state string) string {
	if label := itemLabel(item); label != "<unknown>" {
		return label
	}
	seen := map[string]bool{}
	order := []string{state, "present", "absent", "latest"}
	for _, s := range order {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		if block := getMap(item, s); block != nil {
			if label := itemLabel(block); label != "<unknown>" {
				return label
			}
		}
	}
	return "<unknown>"
}

func shellHostInvocation(hostSpec string) []string {
	if hostSpec == "pwsh" {
		return nil
	}
	if preset, ok := shellHostPresets[hostSpec]; ok {
		return preset
	}
	return strings.Fields(hostSpec)
}

func invokeShellItem(cfg shellStateConfig, label string) engine.ExecResult {
	if cfg.Script == "" && cfg.Command == "" {
		engine.Warn("Shell item '%s' has neither 'command' nor 'script'", label)
		return engine.ExecResult{}
	}

	invocation := shellHostInvocation(cfg.HostSpec)

	runPath := cfg.Script
	var tempFile string
	if runPath == "" {
		extension := ".txt"
		switch {
		case cfg.HostSpec == "pwsh":
			extension = ".ps1"
		case cfg.Extension != "":
			extension = cfg.Extension
		default:
			if e, ok := shellHostExtensions[cfg.HostSpec]; ok {
				extension = e
			}
		}
		f, err := os.CreateTemp("", "ironstate-*"+extension)
		if err != nil {
			engine.Warn("Shell item '%s': %v", label, err)
			return engine.ExecResult{}
		}
		if _, err := f.WriteString(cfg.Command); err != nil {
			_ = f.Close()
			engine.Warn("Shell item '%s': %v", label, err)
			return engine.ExecResult{}
		}
		if err := f.Close(); err != nil {
			engine.Warn("Shell item '%s': %v", label, err)
			return engine.ExecResult{}
		}
		tempFile = f.Name()
		runPath = tempFile
		defer func() { _ = os.Remove(tempFile) }()
	} else if !fileExists(runPath) {
		engine.Warn("Shell script not found: %s", runPath)
		return engine.ExecResult{}
	}

	var result engine.ExecResult
	if len(invocation) == 0 {
		// 'pwsh' host: always a subprocess here (see the deviation note
		// on shellHandler), unlike the original's in-process execution.
		args := append([]string{"-NoLogo", "-NoProfile", "-File", runPath}, cfg.ItemArgs...)
		result = runExternalCommand("pwsh", args)
	} else {
		exe := invocation[0]
		args := append(append([]string{}, invocation[1:]...), append([]string{runPath}, cfg.ItemArgs...)...)
		result = runExternalCommand(exe, args)
	}

	if result.RC != 0 {
		engine.Warn("Shell item '%s' exited with code %d", label, result.RC)
	}
	return result
}

func (shellHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	return testCreatesPresent(asList(item["creates"])), nil
}

func (shellHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	if action == engine.ActionUninstall {
		label := shellItemLabel(item, "absent")
		cfg := resolveShellStateConfig(item, "absent")
		if cfg.Command != "" || cfg.Script != "" {
			return "run shell '" + label + "' via '" + cfg.HostSpec + "' (uninstall)", nil
		}
		return "remove creates entries for shell '" + label + "'", nil
	}
	state := itemState(item)
	label := shellItemLabel(item, state)
	cfg := resolveShellStateConfig(item, state)
	return "run shell '" + label + "' via '" + cfg.HostSpec + "'", nil
}

func (shellHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	state := itemState(item)
	cfg := resolveShellStateConfig(item, state)
	return invokeShellItem(cfg, shellItemLabel(item, state)), nil
}

func (shellHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	cfg := resolveShellStateConfig(item, "absent")
	var result engine.ExecResult
	if cfg.Command != "" || cfg.Script != "" {
		result = invokeShellItem(cfg, shellItemLabel(item, "absent"))
	}
	removeCreatesPatterns(asList(item["creates"]))
	return result, nil
}
