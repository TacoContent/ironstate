package pluginhost

import (
	"context"

	"github.com/TacoContent/ironstate/internal/conditions"
	"github.com/TacoContent/ironstate/internal/expr"
	"github.com/TacoContent/ironstate/internal/templateengines"
	pluginpb "github.com/TacoContent/ironstate/sdk/proto"
)

type hostCallbackServer struct {
	pluginpb.UnimplementedHandlerHostCallbackServer
	filters expr.Filters
}

func (s hostCallbackServer) RenderTemplate(_ context.Context, request *pluginpb.RenderTemplateRequest) (*pluginpb.RenderTemplateResponse, error) {
	rendered, err := templateengines.RenderJinja(request.GetTemplate(), structMap(request.GetVariables()), s.filters)
	if err != nil {
		return nil, err
	}
	return &pluginpb.RenderTemplateResponse{Rendered: rendered}, nil
}

func (s hostCallbackServer) EvaluateCondition(_ context.Context, request *pluginpb.EvaluateConditionRequest) (*pluginpb.EvaluateConditionResponse, error) {
	result, err := conditions.TestCondition(request.GetExpression(), structMap(request.GetVariables()), s.filters)
	if err != nil {
		return nil, err
	}
	return &pluginpb.EvaluateConditionResponse{Result: result}, nil
}
