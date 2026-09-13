package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/TacoContent/ironstate/sdk/becomeexec"
	"github.com/TacoContent/ironstate/sdk/handler"
	"github.com/TacoContent/ironstate/sdk/plugin"
)

const writeFileCommand = "--ironstate-hosts-write"

type hostsHandler struct{}

type hostsSpec struct {
	Path     string
	IP       string
	Hostname string
	Entry    string
}

func (hostsHandler) Test(item map[string]any, _ string, ctx handler.Context) (bool, error) {
	if expression, ok := item["callback_condition"].(string); ok && expression != "" {
		if ctx.Callbacks == nil {
			return false, fmt.Errorf("host callbacks are required for callback_condition")
		}
		matches, err := ctx.Callbacks.EvaluateCondition(expression, ctx.Flat)
		if err != nil {
			return false, err
		}
		if !matches {
			return false, nil
		}
	}
	spec, err := parseSpec(item)
	if err != nil {
		return false, err
	}
	lines, err := readLines(spec.Path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return hasMapping(lines, spec.IP, spec.Hostname), nil
}

func (hostsHandler) Describe(item map[string]any, action handler.Action, ctx handler.Context) (string, error) {
	if template, ok := item["callback_template"].(string); ok && template != "" {
		if ctx.Callbacks == nil {
			return "", fmt.Errorf("host callbacks are required for callback_template")
		}
		return ctx.Callbacks.RenderTemplate(template, ctx.Flat)
	}
	spec, err := parseSpec(item)
	if err != nil {
		return "", err
	}
	verb := "add"
	if action == handler.ActionUninstall {
		verb = "remove"
	}
	return fmt.Sprintf("would %s hosts entry %s", verb, spec.Entry), nil
}

func (hostsHandler) Install(item map[string]any, _ string, ctx handler.Context) (handler.ExecResult, error) {
	spec, err := parseSpec(item)
	if err != nil {
		return failedResult(err), err
	}
	lines, err := readLines(spec.Path)
	if err != nil && !os.IsNotExist(err) {
		return failedResult(err), err
	}
	if hasMapping(lines, spec.IP, spec.Hostname) {
		return handler.ExecResult{RC: 0}, nil
	}
	lines = removeHostname(lines, spec.Hostname)
	lines = append(lines, spec.Entry)
	if err := writeLines(spec.Path, lines, ctx.Become); err != nil {
		return failedResult(err), err
	}
	return resultFor(spec), nil
}

func (hostsHandler) Uninstall(item map[string]any, _ string, ctx handler.Context) (handler.ExecResult, error) {
	spec, err := parseSpec(item)
	if err != nil {
		return failedResult(err), err
	}
	lines, err := readLines(spec.Path)
	if os.IsNotExist(err) {
		return handler.ExecResult{RC: 0}, nil
	}
	if err != nil {
		return failedResult(err), err
	}
	filtered := removeMapping(lines, spec.IP, spec.Hostname)
	if len(filtered) == len(lines) {
		return handler.ExecResult{RC: 0}, nil
	}
	if err := writeLines(spec.Path, filtered, ctx.Become); err != nil {
		return failedResult(err), err
	}
	return resultFor(spec), nil
}

func (hostsHandler) FactName(item map[string]any) (string, bool) {
	name, _ := item["name"].(string)
	return strings.TrimPrefix(name, "$"), name != ""
}

func (hostsHandler) ScanRole() string { return "roles/system/hosts" }

func (hostsHandler) Scan(ctx handler.Context) ([]handler.ScanItem, error) {
	path := defaultHostsPath()
	if value, ok := ctx.Flat["hosts_path"].(string); ok && value != "" {
		path = value
	}
	lines, err := readLines(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	items := make([]handler.ScanItem, 0)
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		for _, hostname := range fields[1:] {
			items = append(items, handler.ScanItem{
				Module: "ensure_entry",
				Name:   hostname,
				Config: map[string]any{"path": path, "ip": fields[0], "hostname": hostname},
			})
		}
	}
	return items, nil
}

func parseSpec(item map[string]any) (hostsSpec, error) {
	path, _ := item["path"].(string)
	if path == "" {
		path = defaultHostsPath()
	}
	ip, _ := item["ip"].(string)
	hostname, _ := item["hostname"].(string)
	if strings.TrimSpace(ip) == "" || strings.TrimSpace(hostname) == "" {
		return hostsSpec{}, fmt.Errorf("hosts handler requires non-empty 'ip' and 'hostname'")
	}
	return hostsSpec{Path: path, IP: strings.TrimSpace(ip), Hostname: strings.TrimSpace(hostname), Entry: strings.TrimSpace(ip) + " " + strings.TrimSpace(hostname)}, nil
}

func defaultHostsPath() string {
	if runtime.GOOS == "windows" {
		root := os.Getenv("SystemRoot")
		if root == "" {
			root = `C:\Windows`
		}
		return filepath.Join(root, "System32", "drivers", "etc", "hosts")
	}
	return "/etc/hosts"
}

func readLines(path string) ([]string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := strings.ReplaceAll(string(contents), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return []string{}, nil
	}
	return strings.Split(text, "\n"), nil
}

func writeLines(path string, lines []string, become handler.Become) error {
	contents := ""
	if len(lines) > 0 {
		contents = strings.Join(lines, "\n") + "\n"
	}
	if !become.Enabled {
		return os.WriteFile(path, []byte(contents), 0o644)
	}
	return writeWithBecome(path, contents, become)
}

func writeWithBecome(path, contents string, become handler.Become) error {
	temporary, err := os.CreateTemp("", "ironstate-hosts-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.WriteString(contents); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	wrappedExecutable, args, err := becomeexec.WrapForBecome(become, executable, []string{writeFileCommand, path, temporaryPath})
	if err != nil {
		return err
	}
	output, err := exec.Command(wrappedExecutable, args...).CombinedOutput() //nolint:gosec // executable and paths are generated by this process
	if err != nil {
		return fmt.Errorf("write hosts file with become: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func writeHelper(args []string) int {
	if len(args) != 3 || args[0] != writeFileCommand {
		return 2
	}
	contents, err := os.ReadFile(args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.WriteFile(args[1], contents, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func hasMapping(lines []string, ip, hostname string) bool {
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == ip && contains(fields[1:], hostname) {
			return true
		}
	}
	return false
}

func removeMapping(lines []string, ip, hostname string) []string {
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == ip && contains(fields[1:], hostname) {
			continue
		}
		filtered = append(filtered, line)
	}
	return filtered
}

func removeHostname(lines []string, hostname string) []string {
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && !strings.HasPrefix(fields[0], "#") && contains(fields[1:], hostname) {
			continue
		}
		filtered = append(filtered, line)
	}
	return filtered
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func resultFor(spec hostsSpec) handler.ExecResult {
	return handler.ExecResult{RC: 0, Extra: map[string]any{"value": map[string]any{"path": spec.Path, "ip": spec.IP, "hostname": spec.Hostname}}}
}

func failedResult(err error) handler.ExecResult {
	message := err.Error()
	return handler.ExecResult{RC: 1, Stderr: message, StderrLines: []string{message}}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == writeFileCommand {
		os.Exit(writeHelper(os.Args[1:]))
	}
	plugin.Serve(map[string]handler.Handler{
		"ensure_entry": hostsHandler{},
	})
}
