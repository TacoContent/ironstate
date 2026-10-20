package engine

import (
	"reflect"
	"testing"

	"github.com/TacoContent/ironstate/internal/tasks"
)

func TestVariableNamespaceMatchesBareVariables(t *testing.T) {
	packageVars := map[string]any{"defaults": map[string]any{"value": "fallback"}, "option": "package"}
	siteVars := map[string]any{"option": "site"}
	flat := mergeFlatContext(
		map[string]any{"host": "gathered"}, nil, packageVars, nil, nil, siteVars,
		map[string]map[string]any{"result": {"rc": 0}},
	)
	vars := flat["vars"].(map[string]any)
	if vars["option"] != "site" || flat["option"] != "site" || !reflect.DeepEqual(vars["defaults"], flat["defaults"]) {
		t.Fatalf("vars namespace disagrees with bare names: %#v", flat)
	}
	for _, name := range []string{"host", "facts", "result", "inputs", "package"} {
		if _, exists := vars[name]; exists {
			t.Errorf("non-variable %q leaked into vars", name)
		}
	}
	vars["option"] = "changed"
	if siteVars["option"] != "site" || packageVars["option"] != "package" {
		t.Fatal("vars namespace mutated its source mappings")
	}
}

func TestVariableNamespaceAtDispatch(t *testing.T) {
	for _, isolated := range []bool{false, true} {
		t.Run(map[bool]string{false: "included", true: "isolated"}[isolated], func(t *testing.T) {
			h := &fakeHandler{}
			opts := baseOpts(map[string]Handler{"widget": h})
			opts.Apply = true
			opts.Vars = map[string]any{"option": "site", "secret": "must not leak"}
			item := leaf("widget", map[string]any{
				"fallback": "${{ missing | default(vars.defaults) }}",
				"option":   "${{ vars.option }}",
			})
			item.PackageVars = map[string]any{"defaults": map[string]any{"value": "fallback"}, "option": "package"}
			item.Isolated = isolated
			item.IsolatedVars = map[string]any{"option": "isolated"}
			results, stopped, err := RunLeaves([]tasks.Leaf{item}, opts, NewState())
			if err != nil || stopped || len(results) != 1 || h.installCall != 1 {
				t.Fatalf("results=%#v stopped=%v err=%v calls=%d", results, stopped, err, h.installCall)
			}
			want := "site"
			if isolated {
				want = "isolated"
			}
			if h.seenItem["option"] != want || !reflect.DeepEqual(h.seenItem["fallback"], item.PackageVars["defaults"]) {
				t.Fatalf("resolved item = %#v, want option=%s and native defaults map", h.seenItem, want)
			}
		})
	}
}
