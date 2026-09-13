package model

import "testing"

func TestPluginsParsesDeclarations(t *testing.T) {
	plugins, err := Plugins(map[string]any{"plugins": []any{map[string]any{"use": "acme.hosts@v1.2.3"}}})
	if err != nil || len(plugins) != 1 || plugins[0] != (Plugin{Namespace: "acme.hosts", Version: "v1.2.3"}) {
		t.Fatalf("Plugins = %#v, %v", plugins, err)
	}
}

func TestParsePluginRejectsInvalidValue(t *testing.T) {
	if _, err := ParsePlugin("acme.hosts"); err == nil {
		t.Fatal("ParsePlugin accepted missing version")
	}
}
