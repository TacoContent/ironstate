package remoteexec

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

func compileInventorySchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	data, err := os.ReadFile("../../inventory.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	const id = "https://raw.githubusercontent.com/TacoContent/ironstate/develop/inventory.schema.json"
	c := jsonschema.NewCompiler()
	if err := c.AddResource(id, doc); err != nil {
		t.Fatal(err)
	}
	schema, err := c.Compile(id)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

// yamlToJSON gives the validator plain JSON types.
func yamlToJSON(t *testing.T, content string) any {
	t.Helper()
	var v any
	if err := yaml.Unmarshal([]byte(content), &v); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestInventorySchemaMatchesLoader(t *testing.T) {
	schema := compileInventorySchema(t)
	if err := schema.Validate(yamlToJSON(t, sampleInventory)); err != nil {
		t.Fatalf("sample inventory fails the schema: %v", err)
	}
	for _, bad := range []string{
		"hosts:\n  a: { adress: x }\n",
		"hosts:\n  a: { platform: plan9 }\n",
		"hosts:\n  all: {}\n",
		"hosts:\n  a: { port: 0 }\n",
		"extra: 1\n",
	} {
		if err := schema.Validate(yamlToJSON(t, bad)); err == nil {
			t.Errorf("schema accepted %q", bad)
		}
	}
}
