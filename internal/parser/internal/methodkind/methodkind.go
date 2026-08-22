// Package methodkind classifies handler method names into
// HTTP method kinds (GET, POST, PUT, PATCH, DELETE) or event handlers.
package methodkind

import "strings"

// Kind is what a method name makes of a method: an HTTP handler, a stream hook
// or an event handler. The zero value is an ordinary method.
type Kind int8

const (
	_ Kind = iota
	GETHandler
	ActionPOSTHandler
	ActionPUTHandler
	ActionPATCHHandler
	ActionDELETEHandler
	StreamOpenHook
	StreamCloseHook
	EventHandler
)

// IsAction reports whether the kind is an action
// (POST, PUT, PATCH, or DELETE).
func (k Kind) IsAction() bool {
	switch k {
	case ActionPOSTHandler,
		ActionPUTHandler,
		ActionPATCHHandler,
		ActionDELETEHandler:
		return true
	}
	return false
}

// HTTPMethod returns the HTTP method string for the kind.
func (k Kind) HTTPMethod() string {
	switch k {
	case GETHandler:
		return "GET"
	case ActionPOSTHandler:
		return "POST"
	case ActionPUTHandler:
		return "PUT"
	case ActionPATCHHandler:
		return "PATCH"
	case ActionDELETEHandler:
		return "DELETE"
	}
	return ""
}

// Classify reads the kind and the name suffix out of a method name.
// An unrecognized name yields the zero Kind.
func Classify(name string) (kind Kind, suffix string) {
	if name == "" {
		return 0, ""
	}
	// Only treat exported identifiers as framework-reserved
	// handlers. This makes pOST / postX / onFoo etc. normal
	// methods.
	if name[0] < 'A' || name[0] > 'Z' {
		return 0, ""
	}

	switch {
	case name == "GET":
		return GETHandler, ""
	case strings.HasPrefix(name, "POST"):
		return ActionPOSTHandler, name[len("POST"):]
	case strings.HasPrefix(name, "PUT"):
		return ActionPUTHandler, name[len("PUT"):]
	case strings.HasPrefix(name, "PATCH"):
		return ActionPATCHHandler, name[len("PATCH"):]
	case strings.HasPrefix(name, "DELETE"):
		return ActionDELETEHandler, name[len("DELETE"):]
	case name == "StreamOpen":
		return StreamOpenHook, ""
	case name == "StreamClose":
		return StreamCloseHook, ""
	case strings.HasPrefix(name, "On"):
		return EventHandler, name[len("On"):]
	default:
		return 0, ""
	}
}
