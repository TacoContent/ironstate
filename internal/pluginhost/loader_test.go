package pluginhost

import (
	"testing"

	"github.com/TacoContent/ironstate/internal/engine"
)

func TestQualifiedHandlersPrefixesDiscoveredNames(t *testing.T) {
	client := &Client{handlers: map[string]engine.Handler{"entry": handlerAdapter{}}}
	handlers, err := client.QualifiedHandlers("acme.hosts")
	if err != nil {
		t.Fatalf("QualifiedHandlers returned error: %v", err)
	}
	if handlers["acme.hosts.entry"] == nil {
		t.Fatal("qualified handler was not returned")
	}
}

func TestQualifiedHandlersRejectsInvalidNamespace(t *testing.T) {
	if _, err := (&Client{}).QualifiedHandlers("acme"); err == nil {
		t.Fatal("QualifiedHandlers accepted namespace without organization.plugin form")
	}
}
