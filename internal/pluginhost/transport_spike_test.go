package pluginhost

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	plugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

const (
	pluginCookieKey   = "IRONSTATE_PLUGIN"
	pluginCookieValue = "handler"
	pluginProtocolV1  = 1
)

var handshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion:  pluginProtocolV1,
	MagicCookieKey:   pluginCookieKey,
	MagicCookieValue: pluginCookieValue,
}

type healthPlugin struct {
	plugin.NetRPCUnsupportedPlugin
}

func (healthPlugin) GRPCServer(_ *plugin.GRPCBroker, server *grpc.Server) error {
	return nil
}

func (healthPlugin) GRPCClient(_ context.Context, _ *plugin.GRPCBroker, connection *grpc.ClientConn) (any, error) {
	return healthpb.NewHealthClient(connection), nil
}

func TestGRPCTransportRoundTrip(t *testing.T) {
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: handshakeConfig,
		VersionedPlugins: map[int]plugin.PluginSet{
			pluginProtocolV1: {
				"health": healthPlugin{},
			},
		},
		Cmd:              helperCommand(t),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
	})
	t.Cleanup(client.Kill)

	rpcClient, err := client.Client()
	if err != nil {
		t.Fatalf("connect to plugin: %v", err)
	}

	dispensed, err := rpcClient.Dispense("health")
	if err != nil {
		t.Fatalf("dispense health service: %v", err)
	}
	healthClient, ok := dispensed.(healthpb.HealthClient)
	if !ok {
		t.Fatalf("dispensed service has type %T, want health client", dispensed)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := healthClient.Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("check plugin health: %v", err)
	}
	if response.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("health status = %s, want %s", response.GetStatus(), healthpb.HealthCheckResponse_SERVING)
	}
}

func TestPluginHelperProcess(t *testing.T) {
	if os.Getenv("IRONSTATE_PLUGIN_HELPER") != "1" {
		return
	}

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: handshakeConfig,
		VersionedPlugins: map[int]plugin.PluginSet{
			pluginProtocolV1: {
				"health": healthPlugin{},
			},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}

func helperCommand(t *testing.T) *exec.Cmd {
	t.Helper()
	command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestPluginHelperProcess$") //nolint:gosec // test-only current binary with a fixed test selector
	command.Env = append(os.Environ(), "IRONSTATE_PLUGIN_HELPER=1")
	return command
}
