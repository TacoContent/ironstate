package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/internal/engine"
	ironexec "github.com/TacoContent/ironstate/internal/exec"
)

func withStatBackend(t *testing.T, backend statBackend, run fakeRunnerFunc) {
	t.Helper()
	oldResolve, oldRunner := resolveStatBackend, runner
	resolveStatBackend = func() (statBackend, error) { return backend, nil }
	runner = run
	t.Cleanup(func() { resolveStatBackend, runner = oldResolve, oldRunner })
}

func TestStatHandlerMissingPathReportsNotExists(t *testing.T) {
	withStatBackend(t, statBackend{exe: "stat", kind: statBackendCoreutils}, func(string, []string) (ironexec.Result, error) {
		t.Fatal("runner should not be called for a missing path")
		return ironexec.Result{}, nil
	})
	path := filepath.Join(t.TempDir(), "nope")
	res, err := statHandler{}.Install(map[string]any{"path": path}, "", testCtx())
	if err != nil || res.RC != 0 {
		t.Fatalf("Install: rc=%d err=%v stderr=%s", res.RC, err, res.Stderr)
	}
	data := res.Extra["stat"].(map[string]any)
	if data["exists"] != false {
		t.Fatalf("exists = %v, want false", data["exists"])
	}
}

func TestStatHandlerRequiresPath(t *testing.T) {
	res, _ := statHandler{}.Install(map[string]any{}, "", testCtx())
	if res.RC != 1 {
		t.Fatalf("rc = %d, want 1", res.RC)
	}
}

func TestStatHandlerCoreutilsBackend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(path, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	var gotArgs []string
	withStatBackend(t, statBackend{exe: "stat", kind: statBackendCoreutils}, func(exe string, args []string) (ironexec.Result, error) {
		gotArgs = args
		out := strings.Join([]string{"81a4", "-rw-r--r--", "1000", "me", "100", "users", "42", "2049", "1", "2", "10", "20", "30"}, "\n") + "\n"
		return ironexec.Result{Stdout: out}, nil
	})
	res, err := statHandler{}.Install(map[string]any{"path": path, "follow": true}, "", testCtx())
	if err != nil || res.RC != 0 {
		t.Fatalf("Install: rc=%d err=%v stderr=%s", res.RC, err, res.Stderr)
	}
	if gotArgs[0] != "-L" || gotArgs[len(gotArgs)-2] != "--" || gotArgs[len(gotArgs)-1] != path {
		t.Fatalf("args = %q", gotArgs)
	}
	data := res.Extra["stat"].(map[string]any)
	want := map[string]any{
		"exists": true, "isreg": true, "isdir": false, "islnk": false, "mode": "0644",
		"permissions": "-rw-r--r--", "uid": 1000.0, "pw_name": "me", "gr_name": "users",
		"inode": "42", "size": 2.0, "mtime": 20.0, "backend": "coreutils",
	}
	for k, v := range want {
		if data[k] != v {
			t.Errorf("%s = %#v, want %#v", k, data[k], v)
		}
	}
}

func TestStatHandlerBSDBackendParsesOctalMode(t *testing.T) {
	dir := t.TempDir()
	withStatBackend(t, statBackend{exe: "stat", kind: statBackendBSD}, func(exe string, args []string) (ironexec.Result, error) {
		if args[0] != "-f" {
			t.Fatalf("args = %q", args)
		}
		out := strings.Join([]string{"40755", "drwxr-xr-x", "501", "me", "20", "staff", "7", "16777220", "3", "96", "10", "20", "30"}, "\n")
		return ironexec.Result{Stdout: out}, nil
	})
	res, _ := statHandler{}.Install(map[string]any{"path": dir}, "", testCtx())
	data := res.Extra["stat"].(map[string]any)
	if data["isdir"] != true || data["mode"] != "0755" || data["backend"] != "bsd" {
		t.Fatalf("data = %#v", data)
	}
}

func TestStatHandlerPwshBackend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "it's.txt")
	if err := os.WriteFile(path, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	withStatBackend(t, statBackend{exe: "pwsh", kind: statBackendPwsh}, func(exe string, args []string) (ironexec.Result, error) {
		if !strings.Contains(args[len(args)-1], "it''s.txt") {
			t.Fatalf("path not escaped in script: %s", args[len(args)-1])
		}
		return ironexec.Result{Stdout: `{"isdir":false,"isreg":true,"islnk":false,"lnk_target":null,"size":2.0,"mode":null,"pw_name":"me"}`}, nil
	})
	res, _ := statHandler{}.Install(map[string]any{"path": path}, "", testCtx())
	data := res.Extra["stat"].(map[string]any)
	if data["isreg"] != true || data["size"] != 2.0 || data["backend"] != "pwsh" {
		t.Fatalf("data = %#v", data)
	}
	if _, present := data["lnk_target"]; present {
		t.Fatal("null fields should be omitted")
	}
}

func TestPwshSingleQuoteEscapesAllQuoteForms(t *testing.T) {
	got := pwshSingleQuote("a'b\u2019c")
	if got != "'a''b\u2019\u2019c'" {
		t.Fatalf("got %q", got)
	}
}

func TestStatHandlerIsReadOnly(t *testing.T) {
	var h engine.Handler = statHandler{}
	if ro, ok := h.(engine.ReadOnlyHandler); !ok || !ro.ReadOnly() {
		t.Fatal("stat should implement engine.ReadOnlyHandler")
	}
}

func TestStatViaPwshReal(t *testing.T) {
	exe, err := statLookPath("pwsh")
	if err != nil {
		t.Skip("pwsh not on PATH")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "it's.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := statViaPwsh(exe, path, false)
	if err != nil {
		t.Fatal(err)
	}
	if data["isreg"] != true || data["isdir"] != false || data["size"] != 5.0 {
		t.Fatalf("data = %#v", data)
	}
	if _, ok := data["mtime"].(float64); !ok {
		t.Fatalf("mtime = %#v", data["mtime"])
	}
	data, err = statViaPwsh(exe, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if data["isdir"] != true {
		t.Fatalf("dir data = %#v", data)
	}
}

// Runs whichever real backend this machine has.
func TestStatHandlerRealBackend(t *testing.T) {
	if _, err := resolveStatBackend(); err != nil {
		t.Skip(err)
	}
	path := filepath.Join(t.TempDir(), "real.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := statHandler{}.Install(map[string]any{"path": path}, "", testCtx())
	if err != nil || res.RC != 0 {
		t.Fatalf("Install: rc=%d err=%v stderr=%s", res.RC, err, res.Stderr)
	}
	data := res.Extra["stat"].(map[string]any)
	if data["exists"] != true || data["isreg"] != true || data["size"] != 5.0 {
		t.Fatalf("data = %#v", data)
	}
}
