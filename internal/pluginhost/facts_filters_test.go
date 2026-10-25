package pluginhost

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/internal/engine"
	"github.com/TacoContent/ironstate/sdk/handler"
	sdkplugin "github.com/TacoContent/ironstate/sdk/plugin"
)

type sampleFactHandler struct{}

func (sampleFactHandler) Test(map[string]any, string, handler.Context) (bool, error) {
	return false, nil
}

func (sampleFactHandler) Describe(map[string]any, handler.Action, handler.Context) (string, error) {
	return "sample fact", nil
}

func (sampleFactHandler) Install(map[string]any, string, handler.Context) (handler.ExecResult, error) {
	return handler.ExecResult{Extra: map[string]any{"value": "42"}}, nil
}

func (sampleFactHandler) Uninstall(map[string]any, string, handler.Context) (handler.ExecResult, error) {
	return handler.ExecResult{}, nil
}

func (sampleFactHandler) FactName(item map[string]any) (string, bool) {
	name, ok := item["fact"].(string)
	return name, ok && name != ""
}

type samplePlainHandler struct{ sampleFactHandler }

// Hides FactName so this handler is not a fact producer.
func (samplePlainHandler) FactName() {}

func TestPluginFactsAndFiltersHelperProcess(t *testing.T) {
	if os.Getenv("IRONSTATE_PLUGIN_FACTS_HELPER") != "1" {
		return
	}
	sdkplugin.ServeWithConfig(sdkplugin.ServeConfig{
		Handlers: map[string]handler.Handler{
			"answer": sampleFactHandler{},
			"plain":  samplePlainHandler{},
		},
		Filters: map[string]handler.Filter{
			"shout": func(value any, args []any) (any, error) {
				suffix := "!"
				if len(args) > 0 {
					suffix = fmt.Sprint(args[0])
				}
				return strings.ToUpper(fmt.Sprint(value)) + suffix, nil
			},
			"fail": func(any, []any) (any, error) { return nil, fmt.Errorf("boom") },
		},
	})
}

func launchFactsFiltersHelper(t *testing.T) *Client {
	t.Helper()
	command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestPluginFactsAndFiltersHelperProcess$") //nolint:gosec // test-only current binary with a fixed test selector
	command.Env = append(os.Environ(), "IRONSTATE_PLUGIN_FACTS_HELPER=1")
	client, err := Launch(command)
	if err != nil {
		t.Fatalf("launch plugin: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestPluginFactProducerIsExposedOnlyWhenDeclared(t *testing.T) {
	client := launchFactsFiltersHelper(t)
	handlers := client.Handlers()

	producer, ok := handlers["answer"].(engine.FactProducer)
	if !ok {
		t.Fatalf("fact-producing plugin handler has type %T, want engine.FactProducer", handlers["answer"])
	}
	name, produced := producer.FactName(map[string]any{"fact": "answer_value"})
	if name != "answer_value" || !produced {
		t.Fatalf("FactName = %q, %v; want answer_value, true", name, produced)
	}
	if _, produced := producer.FactName(map[string]any{}); produced {
		t.Fatal("FactName reported a fact for an item without one")
	}
	if _, ok := handlers["plain"].(engine.FactProducer); ok {
		t.Fatal("plain plugin handler must not be a fact producer")
	}
}

func TestPluginFiltersRoundTrip(t *testing.T) {
	client := launchFactsFiltersHelper(t)
	if got := strings.Join(client.FilterNames(), ","); got != "fail,shout" {
		t.Fatalf("FilterNames = %q, want fail,shout", got)
	}
	filters := client.Filters()
	got, err := filters["shout"]("hi", []any{"?"})
	if err != nil || got != "HI?" {
		t.Fatalf("shout = %#v, %v; want HI?", got, err)
	}
	got, err = filters["shout"]([]string{"a"}, nil)
	if err != nil || got != "[A]!" {
		t.Fatalf("shout typed slice = %#v, %v; want [A]!", got, err)
	}
	if _, err := filters["fail"]("x", nil); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("fail filter error = %v, want boom", err)
	}
}
