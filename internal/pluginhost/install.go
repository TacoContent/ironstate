package pluginhost

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	sdkplugin "github.com/TacoContent/ironstate/sdk/plugin"
)

// Install resolves a plugin release, installs it with Go, validates its
// handshake, and stores the validated executable and manifest.
func Install(ctx context.Context, store *Store, namespace, version string) (Manifest, error) {
	if err := validateIdentity(namespace); err != nil {
		return Manifest{}, err
	}
	if version == "" {
		version = "latest"
	}
	parts := strings.Split(namespace, ".")
	module := pluginModulePath(parts[0], parts[1])
	resolved, err := resolveModuleVersion(ctx, module, version)
	if err != nil {
		return Manifest{}, err
	}
	if err := runGo(ctx, "install", pluginInstallTarget(module, resolved)); err != nil {
		return Manifest{}, err
	}
	binary, err := installedBinaryPath(ctx, parts[1])
	if err != nil {
		return Manifest{}, err
	}
	client, err := Launch(exec.CommandContext(ctx, binary)) //nolint:gosec // binary is discovered from Go's configured install directory
	if err != nil {
		return Manifest{}, fmt.Errorf("validate plugin %s@%s: %w", namespace, resolved, err)
	}
	handlers := client.Handlers()
	if err := client.Close(); err != nil {
		return Manifest{}, err
	}
	names := make([]string, 0, len(handlers))
	for name := range handlers {
		names = append(names, name)
	}
	return store.StorePlugin(Manifest{
		Organization:    parts[0],
		Name:            parts[1],
		Version:         resolved,
		Source:          module,
		HandlerNames:    names,
		ProtocolVersion: sdkplugin.ProtocolVersion,
	}, binary)
}

func pluginModulePath(organization, name string) string {
	return "github.com/" + organization + "/ironstate-handler-" + name
}

func pluginInstallTarget(module, version string) string {
	return module + "@" + version
}

func resolveModuleVersion(ctx context.Context, module, version string) (string, error) {
	if version != "latest" {
		return version, nil
	}
	command := exec.CommandContext(ctx, "go", "list", "-m", "-f", "{{.Version}}", module+"@latest") //nolint:gosec // module is derived from validated plugin identity
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("resolve latest plugin version for %s: %w", module, err)
	}
	resolved := strings.TrimSpace(string(output))
	if !validPathPart(resolved) {
		return "", fmt.Errorf("go resolved invalid plugin version %q", resolved)
	}
	return resolved, nil
}

func runGo(ctx context.Context, args ...string) error {
	command := exec.CommandContext(ctx, "go", args...) //nolint:gosec // args are fixed verbs plus a module path derived from validated plugin identity
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("go %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func installedBinaryPath(ctx context.Context, name string) (string, error) {
	command := exec.CommandContext(ctx, "go", "env", "-json", "GOBIN", "GOPATH")
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("read Go install paths: %w", err)
	}
	var paths struct {
		GOBIN  string
		GOPATH string
	}
	if err := json.Unmarshal(output, &paths); err != nil {
		return "", fmt.Errorf("decode Go install paths: %w", err)
	}
	directory := paths.GOBIN
	if directory == "" {
		directory = filepath.Join(paths.GOPATH, "bin")
	}
	path := filepath.Join(directory, binaryName(name))
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("installed plugin binary %q: %w", path, err)
	}
	return path, nil
}
