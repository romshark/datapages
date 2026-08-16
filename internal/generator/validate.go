package generator

import (
	"fmt"

	"github.com/romshark/datapages/internal/parser/model"
)

// validateModel checks the invariants the writers rely on.
//
// The parser establishes them for every application it accepts.
// They do not hold for the partial model it returns for a rejected one.
// A page whose state type was rejected keeps handlers that ask for that state,
// with no state type to bind them to.
//
// The writers used to dereference the missing type and panic.
// An error reports the same thing without a stack trace and without writing files.
func validateModel(m *model.App) error {
	for _, p := range m.Pages {
		if p == nil {
			return fmt.Errorf("model is incomplete: a page is missing")
		}
		if p.State != nil {
			continue
		}
		if h := statefulHandler(p); h != "" {
			return fmt.Errorf(
				"model is incomplete: %s.%s takes state, "+
					"but %s has no state type", p.TypeName, h, p.TypeName,
			)
		}
	}
	for _, h := range m.Actions {
		if h == nil {
			return fmt.Errorf("model is incomplete: an app action is missing")
		}
		if h.InputState == nil {
			continue
		}
		if _, ok := m.States[h.InputState.StateTypeName]; !ok {
			return fmt.Errorf(
				"model is incomplete: app action %s takes state %s, "+
					"which the model does not declare",
				h.Name, h.InputState.StateTypeName,
			)
		}
	}
	return nil
}

// statefulHandler returns the name of a handler on p that takes state,
// or "" when none does.
func statefulHandler(p *model.Page) string {
	for _, h := range p.Actions {
		if h != nil && h.InputState != nil {
			return h.Name
		}
	}
	if p.StreamOpen != nil && p.StreamOpen.InputState != nil {
		return p.StreamOpen.Name
	}
	if p.StreamClose != nil && p.StreamClose.InputState != nil {
		return p.StreamClose.Name
	}
	for _, h := range p.EventHandlers {
		if h != nil && h.InputState != nil {
			return h.Name
		}
	}
	return ""
}
