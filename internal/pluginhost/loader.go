package pluginhost

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/hashicorp/go-hclog"
	pluginlib "github.com/hashicorp/go-plugin"

	"github.com/TacoContent/ironstate/internal/engine"
	"github.com/TacoContent/ironstate/internal/expr"
	sdkplugin "github.com/TacoContent/ironstate/sdk/plugin"
	pluginpb "github.com/TacoContent/ironstate/sdk/proto"
)

// Client owns one external plugin process and its discovered handlers and
// filters.
type Client struct {
	client   *pluginlib.Client
	remote   pluginpb.HandlerPluginClient
	handlers map[string]engine.Handler
	filters  []string
}

// FilterFunc is one plugin-provided filter, callable like a built-in.
type FilterFunc func(value any, args []any) (any, error)

var filterNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

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
		Logger:           hclog.NewNullLogger(),
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
		metadata := listed.GetHandlerMetadata()[name]
		emoji := listed.GetHandlerEmojis()[name]
		var requiredTools []string
		factProducer := false
		if metadata != nil {
			if metadata.GetEmoji() != "" {
				emoji = metadata.GetEmoji()
			}
			requiredTools = metadata.GetRequiredTools()
			factProducer = metadata.GetFactProducer()
		}
		handlers[name] = newAdapter(handlerAdapter{client: remote, name: name, emoji: emoji, requiredTools: requiredTools, callbacks: callbacks}, factProducer)
	}
	for _, name := range listed.GetFilterNames() {
		if !filterNamePattern.MatchString(name) {
			client.Kill()
			return nil, fmt.Errorf("plugin declared invalid filter name %q; use letters, digits, and underscores", name)
		}
	}
	return &Client{client: client, remote: remote, handlers: handlers, filters: append([]string(nil), listed.GetFilterNames()...)}, nil
}

// FilterNames returns the plugin's declared filter names, unqualified.
func (c *Client) FilterNames() []string {
	if c == nil {
		return nil
	}
	return append([]string(nil), c.filters...)
}

// Filters returns the plugin's filters by unqualified name.
func (c *Client) Filters() map[string]FilterFunc {
	if c == nil {
		return nil
	}
	result := make(map[string]FilterFunc, len(c.filters))
	for _, name := range c.filters {
		filterName := name
		result[filterName] = func(value any, args []any) (any, error) {
			return callFilter(c.remote, filterName, value, args)
		}
	}
	return result
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
