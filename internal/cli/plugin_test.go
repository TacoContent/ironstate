package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/TacoContent/ironstate/internal/engine"
	"github.com/TacoContent/ironstate/internal/pluginhost"
)

func TestParsePluginReference(t *testing.T) {
	namespace, version, err := parsePluginReference("acme.hosts@v1.2.3")
	if err != nil || namespace != "acme.hosts" || version != "v1.2.3" {
		t.Fatalf("parsePluginReference = %q, %q, %v", namespace, version, err)
	}
	namespace, version, err = parsePluginReference("acme.hosts")
	if err != nil || namespace != "acme.hosts" || version != "latest" {
		t.Fatalf("default parsePluginReference = %q, %q, %v", namespace, version, err)
	}
}

func TestParsePluginReferenceRejectsInvalidValue(t *testing.T) {
	if _, _, err := parsePluginReference("acme.hosts@"); err == nil {
		t.Fatal("parsePluginReference accepted an empty version")
	}
}

func TestParsePluginTestItemAcceptsJSONAndYAML(t *testing.T) {
	for _, raw := range []string{`{"name":"build","enabled":true}`, "name: build\nenabled: true"} {
		item, err := parsePluginTestItem(raw)
		if err != nil || item["name"] != "build" || item["enabled"] != true {
			t.Fatalf("parsePluginTestItem(%q) = %#v, %v", raw, item, err)
		}
	}
	if _, err := parsePluginTestItem("- not-a-mapping"); err == nil {
		t.Fatal("parsePluginTestItem accepted a list")
	}
}

func TestPluginTestCommandDryRunAndApply(t *testing.T) {
	store := pluginTestStoreFixture(t)
	originalStore, originalLaunch := pluginTestStore, pluginTestLaunch
	t.Cleanup(func() {
		pluginTestStore, pluginTestLaunch = originalStore, originalLaunch
	})
	pluginTestStore = func() (*pluginhost.Store, error) { return store, nil }

	handler := &pluginTestHandler{}
	pluginTestLaunch = func(*exec.Cmd) (pluginTestClient, error) {
		return pluginTestFakeClient{handlers: map[string]engine.Handler{"entry": handler}}, nil
	}

	command := newPluginTestCommand()
	var stdout, stderr bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&stderr)
	command.SetArgs([]string{"acme.hosts", "--handler", "entry", "--item", `{"state":"absent"}`, "--apply"})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if handler.tests != 1 || handler.describes != 1 || handler.uninstalls != 1 || handler.installs != 0 || !handler.apply {
		t.Fatalf("handler calls = %#v", handler)
	}
	var result pluginTestResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v\n%s", err, stdout.String())
	}
	if result.Action != engine.ActionUninstall || result.Exec.Stdout != "removed" {
		t.Fatalf("result = %#v", result)
	}
	if stderr.String() != "would remove entry\n" {
		t.Fatalf("description = %q", stderr.String())
	}
}

func TestRunPluginTestDryRunDoesNotMutate(t *testing.T) {
	handler := &pluginTestHandler{}
	command := newPluginTestCommand()
	command.SetOut(new(bytes.Buffer))
	command.SetErr(new(bytes.Buffer))
	if err := runPluginTest(command, handler, "entry", map[string]any{"state": "present"}, false); err != nil {
		t.Fatalf("runPluginTest returned error: %v", err)
	}
	if handler.tests != 1 || handler.describes != 1 || handler.installs != 0 || handler.uninstalls != 0 || handler.apply {
		t.Fatalf("handler calls = %#v", handler)
	}
}

func TestRunPluginBenchEmitsTimingSummary(t *testing.T) {
	handler := &pluginTestHandler{}
	command := newPluginBenchCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	if err := runPluginBench(command, handler, "entry", map[string]any{"state": "present"}, "test", 3, false); err != nil {
		t.Fatalf("runPluginBench returned error: %v", err)
	}
	var result pluginBenchResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode benchmark result: %v", err)
	}
	if result.Handler != "entry" || result.Operation != "test" || result.Iterations != 3 || result.TotalNS < 0 || result.AverageNS < 0 || result.MinNS > result.MaxNS || result.AverageNS < float64(result.MinNS) || result.AverageNS > float64(result.MaxNS) {
		t.Fatalf("benchmark result = %+v", result)
	}
	if handler.tests != 3 {
		t.Fatalf("handler test calls = %d, want 3", handler.tests)
	}
}

func TestValidatePluginBenchOperation(t *testing.T) {
	if err := validatePluginBenchOperation("wat"); err == nil {
		t.Fatal("accepted unsupported benchmark operation")
	}
}

func TestPluginTestCommandRejectsUndeclaredHandler(t *testing.T) {
	store := pluginTestStoreFixture(t)
	originalStore, originalLaunch := pluginTestStore, pluginTestLaunch
	t.Cleanup(func() {
		pluginTestStore, pluginTestLaunch = originalStore, originalLaunch
	})
	pluginTestStore = func() (*pluginhost.Store, error) { return store, nil }
	pluginTestLaunch = func(*exec.Cmd) (pluginTestClient, error) {
		return pluginTestFakeClient{handlers: map[string]engine.Handler{}}, nil
	}

	command := newPluginTestCommand()
	command.SetArgs([]string{"acme.hosts", "--handler", "missing", "--item", "name: build"})
	if err := command.Execute(); err == nil {
		t.Fatal("Execute accepted an undeclared handler")
	}
}

func pluginTestStoreFixture(t *testing.T) *pluginhost.Store {
	t.Helper()
	store := pluginhost.NewStore(t.TempDir())
	binary := filepath.Join(t.TempDir(), "ironstate-handler-hosts")
	if err := os.WriteFile(binary, []byte("fixture"), 0o755); err != nil { //nolint:gosec // fixture only needs to exist for the store
		t.Fatal(err)
	}
	if _, err := store.StorePlugin(pluginhost.Manifest{
		Organization: "acme", Name: "hosts", Version: "v1.0.0", Source: "github.com/acme/ironstate-handler-hosts",
	}, binary); err != nil {
		t.Fatal(err)
	}
	return store
}

type pluginTestFakeClient struct{ handlers map[string]engine.Handler }

func (c pluginTestFakeClient) Handlers() map[string]engine.Handler { return c.handlers }
func (pluginTestFakeClient) Close() error                          { return nil }

type pluginTestHandler struct {
	tests, describes, installs, uninstalls int
	apply                                  bool
}

func (h *pluginTestHandler) Test(map[string]any, string, engine.Context) (bool, error) {
	h.tests++
	return false, nil
}

func (h *pluginTestHandler) Describe(_ map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	h.describes++
	h.apply = ctx.Apply
	if action == engine.ActionUninstall {
		return "would remove entry", nil
	}
	return "would add entry", nil
}

func (h *pluginTestHandler) Install(map[string]any, string, engine.Context) (engine.ExecResult, error) {
	h.installs++
	return engine.ExecResult{Stdout: "installed"}, nil
}

func (h *pluginTestHandler) Uninstall(map[string]any, string, engine.Context) (engine.ExecResult, error) {
	h.uninstalls++
	return engine.ExecResult{Stdout: "removed"}, nil
}
