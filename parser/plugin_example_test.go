package parser_test

import (
	"datapages/parser"
	"datapages/parser/model"
	"go/ast"
	"go/types"
)

// ExamplePlugin demonstrates how to create a plugin that adds custom inputs and outputs
// to handlers at various hook points.
type ExamplePlugin struct {
	parser.BasePlugin // Embed BasePlugin to only implement needed hooks
}

// OnGETInputs adds a custom "logger" input to all GET handlers.
func (p *ExamplePlugin) OnGETInputs(ctx *parser.PluginContext) []*model.Input {
	// Create a custom input (you would typically use ctx.TypesInfo to resolve the actual type)
	return []*model.Input{
		{
			Name: "logger",
			Type: model.Type{
				// In a real plugin, you'd resolve the actual Logger type from TypesInfo
				Resolved: types.NewPointer(types.NewNamed(
					types.NewTypeName(0, nil, "Logger", nil),
					types.NewStruct(nil, nil),
					nil,
				)),
			},
		},
	}
}

// OnGETOutputs adds a custom "metrics" output to all GET handlers.
func (p *ExamplePlugin) OnGETOutputs(ctx *parser.PluginContext) []*model.Output {
	return []*model.Output{
		{
			Name: "metrics",
			Type: model.Type{
				Resolved: types.NewPointer(types.NewNamed(
					types.NewTypeName(0, nil, "Metrics", nil),
					types.NewStruct(nil, nil),
					nil,
				)),
			},
		},
	}
}

// OnActionInputs adds a "requestID" input to all action handlers (POST/PUT/DELETE).
func (p *ExamplePlugin) OnActionInputs(ctx *parser.PluginContext) []*model.Input {
	// You can inspect ctx.Handler to make decisions
	// You can inspect ctx.Page or ctx.Abstract to understand the context
	return []*model.Input{
		{
			Name: "requestID",
			Type: model.Type{
				Resolved: types.Typ[types.String],
			},
		},
	}
}

// OnActionOutputs adds an "auditLog" output to POST actions only.
func (p *ExamplePlugin) OnActionOutputs(ctx *parser.PluginContext) []*model.Output {
	// Only add to POST handlers
	if ctx.Handler.HTTPMethod != "POST" {
		return nil
	}

	return []*model.Output{
		{
			Name: "auditLog",
			Type: model.Type{
				Resolved: types.NewPointer(types.NewNamed(
					types.NewTypeName(0, nil, "AuditLog", nil),
					types.NewStruct(nil, nil),
					nil,
				)),
			},
		},
	}
}

// OnEventHandlerInputs adds a "context" input to all event handlers.
func (p *ExamplePlugin) OnEventHandlerInputs(ctx *parser.EventHandlerPluginContext) []*model.Input {
	return []*model.Input{
		{
			Name: "ctx",
			Type: model.Type{
				TypeExpr: &ast.Ident{Name: "context.Context"},
			},
		},
	}
}

// Example usage:
//
// func main() {
//     plugin1 := &ExamplePlugin{}
//     plugin2 := &AnotherPlugin{}
//
//     p := parser.New(plugin1, plugin2)
//     app, errs := p.Parse("./myapp")
//
//     // The parsed handlers will now have the additional inputs/outputs
//     // added by the plugins
// }
