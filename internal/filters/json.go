package filters

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// lookPath is overridable in tests so json_query's jq-present/jq-absent
// branches can both be exercised deliberately (docs/plans/go-rewrite.md
// §4.5 — neither branch should depend on the test runner's real PATH).
var lookPath = exec.LookPath

// runJQ is overridable in tests for the same reason.
var runJQ = func(filterExpr string, input []byte) ([]byte, error) {
	cmd := exec.Command("jq", "-r", filterExpr) //nolint:gosec // 'jq' with a fixed argv position for a user-authored query string, same trust boundary as every other CLI this tool shells out to
	cmd.Stdin = bytes.NewReader(input)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func registerJSONFilters(r *Registry) {
	r.Register("from_json", filterFromJSON)
	r.Register("json_query", filterJSONQuery)
}

func filterFromJSON(value any, _ []any) (any, error) {
	s, ok := value.(string)
	if !ok {
		if value == nil {
			return nil, nil
		}
		return nil, fmt.Errorf("from_json filter requires a string value")
	}
	var out any
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func filterJSONQuery(value any, args []any) (any, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("json_query filter requires at least one argument")
	}
	query := toStr(args[0])

	if _, err := lookPath("jq"); err == nil {
		input, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		out, err := runJQ(query, input)
		if err != nil {
			return nil, err
		}
		var result any
		trimmed := bytes.TrimSpace(out)
		if len(trimmed) == 0 {
			return nil, nil
		}
		if err := json.Unmarshal(trimmed, &result); err != nil {
			// jq -r on a plain string emits it unquoted, which isn't valid
			// JSON on its own — fall back to the raw trimmed text.
			return strings.TrimSpace(string(trimmed)), nil
		}
		return result, nil
	}

	// No jq on PATH: same limitation as the PowerShell fallback
	// (Select-Object -ExpandProperty <query>) — only a single bare
	// property name against a map is supported, not full jq syntax.
	m, ok := value.(map[string]any)
	if !ok {
		return nil, nil
	}
	v, present := m[query]
	if !present {
		return nil, nil
	}
	return v, nil
}
