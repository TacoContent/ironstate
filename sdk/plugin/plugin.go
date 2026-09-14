package plugin

// Package plugin serves external ironstate handlers over go-plugin gRPC.

import (
	"context"
	"sort"

	pluginlib "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/TacoContent/ironstate/sdk/handler"
	pluginpb "github.com/TacoContent/ironstate/sdk/proto"
)

const (
	// ProtocolVersion is the exact protocol version required by hosts and plugins.
	ProtocolVersion = 1
	// HandlerServiceName is the go-plugin service name for handler plugins.
	HandlerServiceName = "handler"
	// HostCallbackIDKey is internal request metadata used to route a handler
	// call back to the host's callback gRPC service.
	HostCallbackIDKey = "__ironstate_host_callback_id"
)

var handshakeConfig = pluginlib.HandshakeConfig{
	ProtocolVersion:  ProtocolVersion,
	MagicCookieKey:   "IRONSTATE_PLUGIN",
	MagicCookieValue: "handler",
}

// HandshakeConfig returns the shared go-plugin handshake configuration.
func HandshakeConfig() pluginlib.HandshakeConfig {
	return handshakeConfig
}

// ClientPluginSet returns the plugin registration a host needs to dispense a
// HandlerPluginClient from an external handler binary.
func ClientPluginSet() pluginlib.PluginSet {
	return pluginlib.PluginSet{HandlerServiceName: grpcPlugin{}}
}

// BrokeredHandlerClient exposes the callback broker used by the host to
// serve HandlerHostCallback for an individual handler call.
type BrokeredHandlerClient interface {
	pluginpb.HandlerPluginClient
	CallbackBroker() *pluginlib.GRPCBroker
}

// Serve starts an external handler plugin. It does not return.
func Serve(handlers map[string]handler.Handler) {
	pluginlib.Serve(&pluginlib.ServeConfig{
		HandshakeConfig: handshakeConfig,
		VersionedPlugins: map[int]pluginlib.PluginSet{
			ProtocolVersion: {
				HandlerServiceName: grpcPlugin{handlers: handlers},
			},
		},
		GRPCServer: pluginlib.DefaultGRPCServer,
	})
}

type grpcPlugin struct {
	pluginlib.NetRPCUnsupportedPlugin
	handlers map[string]handler.Handler
}

func (p grpcPlugin) GRPCServer(broker *pluginlib.GRPCBroker, server *grpc.Server) error {
	pluginpb.RegisterHandlerPluginServer(server, handlerServer{handlers: p.handlers, broker: broker})
	return nil
}

func (grpcPlugin) GRPCClient(_ context.Context, broker *pluginlib.GRPCBroker, connection *grpc.ClientConn) (any, error) {
	return brokeredHandlerClient{HandlerPluginClient: pluginpb.NewHandlerPluginClient(connection), broker: broker}, nil
}

type brokeredHandlerClient struct {
	pluginpb.HandlerPluginClient
	broker *pluginlib.GRPCBroker
}

func (c brokeredHandlerClient) CallbackBroker() *pluginlib.GRPCBroker { return c.broker }

type handlerServer struct {
	pluginpb.UnimplementedHandlerPluginServer
	handlers map[string]handler.Handler
	broker   *pluginlib.GRPCBroker
}

func (s handlerServer) ListHandlers(_ context.Context, _ *pluginpb.ListHandlersRequest) (*pluginpb.ListHandlersResponse, error) {
	names := make([]string, 0, len(s.handlers))
	emojis := make(map[string]string)
	for name, registered := range s.handlers {
		names = append(names, name)
		if provider, ok := registered.(handler.EmojiProvider); ok && provider.Emoji() != "" {
			emojis[name] = provider.Emoji()
		}
	}
	sort.Strings(names)
	return &pluginpb.ListHandlersResponse{HandlerNames: names, HandlerEmojis: emojis}, nil
}

func (s handlerServer) Test(_ context.Context, request *pluginpb.HandlerRequest) (*pluginpb.TestResponse, error) {
	h, item, name, handlerContext, err := s.requestValues(request)
	if err != nil {
		return nil, err
	}
	satisfied, err := h.Test(item, name, handlerContext)
	if err != nil {
		return nil, handlerError(err)
	}
	return &pluginpb.TestResponse{Satisfied: satisfied}, nil
}

func (s handlerServer) Describe(_ context.Context, request *pluginpb.DescribeRequest) (*pluginpb.DescribeResponse, error) {
	h, item, _, handlerContext, err := s.requestValues(request.GetHandler())
	if err != nil {
		return nil, err
	}
	description, err := h.Describe(item, handler.Action(request.GetAction()), handlerContext)
	if err != nil {
		return nil, handlerError(err)
	}
	return &pluginpb.DescribeResponse{Description: description}, nil
}

func (s handlerServer) Install(_ context.Context, request *pluginpb.HandlerRequest) (*pluginpb.ExecResult, error) {
	h, item, name, handlerContext, err := s.requestValues(request)
	if err != nil {
		return nil, err
	}
	result, err := h.Install(item, name, handlerContext)
	if err != nil {
		return nil, handlerError(err)
	}
	return execResult(result)
}

func (s handlerServer) Uninstall(_ context.Context, request *pluginpb.HandlerRequest) (*pluginpb.ExecResult, error) {
	h, item, name, handlerContext, err := s.requestValues(request)
	if err != nil {
		return nil, err
	}
	result, err := h.Uninstall(item, name, handlerContext)
	if err != nil {
		return nil, handlerError(err)
	}
	return execResult(result)
}

