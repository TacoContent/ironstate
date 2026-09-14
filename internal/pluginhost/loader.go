package pluginhost

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/hashicorp/go-hclog"
	pluginlib "github.com/hashicorp/go-plugin"

	"github.com/TacoContent/ironstate/internal/engine"
	"github.com/TacoContent/ironstate/internal/expr"
	sdkplugin "github.com/TacoContent/ironstate/sdk/plugin"
	pluginpb "github.com/TacoContent/ironstate/sdk/proto"
)

// Client owns one external plugin process and its discovered handlers.
type Client struct {
	client   *pluginlib.Client
	handlers map[string]engine.Handler
}

// Launch starts command, verifies its plugin handshake, and discovers its
// declared handlers. The returned client must be closed when the run ends.
func Launch(command *exec.Cmd) (*Client, error) {
	return launch(command, nil)
}

// LaunchWithCallbacks starts a plugin and makes the host's expression
// services available during handler calls.
func LaunchWithCallbacks(command *exec.Cmd, filters expr.Filters) (*Client, error) {
	return launch(command, filters)
}

func launch(command *exec.Cmd, filters expr.Filters) (*Client, error) {
	if command == nil {
		return nil, fmt.Errorf("plugin command is required")
	}
	client := pluginlib.NewClient(&pluginlib.ClientConfig{
		HandshakeConfig: sdkplugin.HandshakeConfig(),
		VersionedPlugins: map[int]pluginlib.PluginSet{
			sdkplugin.ProtocolVersion: sdkplugin.ClientPluginSet(),
		},
		Cmd:              command,
		AllowedProtocols: []pluginlib.Protocol{pluginlib.ProtocolGRPC},
		Logger: hclog.New(&hclog.LoggerOptions{
			Name:   "plugin",
			Level:  hclog.Trace,
			Output: os.Stderr,
		}),
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("connect to plugin: %w", err)
	}
	dispensed, err := rpcClient.Dispense(sdkplugin.HandlerServiceName)
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("dispense plugin handler service: %w", err)
	}
	remote, ok := dispensed.(pluginpb.HandlerPluginClient)
	if !ok {
		client.Kill()
		return nil, fmt.Errorf("plugin handler service has type %T", dispensed)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	listed, err := remote.ListHandlers(ctx, &pluginpb.ListHandlersRequest{})
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("list plugin handlers: %w", err)
	}
	var callbacks *callbackBroker
	if filters != nil {
		brokered, ok := dispensed.(sdkplugin.BrokeredHandlerClient)
		if !ok {
			client.Kill()
			return nil, fmt.Errorf("plugin handler service does not support host callbacks")
		}
		callbacks = &callbackBroker{broker: brokered.CallbackBroker(), filters: filters}
	}
	handlers := make(map[string]engine.Handler, len(listed.GetHandlerNames()))
	for _, name := range listed.GetHandlerNames() {
		if name == "" {
			client.Kill()
			return nil, fmt.Errorf("plugin declared an empty handler name")
		}
		if callbacks == nil {
			handlers[name] = handlerAdapter{client: remote, name: name, emoji: listed.GetHandlerEmojis()[name]}
		} else {
			handlers[name] = handlerAdapter{client: remote, name: name, emoji: listed.GetHandlerEmojis()[name], callbacks: callbacks}
		}
	}
	return &Client{client: client, handlers: handlers}, nil
}

// Handlers returns the plugin's discovered handlers by unqualified name.
func (c *Client) Handlers() map[string]engine.Handler {
	if c == nil {
		return nil
	}
	result := make(map[string]engine.Handler, len(c.handlers))
	for name, handler := range c.handlers {
		result[name] = handler
	}
	return result
}

// QualifiedHandlers returns handlers addressed as
// <organization>.<plugin>.<handler>, ready to merge into handlers.Registry.
func (c *Client) QualifiedHandlers(namespace string) (map[string]engine.Handler, error) {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" || strings.HasPrefix(namespace, ".") || strings.HasSuffix(namespace, ".") || strings.Count(namespace, ".") != 1 {
		return nil, fmt.Errorf("plugin namespace must use organization.plugin form")
	}
	result := make(map[string]engine.Handler, len(c.handlers))
	for name, handler := range c.handlers {
		result[namespace+"."+name] = handler
	}
	return result, nil
}

// Close terminates the external plugin process.
func (c *Client) Close() error {
	if c != nil && c.client != nil {
		c.client.Kill()
	}
	return nil
}
