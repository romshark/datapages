package parser

import (
	"datapages/parser/model"
	"go/types"
)

// Plugin defines the interface for parser plugins that can extend
// handler inputs and outputs at various hook points during parsing.
type Plugin interface {
	// OnGETInputs is called after a GET handler is parsed.
	// It can return additional inputs to append to the handler.
	OnGETInputs(ctx *PluginContext) []*model.Input

	// OnGETOutputs is called after a GET handler is parsed.
	// It can return additional outputs to append to the handler.
	OnGETOutputs(ctx *PluginContext) []*model.Output

	// OnActionInputs is called after an action handler (POST/PUT/DELETE) is parsed.
	// It can return additional inputs to append to the handler.
	OnActionInputs(ctx *PluginContext) []*model.Input

	// OnActionOutputs is called after an action handler (POST/PUT/DELETE) is parsed.
	// It can return additional outputs to append to the handler.
	OnActionOutputs(ctx *PluginContext) []*model.Output

	// OnEventHandlerInputs is called after an event handler is parsed.
	// It can return additional inputs to append to the event handler.
	OnEventHandlerInputs(ctx *EventHandlerPluginContext) []*model.Input
}

// PluginContext provides context to plugins when processing HTTP handlers.
type PluginContext struct {
	// Page is the page type being processed (may be nil for abstract pages).
	Page *model.Page

	// Abstract is the abstract page type being processed (may be nil for pages).
	Abstract *model.AbstractPage

	// Handler is the handler being processed.
	Handler *model.Handler

	// ReceiverTypeName is the name of the receiver type.
	ReceiverTypeName string

	// TypesInfo provides type information from the Go type checker.
	TypesInfo *types.Info
}

// EventHandlerPluginContext provides context to plugins when processing event handlers.
type EventHandlerPluginContext struct {
	// Page is the page type being processed (may be nil for abstract pages).
	Page *model.Page

	// Abstract is the abstract page type being processed (may be nil for pages).
	Abstract *model.AbstractPage

	// EventHandler is the event handler being processed.
	EventHandler *model.EventHandler

	// ReceiverTypeName is the name of the receiver type.
	ReceiverTypeName string

	// TypesInfo provides type information from the Go type checker.
	TypesInfo *types.Info
}

// BasePlugin provides a default implementation of the Plugin interface
// with no-op methods. Embed this in your plugin to only implement
// the hooks you need.
type BasePlugin struct{}

func (BasePlugin) OnGETInputs(ctx *PluginContext) []*model.Input                     { return nil }
func (BasePlugin) OnGETOutputs(ctx *PluginContext) []*model.Output                   { return nil }
func (BasePlugin) OnActionInputs(ctx *PluginContext) []*model.Input                  { return nil }
func (BasePlugin) OnActionOutputs(ctx *PluginContext) []*model.Output                { return nil }
func (BasePlugin) OnEventHandlerInputs(ctx *EventHandlerPluginContext) []*model.Input { return nil }
