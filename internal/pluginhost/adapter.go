// Package pluginhost adapts external handler plugin RPC clients to engine handlers.
package pluginhost

import (
	"context"
	"fmt"
	"time"

	pluginlib "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/TacoContent/ironstate/internal/engine"
	"github.com/TacoContent/ironstate/internal/expr"
	sdkplugin "github.com/TacoContent/ironstate/sdk/plugin"
	pluginpb "github.com/TacoContent/ironstate/sdk/proto"
)

const (
	testTimeout      = 30 * time.Second
	operationTimeout = 5 * time.Minute
)

// NewHandler adapts a named remote plugin handler to engine.Handler.
func NewHandler(client pluginpb.HandlerPluginClient, name string) engine.Handler {
	return handlerAdapter{client: client, name: name}
}

type handlerAdapter struct {
	client        pluginpb.HandlerPluginClient
	name          string
	emoji         string
	requiredTools []string
	callbacks     *callbackBroker
}

func (h handlerAdapter) Emoji() string { return h.emoji }

func (h handlerAdapter) RequiredTools() []string { return append([]string(nil), h.requiredTools...) }

func (h handlerAdapter) ScanRole() string {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	response, err := h.client.ScanRole(ctx, &pluginpb.ScanRoleRequest{HandlerName: h.name})
	if err != nil || !response.GetSupported() {
		return ""
	}
	return response.GetRole()
}

func (h handlerAdapter) Scan(ctx engine.Context) ([]engine.ScanItem, error) {
	prepared, cleanup, err := h.prepare(ctx)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	rpcCtx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()
	response, err := h.client.Scan(rpcCtx, &pluginpb.ScanRequest{
		HandlerName: h.name,
		Context: &pluginpb.Context{Flat: mustStruct(prepared.Flat), Apply: prepared.Apply, Become: &pluginpb.Become{
			Enabled: prepared.Become.Enabled,
			User:    prepared.Become.User,
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("plugin handler %q scan: %w", h.name, err)
	}
	items := make([]engine.ScanItem, 0, len(response.GetItems()))
	for _, item := range response.GetItems() {
		items = append(items, engine.ScanItem{Module: item.GetModule(), Name: item.GetName(), Config: structMap(item.GetConfig()), Tags: item.GetTags()})
	}
	return items, nil
}

func (h handlerAdapter) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	ctx, cleanup, err := h.prepare(ctx)
	if err != nil {
		return false, err
	}
	defer cleanup()
	rpcCtx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	response, err := h.client.Test(rpcCtx, h.request(item, name, ctx))
	if err != nil {
		return false, fmt.Errorf("plugin handler %q test: %w", h.name, err)
	}
	return response.GetSatisfied(), nil
}

func (h handlerAdapter) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	ctx, cleanup, err := h.prepare(ctx)
	if err != nil {
		return "", err
	}
	defer cleanup()
	rpcCtx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	response, err := h.client.Describe(rpcCtx, &pluginpb.DescribeRequest{
		Handler: h.request(item, "", ctx),
		Action:  string(action),
	})
	if err != nil {
		return "", fmt.Errorf("plugin handler %q describe: %w", h.name, err)
	}
	return response.GetDescription(), nil
}

func (h handlerAdapter) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return h.run(item, name, ctx, h.client.Install)
}

func (h handlerAdapter) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return h.run(item, name, ctx, h.client.Uninstall)
}

func (h handlerAdapter) run(item map[string]any, name string, ctx engine.Context, handlerCall func(context.Context, *pluginpb.HandlerRequest, ...grpc.CallOption) (*pluginpb.ExecResult, error)) (engine.ExecResult, error) {
	ctx, cleanup, err := h.prepare(ctx)
	if err != nil {
		return engine.ExecResult{}, err
	}
	defer cleanup()
	rpcCtx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()
	response, err := handlerCall(rpcCtx, h.request(item, name, ctx))
	if err != nil {
		return engine.ExecResult{}, fmt.Errorf("plugin handler %q: %w", h.name, err)
	}
	return engine.ExecResult{
		RC:          int(response.GetRc()),
		Stdout:      response.GetStdout(),
		StdoutLines: response.GetStdoutLines(),
		Stderr:      response.GetStderr(),
		StderrLines: response.GetStderrLines(),
		Extra:       structMap(response.GetExtra()),
	}, nil
}

func (h handlerAdapter) prepare(ctx engine.Context) (engine.Context, func(), error) {
	if h.callbacks == nil {
		return ctx, func() {}, nil
	}
	return h.callbacks.prepare(ctx)
}

func (h handlerAdapter) request(item map[string]any, name string, ctx engine.Context) *pluginpb.HandlerRequest {
	return &pluginpb.HandlerRequest{
		HandlerName: h.name,
		Item:        mustStruct(item),
		Name:        name,
		Context: &pluginpb.Context{
			Flat:  mustStruct(ctx.Flat),
			Apply: ctx.Apply,
			Become: &pluginpb.Become{
				Enabled: ctx.Become.Enabled,
				User:    ctx.Become.User,
			},
		},
	}
}

func mustStruct(value map[string]any) *structpb.Struct {
	converted, err := structpb.NewStruct(value)
	if err != nil {
		return &structpb.Struct{}
	}
	return converted
}

func structMap(value *structpb.Struct) map[string]any {
	if value == nil {
		return nil
	}
	return value.AsMap()
}

type callbackBroker struct {
	broker  *pluginlib.GRPCBroker
	filters expr.Filters
}

func (b *callbackBroker) prepare(ctx engine.Context) (engine.Context, func(), error) {
	if b == nil || b.broker == nil {
		return ctx, func() {}, nil
	}
	id := b.broker.NextId()
	listener, err := b.broker.Accept(id)
	if err != nil {
		return ctx, nil, fmt.Errorf("start host callback listener: %w", err)
	}
	server := grpc.NewServer()
	pluginpb.RegisterHandlerHostCallbackServer(server, hostCallbackServer{filters: b.filters})
	go func() { _ = server.Serve(listener) }()
	flat := make(map[string]any, len(ctx.Flat)+1)
	for key, value := range ctx.Flat {
		flat[key] = value
	}
	flat[sdkplugin.HostCallbackIDKey] = float64(id)
	ctx.Flat = flat
	cleanup := func() {
		server.Stop()
		_ = listener.Close()
	}
	return ctx, cleanup, nil
}
