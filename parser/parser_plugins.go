package parser

import (
	"datapages/parser/model"
	"go/types"
)

// applyHandlerPlugins invokes all registered plugins for an HTTP handler.
func (p *Parser) applyHandlerPlugins(
	h *model.Handler,
	pg *model.Page,
	ap *model.AbstractPage,
	recv string,
	info *types.Info,
) {
	if len(p.plugins) == 0 {
		return
	}

	ctx := &PluginContext{
		Page:             pg,
		Abstract:         ap,
		Handler:          h,
		ReceiverTypeName: recv,
		TypesInfo:        info,
	}

	for _, plugin := range p.plugins {
		switch h.HTTPMethod {
		case "GET":
			if inputs := plugin.OnGETInputs(ctx); len(inputs) > 0 {
				h.Inputs = append(h.Inputs, inputs...)
			}
			if outputs := plugin.OnGETOutputs(ctx); len(outputs) > 0 {
				h.Outputs = append(h.Outputs, outputs...)
			}
		case "POST", "PUT", "DELETE":
			if inputs := plugin.OnActionInputs(ctx); len(inputs) > 0 {
				h.Inputs = append(h.Inputs, inputs...)
			}
			if outputs := plugin.OnActionOutputs(ctx); len(outputs) > 0 {
				h.Outputs = append(h.Outputs, outputs...)
			}
		}
	}
}

// applyEventHandlerPlugins invokes all registered plugins for an event handler.
func (p *Parser) applyEventHandlerPlugins(
	h *model.EventHandler,
	pg *model.Page,
	ap *model.AbstractPage,
	recv string,
	info *types.Info,
) {
	if len(p.plugins) == 0 {
		return
	}

	ctx := &EventHandlerPluginContext{
		Page:             pg,
		Abstract:         ap,
		EventHandler:     h,
		ReceiverTypeName: recv,
		TypesInfo:        info,
	}

	for _, plugin := range p.plugins {
		if inputs := plugin.OnEventHandlerInputs(ctx); len(inputs) > 0 {
			h.Inputs = append(h.Inputs, inputs...)
		}
	}
}
