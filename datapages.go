package datapages

import (
	"context"
	"io"
)

// Component is the interface that all templates implement.
type Component interface {
	// Render renders the template to w.
	Render(ctx context.Context, w io.Writer) error
}

// SSE is the server-sent-event handle passed to action (POST/PUT/PATCH/DELETE)
// and event (OnXXX) handlers.
type SSE interface {
	// Context returns the context of the SSE stream.
	Context() context.Context

	// PatchElement patches the elements rendered by c into the DOM.
	// They are morphed by default, use [WithMode] to patch them differently.
	PatchElement(c Component, opts ...PatchOption) error

	// RemoveElement removes the elements matching the CSS selector from the DOM.
	RemoveElement(selector string) error

	// ExecuteScript runs a script on the client.
	ExecuteScript(script string) error

	// PatchSignals updates client-side signals from v, which is marshaled to JSON.
	// It overwrites the signals the client already has.
	// To send JSON as is, pass an [encoding/json.RawMessage]:
	//
	//	sse.PatchSignals(json.RawMessage(`{"count":42}`))
	PatchSignals(v any) error

	// PatchSignalsIfMissing works like [SSE.PatchSignals] but only sets the
	// signals the client doesn't have yet. The rest keep their values.
	PatchSignalsIfMissing(v any) error

	// Redirect navigates the client to url by assigning window.location.href,
	// which pushes a new browser history entry.
	// To replace the current entry instead, navigate with [SSE.ExecuteScript]:
	//
	//	sse.ExecuteScript(fmt.Sprintf("window.location.replace(%q)", url))
	Redirect(url string) error

	// Prefetch asks the browser to prefetch urls through the speculation rules API.
	// Browsers without support for it ignore the request.
	//
	// https://developer.mozilla.org/en-US/docs/Web/API/Speculation_Rules_API
	Prefetch(urls ...string) error
}

// PatchConfig is the accumulated configuration of a [SSE.PatchElement] call.
// The generated runtime translates it to the underlying Datastar options.
//
// Selector and SelectorID both name the patch target and are mutually exclusive.
// If both are set then SelectorID wins, no matter in which order the options were passed.
type PatchConfig struct {
	Selector   string
	SelectorID string
	Mode       PatchMode
}

// PatchMode determines how patched elements are applied to the DOM.
// Removal has no mode, use [SSE.RemoveElement] instead.
// Zero value is equivalent to [PatchModeOuter].
type PatchMode string

const (
	// PatchModeOuter (default) morphs the element into the existing element.
	PatchModeOuter PatchMode = "outer"

	// PatchModeInner replaces the inner HTML of the existing element.
	PatchModeInner PatchMode = "inner"

	// PatchModeReplace replaces the existing element with the new element.
	PatchModeReplace PatchMode = "replace"

	// PatchModePrepend prepends the element inside the existing element.
	PatchModePrepend PatchMode = "prepend"

	// PatchModeAppend appends the element inside the existing element.
	PatchModeAppend PatchMode = "append"

	// PatchModeBefore inserts the element before the existing element.
	PatchModeBefore PatchMode = "before"

	// PatchModeAfter inserts the element after the existing element.
	PatchModeAfter PatchMode = "after"
)

// PatchOption configures [SSE.PatchElement].
type PatchOption func(*PatchConfig)

// WithSelector targets the element(s) matching a CSS selector.
// Mutually exclusive with [WithSelectorID] (which wins if both are given).
func WithSelector(selector string) PatchOption {
	return func(c *PatchConfig) { c.Selector = selector }
}

// WithSelectorID targets the element with the given id.
// Mutually exclusive with [WithSelector] and wins if both are given.
func WithSelectorID(id string) PatchOption {
	return func(c *PatchConfig) { c.SelectorID = id }
}

// WithMode determines how the patched elements are applied to the DOM.
// Defaults to [PatchModeOuter], which morphs.
// mode must be one of the [PatchMode] constants, any other value is ignored:
//
//   - [PatchModeOuter]
//   - [PatchModeInner]
//   - [PatchModeReplace]
//   - [PatchModePrepend]
//   - [PatchModeAppend]
//   - [PatchModeBefore]
//   - [PatchModeAfter]
func WithMode(mode PatchMode) PatchOption {
	return func(c *PatchConfig) { c.Mode = mode }
}
