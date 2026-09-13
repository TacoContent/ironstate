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

func (p grpcPlugin) GRPCServer(_ *pluginlib.GRPCBroker, server *grpc.Server) error {
	pluginpb.RegisterHandlerPluginServer(server, handlerServer{handlers: p.handlers})
	return nil
}

func (grpcPlugin) GRPCClient(_ context.Context, _ *pluginlib.GRPCBroker, connection *grpc.ClientConn) (any, error) {
	return pluginpb.NewHandlerPluginClient(connection), nil
}

type handlerServer struct {
	pluginpb.UnimplementedHandlerPluginServer
	handlers map[string]handler.Handler
}

func (s handlerServer) ListHandlers(_ context.Context, _ *pluginpb.ListHandlersRequest) (*pluginpb.ListHandlersResponse, error) {
	names := make([]string, 0, len(s.handlers))
	for name := range s.handlers {
		names = append(names, name)
	}
	sort.Strings(names)
	return &pluginpb.ListHandlersResponse{HandlerNames: names}, nil
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
	items, err := scanner.Scan(contextValue(request.GetContext()))
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
	return h, structMap(request.GetItem()), request.GetName(), contextValue(request.GetContext()), nil
}

func (s handlerServer) handler(name string) (handler.Handler, error) {
	h, ok := s.handlers[name]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "handler %q is not provided by this plugin", name)
	}
	return h, nil
}

func contextValue(value *pluginpb.Context) handler.Context {
	if value == nil {
		return handler.Context{Flat: map[string]any{}}
	}
	return handler.Context{
		Flat:   structMap(value.GetFlat()),
		Apply:  value.GetApply(),
		Become: handler.Become{Enabled: value.GetBecome().GetEnabled(), User: value.GetBecome().GetUser()},
	}
}

func structMap(value *structpb.Struct) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value.AsMap()
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
