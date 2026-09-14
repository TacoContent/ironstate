package pluginhost

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/TacoContent/ironstate/internal/engine"
	"github.com/TacoContent/ironstate/internal/filters"
)

func TestSampleHostsPluginRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not available to build the sample plugin")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	sampleDir := filepath.Join(root, "examples", "ironstate-handler-hosts")
	binaryName := "ironstate-handler-hosts"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binary := filepath.Join(t.TempDir(), binaryName)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, ".") //nolint:gosec // fixed test tool and repository-local package
	build.Dir = sampleDir
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build sample plugin: %v\n%s", err, output)
	}

	client, err := LaunchWithCallbacks(exec.Command(binary), filters.New()) //nolint:gosec // test binary was built from repository-local source
	if err != nil {
		t.Fatalf("launch sample plugin: %v", err)
	}
	defer func() { _ = client.Close() }()

	qualified, err := client.QualifiedHandlers("acme.hosts")
	if err != nil {
		t.Fatalf("qualify sample handlers: %v", err)
	}
	handler, ok := qualified["acme.hosts.entry"]
	if !ok {
		t.Fatalf("qualified handlers = %v", qualified)
	}
	if provider, ok := handler.(engine.EmojiProvider); !ok || provider.Emoji() != "📇" {
		t.Fatalf("handler emoji = %#v, want 📇", handler)
	}

	hostsPath := filepath.Join(t.TempDir(), "hosts")
	item := map[string]any{"path": hostsPath, "ip": "10.0.0.12", "hostname": "build.local", "name": "build_host", "callback_condition": "hostname == 'build.local'", "callback_template": "{{ hostname }}"}
	ctxValue := engine.Context{Flat: map[string]any{"hostname": "build.local"}, Apply: true}
	description, err := handler.Describe(item, engine.ActionInstall, ctxValue)
	if err != nil || description != "build.local" {
		t.Fatalf("callback Describe = %q, %v; want build.local", description, err)
	}
	satisfied, err := handler.Test(item, "build_host", ctxValue)
	if err != nil || satisfied {
		t.Fatalf("initial Test = %v, %v; want false, nil", satisfied, err)
	}
	result, err := handler.Install(item, "build_host", ctxValue)
	if err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if result.RC != 0 || result.Extra["value"] == nil {
		t.Fatalf("Install result = %#v", result)
	}
	satisfied, err = handler.Test(item, "build_host", ctxValue)
	if err != nil || !satisfied {
		t.Fatalf("installed Test = %v, %v; want true, nil", satisfied, err)
	}
	contents, err := os.ReadFile(hostsPath) //nolint:gosec // hostsPath is a t.TempDir()-derived path this test just wrote
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "10.0.0.12 build.local\n" {
		t.Fatalf("hosts content = %q", contents)
	}
	if _, err := handler.Uninstall(item, "build_host", ctxValue); err != nil {
		t.Fatalf("Uninstall returned error: %v", err)
	}
	satisfied, err = handler.Test(item, "build_host", ctxValue)
	if err != nil || satisfied {
		t.Fatalf("uninstalled Test = %v, %v; want false, nil", satisfied, err)
	}
}
