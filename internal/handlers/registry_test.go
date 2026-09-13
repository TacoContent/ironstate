package handlers

import (
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/internal/engine"
)

func TestRegistryMergesExternalHandlerIntoBothViews(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Merge(map[string]engine.Handler{"acme.hosts.ensure_entry": logHandler{}}); err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}
	if registry.Handlers()["acme.hosts.ensure_entry"] == nil {
		t.Fatal("merged handler missing from dispatch view")
	}
	found := false
	for _, name := range registry.ModuleNames() {
		if name == "acme.hosts.ensure_entry" {
			found = true
		}
	}
	if !found {
		t.Fatal("merged handler missing from module-name view")
	}
}

func TestRegistryRejectsBuiltinCollision(t *testing.T) {
	err := NewRegistry().Merge(map[string]engine.Handler{"shell": logHandler{}})
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("Merge error = %v, want collision error", err)
	}
}
