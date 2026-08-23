package handlers

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// goHandler ports Handlers/Go.psm1 (Go binaries via 'go install').
type goHandler struct{}

var goBinDirCache string

func goBinDir() string {
	if goBinDirCache != "" {
		return goBinDirCache
	}
	if result, err := runner.Run("go", []string{"env", "GOBIN"}); err == nil {
		if gobin := strings.TrimSpace(result.Stdout); gobin != "" {
			goBinDirCache = gobin
			return goBinDirCache
		}
	}
	gopath := ""
	if result, err := runner.Run("go", []string{"env", "GOPATH"}); err == nil {
		gopath = strings.TrimSpace(result.Stdout)
	}
	if gopath == "" {
		home, _ := os.UserHomeDir()
		gopath = filepath.Join(home, "go")
	}
	goBinDirCache = filepath.Join(gopath, "bin")
	return goBinDirCache
}

func goBinaryPath(item map[string]any) string {
	pkg := getString(item, "package")
	binName := path.Base(pkg) + ".exe"
	return filepath.Join(goBinDir(), binName)
}

func (goHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	return fileExists(goBinaryPath(item)), nil
}

func (goHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	pkg := getString(item, "package")
	version := getStringOr(item, "version", "latest")
	if action == engine.ActionUninstall {
		return "remove " + goBinaryPath(item), nil
	}
	return "go install " + pkg + "@" + version, nil
}

func (goHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkg := getString(item, "package")
	version := getStringOr(item, "version", "latest")
	result := runExternalCommand("go", []string{"install", pkg + "@" + version})
	if result.RC != 0 {
		engine.Warn("go install %s@%s exited with code %d", pkg, version, result.RC)
	}
	return result, nil
}

func (goHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	binPath := goBinaryPath(item)
	if fileExists(binPath) {
		_ = os.Remove(binPath)
	}
	return engine.ExecResult{}, nil
}
