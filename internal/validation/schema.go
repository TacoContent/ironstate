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

const (
	remoteSchemaBase = "https://raw.githubusercontent.com/TacoContent/ironstate/refs/heads/develop/"
	RemoteSchemaURL  = remoteSchemaBase + PlaybookSchema

	// PlaybookSchema validates playbooks, packages, roles, and overlays.
	PlaybookSchema = "ironstate.schema.json"
	// InventorySchema validates remote-execution inventory files.
	InventorySchema = "inventory.schema.json"

	lockFileName = "ironstate.lock.yaml"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}
var schemaLoader = loadSchema

// ValidateFile validates one YAML playbook against ironstate.schema.json.
// Schema lookup is local-first so offline validation works from a checkout or
// release archive, with the develop-branch schema as the fallback.
func ValidateFile(playbookPath string) error {
	return validateFileAgainst(playbookPath, PlaybookSchema)
}

// ValidateInventoryFile validates one inventory YAML file against
// inventory.schema.json.
func ValidateInventoryFile(inventoryPath string) error {
	return validateFileAgainst(inventoryPath, InventorySchema)
}

func validateFileAgainst(playbookPath, schemaName string) error {
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

	schemaData, schemaSource, err := schemaLoader(playbookPath, schemaName)
	if err != nil {
		return err
	}
	var schemaDoc any
	if err := json.Unmarshal(schemaData, &schemaDoc); err != nil {
		return fmt.Errorf("schema %s is not valid JSON: %w", schemaSource, err)
	}

	schemaURL := remoteSchemaBase + schemaName
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(schemaURL, schemaDoc); err != nil {
		return fmt.Errorf("cannot load schema %s: %w", schemaSource, err)
	}
	schema, err := compiler.Compile(schemaURL)
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
// overlays are standalone schema documents in the playbook layout. Inventory
// files are validated against inventory.schema.json; the plugin lockfile and
// dot-directories such as .git and .github are skipped.
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
		if entry.IsDir() {
			if path != rootDir && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Clean(path) == filepath.Clean(abs) || strings.EqualFold(entry.Name(), lockFileName) {
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
		validate := ValidateFile
		if file != abs && isInventoryFile(file) {
			validate = ValidateInventoryFile
		}
		if err := validate(file); err != nil {
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

// isInventoryFile reports whether a YAML document is shaped like an
// inventory (or an inventory-directory fragment): a mapping whose top-level
// keys are all inventory keys.
func isInventoryFile(path string) bool {
	data, err := os.ReadFile(path) //nolint:gosec // file found under the user-selected playbook directory
	if err != nil {
		return false
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc) == 0 {
		return false
	}
	for key := range doc {
		switch key {
		case "defaults", "hosts", "groups":
		default:
			return false
		}
	}
	return true
}

func loadSchema(playbookPath, schemaName string) ([]byte, string, error) {
	candidates := []string{schemaName}
	if abs, err := filepath.Abs(playbookPath); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(abs), schemaName))
	}
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(executable), schemaName))
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

	remoteURL := remoteSchemaBase + schemaName
	response, err := httpClient.Get(remoteURL)
	if err == nil {
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
			data, readErr := io.ReadAll(response.Body)
			if readErr == nil {
				return data, remoteURL, nil
			}
		}
	}
	return nil, "", fmt.Errorf("%s cannot be validated because the schema %s could not be found", playbookPath, schemaName)
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
