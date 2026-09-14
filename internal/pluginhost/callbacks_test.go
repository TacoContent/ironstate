package pluginhost

import (
	"context"
	"testing"

	"github.com/TacoContent/ironstate/internal/engine"
	pluginpb "github.com/TacoContent/ironstate/sdk/proto"
)

func TestHostCallbackLogUsesEngineInfo(t *testing.T) {
	originalInfo := engine.Info
	t.Cleanup(func() { engine.Info = originalInfo })
	var got string
	engine.Info = func(format string, args ...any) { got = format }

	_, err := (hostCallbackServer{}).Log(context.Background(), &pluginpb.LogRequest{Message: "plugin message"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "%s" {
		t.Fatalf("engine.Info format = %q, want %%s", got)
	}
}
