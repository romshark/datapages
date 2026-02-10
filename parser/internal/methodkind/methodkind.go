// Package methodkind classifies handler method names into
// HTTP method kinds (GET, POST, PUT, DELETE) or event handlers.
package methodkind

import "strings"

// Kind represents the kind of handler method.
type Kind int8

const (
	_ Kind = iota
	GETHandler
	ActionPOSTHandler
	ActionPUTHandler
	ActionDELETEHandler
	EventHandler
)

// IsAction reports whether the kind is an action
// (POST, PUT, or DELETE).
func (k Kind) IsAction() bool {
	switch k {
	case ActionPOSTHandler,
		ActionPUTHandler,
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
	case ActionDELETEHandler:
		return "DELETE"
	}
	return ""
}

// Classify determines the handler kind and name suffix from
// a method name. Returns zero Kind for unrecognized names.
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
	case strings.HasPrefix(name, "DELETE"):
		return ActionDELETEHandler, name[len("DELETE"):]
	case strings.HasPrefix(name, "On"):
		return EventHandler, name[len("On"):]
	default:
		return 0, ""
	}
}