func (s handlerServer) FactName(_ context.Context, request *pluginpb.FactNameRequest) (*pluginpb.FactNameResponse, error) {
	h, err := s.handler(request.GetHandlerName())
	if err != nil {
		return nil, err
	}
	producer, ok := h.(handler.FactProducer)
	if !ok {
		return &pluginpb.FactNameResponse{}, nil
	}
	name, produced := producer.FactName(structMap(request.GetItem()))
	return &pluginpb.FactNameResponse{Name: name, Produced: produced}, nil
}

func (s handlerServer) ScanRole(_ context.Context, request *pluginpb.ScanRoleRequest) (*pluginpb.ScanRoleResponse, error) {
	h, err := s.handler(request.GetHandlerName())
	if err != nil {
		return nil, err
	}
	scanner, ok := h.(handler.ScanCapable)
	if !ok {
		return &pluginpb.ScanRoleResponse{}, nil
	}
	return &pluginpb.ScanRoleResponse{Role: scanner.ScanRole(), Supported: true}, nil
}

func (s handlerServer) Scan(_ context.Context, request *pluginpb.ScanRequest) (*pluginpb.ScanResponse, error) {
	h, err := s.handler(request.GetHandlerName())
	if err != nil {
		return nil, err
	}
	scanner, ok := h.(handler.ScanCapable)
	if !ok {
		return nil, status.Error(codes.Unimplemented, "handler does not support scanning")
	}
	items, err := scanner.Scan(contextValue(request.GetContext(), s.broker))
	if err != nil {
		return nil, handlerError(err)
	}
	response := &pluginpb.ScanResponse{Items: make([]*pluginpb.ScanItem, 0, len(items))}
	for _, item := range items {
		config, err := structpb.NewStruct(item.Config)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "scan item %q config: %v", item.Name, err)
		}
		response.Items = append(response.Items, &pluginpb.ScanItem{Module: item.Module, Name: item.Name, Config: config, Tags: item.Tags})
	}
	return response, nil
}

func (s handlerServer) requestValues(request *pluginpb.HandlerRequest) (handler.Handler, map[string]any, string, handler.Context, error) {
	h, err := s.handler(request.GetHandlerName())
	if err != nil {
		return nil, nil, "", handler.Context{}, err
	}
	return h, structMap(request.GetItem()), request.GetName(), contextValue(request.GetContext(), s.broker), nil
}

func (s handlerServer) handler(name string) (handler.Handler, error) {
	h, ok := s.handlers[name]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "handler %q is not provided by this plugin", name)
	}
	return h, nil
}

func contextValue(value *pluginpb.Context, broker *pluginlib.GRPCBroker) handler.Context {
	if value == nil {
		return handler.Context{Flat: map[string]any{}}
	}
	flat := structMap(value.GetFlat())
	callbackID := callbackID(flat)
	delete(flat, HostCallbackIDKey)
	return handler.Context{
		Flat:      flat,
		Apply:     value.GetApply(),
		Become:    handler.Become{Enabled: value.GetBecome().GetEnabled(), User: value.GetBecome().GetUser()},
		Callbacks: newHostCallbacks(broker, callbackID),
	}
}

func callbackID(flat map[string]any) uint32 {
	switch value := flat[HostCallbackIDKey].(type) {
	case float64:
		if value > 0 && value <= float64(^uint32(0)) {
			return uint32(value)
		}
	case int:
		if value > 0 {
			return uint32(value)
		}
	}
	return 0
}

func newHostCallbacks(broker *pluginlib.GRPCBroker, id uint32) handler.HostCallbacks {
	if broker == nil || id == 0 {
		return nil
	}
	return hostCallbacks{broker: broker, id: id}
}

type hostCallbacks struct {
	broker *pluginlib.GRPCBroker
	id     uint32
}

func (c hostCallbacks) withClient(fn func(pluginpb.HandlerHostCallbackClient) error) error {
	connection, err := c.broker.Dial(c.id)
	if err != nil {
		return err
	}
	defer connection.Close()
	return fn(pluginpb.NewHandlerHostCallbackClient(connection))
}

func (c hostCallbacks) RenderTemplate(value string, variables map[string]any) (string, error) {
	var rendered string
	err := c.withClient(func(client pluginpb.HandlerHostCallbackClient) error {
		response, err := client.RenderTemplate(context.Background(), &pluginpb.RenderTemplateRequest{Template: value, Variables: mustStruct(variables)})
		if err == nil {
			rendered = response.GetRendered()
		}
		return err
	})
	return rendered, err
}

func (c hostCallbacks) EvaluateCondition(expression string, variables map[string]any) (bool, error) {
	var result bool
	err := c.withClient(func(client pluginpb.HandlerHostCallbackClient) error {
		response, err := client.EvaluateCondition(context.Background(), &pluginpb.EvaluateConditionRequest{Expression: expression, Variables: mustStruct(variables)})
		if err == nil {
			result = response.GetResult()
		}
		return err
	})
	return result, err
}

func structMap(value *structpb.Struct) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value.AsMap()
}

func mustStruct(value map[string]any) *structpb.Struct {
	converted, err := structpb.NewStruct(value)
	if err != nil {
		return &structpb.Struct{}
	}
	return converted
}

func execResult(value handler.ExecResult) (*pluginpb.ExecResult, error) {
	extra, err := structpb.NewStruct(value.Extra)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "result extra: %v", err)
	}
	return &pluginpb.ExecResult{
		Rc:          int32(value.RC),
		Stdout:      value.Stdout,
		StdoutLines: value.StdoutLines,
		Stderr:      value.Stderr,
		StderrLines: value.StderrLines,
		Extra:       extra,
	}, nil
}

func handlerError(err error) error {
	return status.Error(codes.Internal, err.Error())
}
