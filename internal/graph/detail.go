package graph

import (
	"go/types"

	"github.com/romshark/datapages/internal/gotypes"
	"github.com/romshark/datapages/internal/parser/model"
	"github.com/romshark/datapages/internal/structtag"
)

// handlerIO renders what a handler takes and what it returns, in the order the
// method declares them.
func handlerIO(h *model.Handler) (in, out []Param) {
	if h == nil {
		return nil, nil
	}
	for _, i := range h.OrderedInputs {
		in = append(in, inputParam(h, i))
	}
	for _, o := range h.OrderedOutputs {
		out = append(out, Param{
			Kind: o.Kind, Name: o.Name, Type: typeName(o.Type),
		})
	}
	return in, out
}

// eventHandlerIO is [handlerIO] for the OnEventXXX methods, which take an event
// and return at most an error.
func eventHandlerIO(h *model.EventHandler) (in, out []Param) {
	if h == nil {
		return nil, nil
	}
	for _, i := range h.OrderedInputs {
		p := Param{Kind: i.Kind, Name: i.Name, Type: typeName(i.Type)}
		if i.Kind == model.InputKindEvent {
			p.Event = h.EventTypeName
		}
		in = append(in, p)
	}
	if h.OutputErr != nil {
		out = append(out, Param{
			Kind: h.OutputErr.Kind, Name: h.OutputErr.Name,
			Type: typeName(h.OutputErr.Type),
		})
	}
	return in, out
}

// inputParam renders one parameter. The path, query and signals structs are
// expanded: their fields are what the handler actually reads off the request.
func inputParam(h *model.Handler, i *model.Input) Param {
	p := Param{Kind: i.Kind, Name: i.Name, Type: typeName(i.Type)}
	switch i.Kind {
	case model.InputKindPath:
		p.Fields = structFields(i.Type.Resolved, structtag.PathTagValue, nil)
	case model.InputKindQuery:
		p.Fields = structFields(i.Type.Resolved, structtag.QueryTagValue, nil)
	case model.InputKindSignals:
		p.Fields = structFields(i.Type.Resolved, structtag.JSONTagValue, nil)
	case model.InputKindDispatch:
		for _, d := range h.InputDispatches {
			if d.Input == i {
				p.Event = d.EventTypeName
			}
		}
		// datapages.Dispatcher[E] says nothing the event name does not.
		p.Type = ""
	}
	// The struct these are declared as is anonymous and unreadable rendered;
	// the fields carry everything worth showing.
	if len(p.Fields) > 0 {
		p.Type = ""
	}
	return p
}

// structFields lists the fields of a struct type, named on the wire by wire and
// annotated by note. Both may be nil.
func structFields(
	t types.Type,
	wire func(tag string) string,
	note func(field string) string,
) []Field {
	if t == nil {
		return nil
	}
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	fields := make([]Field, 0, st.NumFields())
	for i := range st.NumFields() {
		f := st.Field(i)
		if !f.Exported() {
			continue
		}
		field := Field{Name: f.Name(), Type: gotypes.QualifiedTypeName(f.Type())}
		if wire != nil {
			field.Wire = wire(st.Tag(i))
		}
		if note != nil {
			field.Note = note(f.Name())
		}
		fields = append(fields, field)
	}
	return fields
}

// eventFields renders the payload of an event: every field, with the subject
// segments marked, since those route the event instead of travelling in it.
func eventFields(e *model.Event, t types.Type) []Field {
	note := func(name string) string {
		for _, sf := range e.SubjectFields {
			if sf.FieldName != name {
				continue
			}
			switch {
			case sf.Kind.IsUser():
				return "subject: user"
			case sf.SignalName != "":
				return "subject, signal: " + sf.SignalName
			}
			return "subject"
		}
		return ""
	}
	return structFields(t, structtag.JSONTagValue, note)
}

// eventTypes maps every event type name the model reaches to its Go type,
// which the model itself does not carry: it is read off the handler that takes
// the event, or off the type argument of a dispatcher that publishes it.
func eventTypes(m *model.App) map[string]types.Type {
	out := map[string]types.Type{}
	handlers := func(hs ...*model.Handler) {
		for _, h := range hs {
			if h == nil {
				continue
			}
			for _, d := range h.InputDispatches {
				// A partial model can carry the event name without the
				// parameter it was read from.
				if d.Input == nil {
					continue
				}
				if _, ok := out[d.EventTypeName]; ok {
					continue
				}
				if t := dispatcherEvent(d.Type.Resolved); t != nil {
					out[d.EventTypeName] = t
				}
			}
		}
	}
	eventHandlers := func(ehs []*model.EventHandler) {
		for _, eh := range ehs {
			if eh.InputEvent == nil || eh.InputEvent.Type.Resolved == nil {
				continue
			}
			out[eh.EventTypeName] = eh.InputEvent.Type.Resolved
		}
	}

	handlers(m.Actions...)
	for _, p := range m.Pages {
		if p.GET != nil {
			handlers(p.GET.Handler)
		}
		handlers(p.StreamOpen, p.StreamClose)
		handlers(p.Actions...)
		eventHandlers(p.EventHandlers)
	}
	for _, ap := range abstractPages(m) {
		handlers(ap.StreamOpen, ap.StreamClose)
		handlers(ap.Methods...)
		eventHandlers(ap.EventHandlers)
	}
	return out
}

// dispatcherEvent returns the E of a datapages.Dispatcher[E].
func dispatcherEvent(t types.Type) types.Type {
	n, ok := t.(*types.Named)
	if !ok || n.TypeArgs() == nil || n.TypeArgs().Len() == 0 {
		return nil
	}
	return n.TypeArgs().At(0)
}

func typeName(t model.Type) string {
	if t.Resolved == nil {
		return ""
	}
	return gotypes.QualifiedTypeName(t.Resolved)
}
