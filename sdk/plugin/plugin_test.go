package plugin

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	pluginlib "github.com/hashicorp/go-plugin"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/TacoContent/ironstate/sdk/handler"
	pluginpb "github.com/TacoContent/ironstate/sdk/proto"
)

const helperEnvironment = "IRONSTATE_SDK_PLUGIN_HELPER"

type fixtureHandler struct{}

func (fixtureHandler) Emoji() string { return "fixture" }

func (fixtureHandler) Test(item map[string]any, _ string, _ handler.Context) (bool, error) {
	enabled, _ := item["enabled"].(bool)
	return enabled, nil
}

func (fixtureHandler) Describe(_ map[string]any, action handler.Action, _ handler.Context) (string, error) {
	return string(action), nil
}

func (fixtureHandler) Install(_ map[string]any, _ string, _ handler.Context) (handler.ExecResult, error) {
	return handler.ExecResult{RC: 7, Extra: map[string]any{"source": "fixture"}}, nil
}

func (fixtureHandler) Uninstall(_ map[string]any, _ string, _ handler.Context) (handler.ExecResult, error) {
	return handler.ExecResult{}, nil
}

func TestServeDispatchesHandlerOverGRPC(t *testing.T) {
	client := pluginlib.NewClient(&pluginlib.ClientConfig{
		HandshakeConfig: HandshakeConfig(),
		VersionedPlugins: map[int]pluginlib.PluginSet{
			ProtocolVersion: ClientPluginSet(),
		},
		Cmd:              helperCommand(t),
		AllowedProtocols: []pluginlib.Protocol{pluginlib.ProtocolGRPC},
	})
	t.Cleanup(client.Kill)

	rpcClient, err := client.Client()
	if err != nil {
		t.Fatalf("connect to plugin: %v", err)
	}
	dispensed, err := rpcClient.Dispense(HandlerServiceName)
	if err != nil {
		t.Fatalf("dispense handler service: %v", err)
	}
	handlerClient, ok := dispensed.(pluginpb.HandlerPluginClient)
	if !ok {
		t.Fatalf("dispensed service has type %T, want HandlerPluginClient", dispensed)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	listed, err := handlerClient.ListHandlers(ctx, &pluginpb.ListHandlersRequest{})
	if err != nil {
		t.Fatalf("list handlers: %v", err)
	}
	if len(listed.GetHandlerNames()) != 1 || listed.GetHandlerNames()[0] != "fixture" {
		t.Fatalf("handler names = %v, want [fixture]", listed.GetHandlerNames())
	}
	if listed.GetHandlerEmojis()["fixture"] != "fixture" {
		t.Fatalf("handler emoji = %q, want fixture", listed.GetHandlerEmojis()["fixture"])
	}

	item, err := structpb.NewStruct(map[string]any{"enabled": true})
	if err != nil {
		t.Fatalf("create request item: %v", err)
	}
	request := &pluginpb.HandlerRequest{HandlerName: "fixture", Item: item, Name: "test"}
	tested, err := handlerClient.Test(ctx, request)
	if err != nil {
		t.Fatalf("test handler: %v", err)
	}
	if !tested.GetSatisfied() {
		t.Fatal("handler Test returned false, want true")
	}

	installed, err := handlerClient.Install(ctx, request)
	if err != nil {
		t.Fatalf("install handler: %v", err)
	}
	if installed.GetRc() != 7 || installed.GetExtra().AsMap()["source"] != "fixture" {
		t.Fatalf("install result = %#v, want rc 7 and fixture extra", installed)
	}
}

func TestPluginHelperProcess(t *testing.T) {
	if os.Getenv(helperEnvironment) != "1" {
		return
	}
	Serve(map[string]handler.Handler{"fixture": fixtureHandler{}})
}

func helperCommand(t *testing.T) *exec.Cmd {
	t.Helper()
	command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestPluginHelperProcess$")
	command.Env = append(os.Environ(), helperEnvironment+"=1")
	return command
}
