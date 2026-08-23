package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/internal/engine"
	"github.com/TacoContent/ironstate/internal/filters"
)

func testCtxWithFilters(flat map[string]any) engine.Context {
	if flat == nil {
		flat = map[string]any{}
	}
	return engine.Context{Flat: flat, Filters: filters.New(), Apply: true}
}

func TestTemplateHandlerJinjaRenderAndTest(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "tpl.j2")
	if err := os.WriteFile(src, []byte("Hello {{ name }}!"), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "out.txt")

	h := templateHandler{}
	item := map[string]any{"src": src, "dest": dest, "engine": "jinja"}
	ctx := testCtxWithFilters(map[string]any{"name": "World"})

	installed, err := h.Test(item, "", ctx)
	if err != nil || installed {
		t.Fatalf("installed=%v err=%v, want false before rendering", installed, err)
	}
	if _, err := h.Install(item, "", ctx); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest) //nolint:gosec // dest is a t.TempDir()-derived path this same test just wrote, not user input
	if err != nil || string(data) != "Hello World!" {
		t.Fatalf("data=%q err=%v", data, err)
	}
	installed, err = h.Test(item, "", ctx)
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true after rendering", installed, err)
	}
}

func TestTemplateHandlerOwnVarsLayerOnTopOfContext(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "tpl.j2")
	if err := os.WriteFile(src, []byte("{{ profile.name }}"), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "out.txt")

	h := templateHandler{}
	item := map[string]any{
		"src": src, "dest": dest, "engine": "jinja",
		"vars": map[string]any{"profile": map[string]any{"name": "alice"}},
	}
	if _, err := h.Install(item, "", testCtxWithFilters(nil)); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(dest) //nolint:gosec // dest is a t.TempDir()-derived path this same test just wrote, not user input
	if string(data) != "alice" {
		t.Fatalf("data = %q", data)
	}
}

func TestTemplateHandlerUnknownEngineErrors(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "tpl.txt")
	if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := templateHandler{}
	item := map[string]any{"src": src, "dest": filepath.Join(dir, "out.txt"), "engine": "eps"}
	if _, err := h.Install(item, "", testCtxWithFilters(nil)); err == nil {
		t.Fatal("expected an error for the dropped 'eps' engine")
	}
}

func TestBlockInFileTemplateFieldRendersViaJinja(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "block.j2")
	if err := os.WriteFile(src, []byte("managed: {{ value }}"), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "dest.conf")

	h := blockInFileHandler{}
	item := map[string]any{
		"dest":   dest,
		"create": true,
		"template": map[string]any{
			"src":    src,
			"engine": "jinja",
			"vars":   map[string]any{"value": "x"},
		},
	}
	ctx := testCtxWithFilters(nil)
	if _, err := h.Install(item, "task", ctx); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest) //nolint:gosec // dest is a t.TempDir()-derived path this same test just wrote, not user input
	if err != nil || !strings.Contains(string(data), "managed: x") {
		t.Fatalf("data=%q err=%v", data, err)
	}
	installed, err := h.Test(item, "task", ctx)
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v", installed, err)
	}
}
