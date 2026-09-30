package validation

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/TacoContent/ironstate/internal/model"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

const RemoteSchemaURL = "https://raw.githubusercontent.com/TacoContent/ironstate/refs/heads/develop/ironstate.schema.json"

var httpClient = &http.Client{Timeout: 10 * time.Second}
var schemaLoader = loadSchema

// ValidateFile validates one YAML playbook against ironstate.schema.json.
// Schema lookup is local-first so offline validation works from a checkout or
// release archive, with the develop-branch schema as the fallback.
func ValidateFile(playbookPath string) error {
	data, err := os.ReadFile(playbookPath) //nolint:gosec // the path is the user-selected playbook
	if err != nil {
		return fmt.Errorf("cannot read playbook %s: %w", playbookPath, err)
	}

	var yamlDoc yaml.Node
	if err := yaml.Unmarshal(data, &yamlDoc); err != nil {
		return fmt.Errorf("cannot parse playbook %s: %w", playbookPath, err)
	}
	instance, err := model.Unmarshal(data)
	if err != nil {
		return fmt.Errorf("cannot parse playbook %s: %w", playbookPath, err)
	}

	schemaData, schemaSource, err := schemaLoader(playbookPath)
	if err != nil {
		return err
	}
	var schemaDoc any
	if err := json.Unmarshal(schemaData, &schemaDoc); err != nil {
		return fmt.Errorf("schema %s is not valid JSON: %w", schemaSource, err)
	}

	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(RemoteSchemaURL, schemaDoc); err != nil {
		return fmt.Errorf("cannot load schema %s: %w", schemaSource, err)
	}
	schema, err := compiler.Compile(RemoteSchemaURL)
	if err != nil {
		return fmt.Errorf("cannot compile schema %s: %w", schemaSource, err)
	}
	if err := schema.Validate(instance); err != nil {
		return formatValidationErrors(playbookPath, &yamlDoc, err)
	}
	return nil
}

// ValidatePlaybook validates the root playbook and every YAML document below
// its directory. Included packages, roles, host overlays, and variable
// overlays are standalone schema documents in the playbook layout.
func ValidatePlaybook(playbookPath string) ([]string, error) {
	abs, err := filepath.Abs(playbookPath)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve playbook path %s: %w", playbookPath, err)
	}
	rootDir := filepath.Dir(abs)
	files := []string{abs}
	err = filepath.WalkDir(rootDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Clean(path) == filepath.Clean(abs) {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".yml" || ext == ".yaml" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("cannot scan playbook directory %s: %w", rootDir, err)
	}
	sort.Strings(files)

	valid := make([]string, 0, len(files))
	var failures []string
	for _, file := range files {
		if err := ValidateFile(file); err != nil {
			failures = append(failures, err.Error())
			continue
		}
		valid = append(valid, file)
	}
	if len(failures) > 0 {
		return valid, fmt.Errorf("playbook validation failed:\n%s", strings.Join(failures, "\n"))
	}
	return valid, nil
}

func loadSchema(playbookPath string) ([]byte, string, error) {
	candidates := []string{"ironstate.schema.json"}
	if abs, err := filepath.Abs(playbookPath); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(abs), "ironstate.schema.json"))
	}
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(executable), "ironstate.schema.json"))
	}

	seen := map[string]bool{}
	for _, candidate := range candidates {
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		data, err := os.ReadFile(candidate) //nolint:gosec // candidates are fixed schema locations
		if err == nil {
			return data, candidate, nil
		}
	}

	response, err := httpClient.Get(RemoteSchemaURL)
	if err == nil {
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
			data, readErr := io.ReadAll(response.Body)
			if readErr == nil {
				return data, RemoteSchemaURL, nil
			}
		}
	}
	return nil, "", fmt.Errorf("playbook cannot be validated because the schema could not be found")
}

type sourceLocation struct {
	line   int
	column int
}

func formatValidationErrors(playbookPath string, yamlDoc *yaml.Node, validationErr error) error {
	root := validationErr
	if rootErr, ok := validationErr.(*jsonschema.ValidationError); ok {
		var leaves []*jsonschema.ValidationError
		collectValidationLeaves(rootErr, &leaves)
		locations := yamlLocations(yamlDoc)
		messages := make([]string, 0, len(leaves))
		for _, leaf := range leaves {
			path := leaf.InstanceLocation
			location := locations[pathKey(path)]
			line, column := location.line, location.column
			if line == 0 {
				line, column = 1, 1
			}
			instance := "/"
			if len(path) > 0 {
				instance = "/" + strings.Join(path, "/")
			}
			messages = append(messages, fmt.Sprintf("%s:%d:%d: %s (%s)", playbookPath, line, column, leaf.Error(), instance))
		}
		if len(messages) > 0 {
			return fmt.Errorf("playbook schema validation failed:\n%s", strings.Join(messages, "\n"))
		}
	}
	return fmt.Errorf("playbook schema validation failed for %s: %w", playbookPath, root)
}

func collectValidationLeaves(err *jsonschema.ValidationError, leaves *[]*jsonschema.ValidationError) {
	if len(err.Causes) == 0 {
		*leaves = append(*leaves, err)
		return
	}
	for _, cause := range err.Causes {
		collectValidationLeaves(cause, leaves)
	}
}

func yamlLocations(doc *yaml.Node) map[string]sourceLocation {
	locations := map[string]sourceLocation{}
	if doc == nil || len(doc.Content) == 0 {
		return locations
	}
	indexYAMLNode(doc.Content[0], nil, locations)
	return locations
}

func indexYAMLNode(node *yaml.Node, path []string, locations map[string]sourceLocation) {
	if node == nil {
		return
	}
	locations[pathKey(path)] = sourceLocation{line: node.Line, column: node.Column}
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			childPath := append(append([]string{}, path...), key.Value)
			indexYAMLNode(value, childPath, locations)
		}
	case yaml.SequenceNode:
		for i, value := range node.Content {
			indexYAMLNode(value, append(append([]string{}, path...), fmt.Sprint(i)), locations)
		}
	case yaml.AliasNode:
		indexYAMLNode(node.Alias, path, locations)
	}
}

func pathKey(path []string) string {
	return strings.Join(path, "\x00")
}
