package handlers

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/TacoContent/ironstate/internal/engine"
	ironexec "github.com/TacoContent/ironstate/internal/exec"
	"github.com/TacoContent/ironstate/internal/filters"
	"github.com/TacoContent/ironstate/internal/model"
	"github.com/TacoContent/ironstate/internal/tasks"
)

func TestShellSilentCapturesOutputWithoutLogging(t *testing.T) {
	recorder := &recordingRunner{responses: []ironexec.Result{{
		RC: 0, Stdout: "captured-out", StdoutLines: []string{"captured-out"},
		Stderr: "captured-err", StderrLines: []string{"captured-err"},
	}}}
	withRunner(t, recorder)
	var messages []string
	oldInfo, oldWarn := engine.Info, engine.Warn
	logger := func(format string, args ...any) { messages = append(messages, fmt.Sprintf(format, args...)) }
	engine.Info, engine.Warn = logger, logger
	t.Cleanup(func() { engine.Info, engine.Warn = oldInfo, oldWarn })
	result, err := (shellHandler{}).Install(map[string]any{
		"host": "pwsh", "command": "example", "silent": true,
	}, "", testCtx())
	if err != nil || result.Stdout != "captured-out" || result.Stderr != "captured-err" || len(messages) != 0 {
		t.Fatalf("result=%#v err=%v messages=%#v", result, err, messages)
	}
}

func TestScopedHomePathWorksAcrossBashAndZsh(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix shells")
	}
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is not installed")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not installed")
	}
	home := t.TempDir()
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "ironstate_path_fixture"), []byte("#!/bin/sh\nprintf 'fixture-version\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("ZDOTDIR", home)
	t.Setenv("SHELL", zsh)
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", originalPath)
	originalRunner := runner
	runner = ironexec.Default
	t.Cleanup(func() { runner = originalRunner })

	doc, err := model.Unmarshal([]byte(`
tasks:
  - fact: {name: local_bin_path, value: "~/.local/bin"}
  - environment:
      PATH: "${{ facts.local_bin_path }}:${{ envs.PATH }}"
    actions:
      - envs:
          PATH: "${{ facts.local_bin_path }}:${{ envs.PATH }}"
        actions:
          - fact:
              name: fixture_installed
              shell: {type: bash, command: "command -v ironstate_path_fixture &> /dev/null"}
              value: "${{ rc == 0 }}"
            failed_when: false
          - when: facts.fixture_installed
            fact:
              name: fixture_version
              shell: {command: "ironstate_path_fixture"}
              value: "${{ stdout | trim }}"
`))
	if err != nil {
		t.Fatal(err)
	}
	list, err := model.TaskList(doc)
	if err != nil {
		t.Fatal(err)
	}
	fset := filters.New()
	leaves, err := tasks.Expand(list, tasks.Options{ModuleNames: AllModuleNames, Filters: fset})
	if err != nil {
		t.Fatal(err)
	}
	var gathered map[string]any
	results, stopped, err := engine.Run(leaves, engine.Options{
		Handlers: All(), Filters: fset,
		OnFactsGathered: func(facts map[string]any) { gathered = facts },
	})
	if err != nil || stopped || len(results) != 3 || gathered["fixture_installed"] != true || gathered["fixture_version"] != "fixture-version" {
		t.Fatalf("results=%#v stopped=%v err=%v facts=%#v", results, stopped, err, gathered)
	}
	if os.Getenv("PATH") != originalPath {
		t.Fatal("scoped PATH leaked")
	}
}
