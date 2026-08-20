package model

import (
	"go/ast"
	"go/token"
	"go/types"
)

type App struct {
	Fset    *token.FileSet
	PkgPath string
	// PkgName is the name the app package declares. It qualifies app types
	// in generated code and need not match the last element of PkgPath.
	PkgName string
	Expr    ast.Expr

	PageIndex    *Page
	PageError404 *Page
	PageError500 *Page

	RecoverError        *RecoverError // Nullable.
	GlobalHeadGenerator *GlobalHead   // Nullable.

	Session *SessionType // Nullable.

	Pages   []*Page
	Events  []*Event
	Actions []*Handler // App-level POST/PUT/PATCH/DELETE actions.

	// States are all state types the source package declares, keyed by type name.
	// Pages reference them directly or through embedded abstract pages.
	// Several pages may share one state type.
	States map[string]*StateType
}

// GlobalHead is the app-wide (*App).Head hook. Its parameters are matched by
// type in any order, so OrderedInputs records the order it declares them in.
type GlobalHead struct {
	Expr         ast.Expr
	InputSession bool
	// OrderedInputs lists InputKind constants in declaration order.
	OrderedInputs []string
}

// RecoverError is the (*App).RecoverError hook. Its parameters are matched by
// type in any order, so OrderedInputs records the order it declares them in.
type RecoverError struct {
	Expr ast.Expr
	// OrderedInputs lists InputKind constants in declaration order.
	OrderedInputs []string
}

// SessionType is the datapages.Session[Data] instantiation the application uses.
// All handlers must agree on the same one.
type SessionType struct {
	Expr ast.Expr
	Data Type // The Data type argument.
}

// StateType represents a per-page-instance server-side state type
// declared as an exported struct.
type StateType struct {
	Expr     ast.Expr
	TypeName string
}

type PageSpecialization int8

const (
	_ PageSpecialization = iota
	PageTypeIndex
	PageTypeError404
	PageTypeError500
)

type Page struct {
	Expr     ast.Expr
	TypeName string
	Route    string

	PageSpecialization PageSpecialization

	GET           *HandlerGET
	Actions       []*Handler
	StreamOpen    *Handler
	StreamClose   *Handler
	EventHandlers []*EventHandler
	Embeds        []*AbstractPage

	// EmbedTypes maps an embedded abstract page type name to the type
	// written at the embed site, keyed by base name.
	// For a generic abstract the type is the instantiation, e.g. "Base" maps to
	// Base[StateFoo]. Needed to construct the embed in generated code.
	EmbedTypes map[string]Type

	// State is the state type referenced by any stateful handler on
	// this page (or its embedded abstract pages). Nullable.
	State *StateType
}

type AbstractPage struct {
	Expr     ast.Expr
	TypeName string

	// TypeParams lists the abstract page's type parameter names in
	// declaration order, e.g. ["S"] for `type Base[S any] struct {...}`.
	// Empty when the abstract is not generic.
	TypeParams []string

	Methods       []*Handler
	StreamOpen    *Handler
	StreamClose   *Handler
	EventHandlers []*EventHandler
	Embeds        []*AbstractPage

	// State is the state type referenced by any stateful handler on
	// this abstract page. Nullable. Not meaningful when the handler's
	// state parameter references a type parameter — the concrete
	// binding is resolved at each embed site during flattening.
	State *StateType
}

type TemplComponent struct {
	*Output
}

type HandlerGET struct {
	*Handler
	OutputBody *TemplComponent
	OutputHead *TemplComponent
}

type Handler struct {
	Expr ast.Expr

	Name string

	HTTPMethod string
	Route      string

	InputRequest  *Input
	InputStreamID *Input
	InputSSE      *Input
	InputSession  *Input
	InputPath     *Input
	InputQuery    *Input
	InputSignals  *Input
	InputState    *InputState // state *T; nullable.
	InputStateID  *Input      // stateID string; nullable.
	// InputDispatches are the datapages.Dispatcher[EventXXX] parameters,
	// in user-defined order. One dispatcher publishes one event type.
	InputDispatches []*InputDispatch
	OrderedInputs   []*Input // Inputs in user-defined order.

	OutputBody           *TemplComponent // templ.Component body (actions only)
	OutputHead           *TemplComponent // templ.Component head (actions only)
	OutputRedirect       *Output
	OutputNewSession     *Output
	OutputCloseSession   *Output
	OutputEnableBgStream *Output
	OutputDisableRefresh *Output
	OutputErr            *Output
	OrderedOutputs       []*Output // Outputs in user-defined order.
}

type InputDispatch struct {
	*Input
	EventTypeName string
}

// InputState wraps the state input with the referenced state type name.
//
// When IsTypeParam is true, StateTypeName holds the type-parameter name
// (e.g. "S") of the enclosing abstract page, not a concrete state type.
// The concrete binding is resolved per-page during embed flattening.
type InputState struct {
	*Input
	StateTypeName string
	IsTypeParam   bool
}

type EventHandler struct {
	Expr ast.Expr

	Name          string
	EventTypeName string

	InputEvent    *Input
	InputSSE      *Input
	InputStreamID *Input
	InputSession  *Input
	InputState    *InputState // state *T; nullable.
	InputStateID  *Input      // stateID string; nullable.
	OrderedInputs []*Input    // Inputs in user-defined order.

	OutputErr *Output
}

// InputKind constants identify handler input parameter kinds.
const (
	InputKindRequest  = "request"
	InputKindStreamID = "streamID"
	InputKindSSE      = "sse"
	InputKindSession  = "session"
	InputKindPath     = "path"
	InputKindQuery    = "query"
	InputKindSignals  = "signals"
	InputKindDispatch = "dispatch"
	InputKindEvent    = "event"
	InputKindState    = "state"
	InputKindStateID  = "stateID"
	InputKindErr      = "err"
)

// OutputKind constants identify handler output return value kinds.
const (
	OutputKindBody           = "body"
	OutputKindHead           = "head"
	OutputKindRedirect       = "redirect"
	OutputKindNewSession     = "newSession"
	OutputKindCloseSession   = "closeSession"
	OutputKindEnableBgStream = "enableBackgroundStreaming"
	OutputKindDisableRefresh = "disableRefreshAfterHidden"
	OutputKindErr            = "err"
)

type Input struct {
	Expr ast.Expr

	Kind string // InputKind constant.
	Name string
	Type Type
}

type Output struct {
	Expr ast.Expr

	Kind string // OutputKind constant.
	Name string

	Type Type
}

type Type struct {
	Resolved types.Type
	TypeExpr ast.Expr
}

// SubjectKind identifies the datapages subject segment type of an event field.
type SubjectKind uint8

const (
	// SubjectKindNone means the field isn't a subject field.
	SubjectKindNone SubjectKind = iota

	// SubjectKindValue is datapages.Subject.
	SubjectKindValue

	// SubjectKindUser is datapages.SubjectUser.
	SubjectKindUser

	// SubjectKindStateID is datapages.SubjectStateID.
	SubjectKindStateID
)

// IsSubject reports whether k is any subject segment kind.
func (k SubjectKind) IsSubject() bool { return k != SubjectKindNone }

// IsUser reports whether k carries the ID of the user the event is addressed to,
// which makes its stream require authentication.
func (k SubjectKind) IsUser() bool { return k == SubjectKindUser }

// IsStateID reports whether k carries the state ID of the tab the event is
// addressed to, which the server resolves from the connecting tab's
// validated instance header.
func (k SubjectKind) IsStateID() bool { return k == SubjectKindStateID }

// String returns the datapages type name of k.
func (k SubjectKind) String() string {
	switch k {
	case SubjectKindValue:
		return "datapages.Subject"
	case SubjectKindUser:
		return "datapages.SubjectUser"
	case SubjectKindStateID:
		return "datapages.SubjectStateID"
	}
	return ""
}

// SubjectField represents a field of an event type that is typed as a
// datapages subject segment. Its value is appended to the NATS subject
// at dispatch time, in field definition order.
//
// SignalName, when non-empty, marks the field as signal-scoped:
// the SSE stream handler reads this signal from the client
// and subscribes to the subject with the signal value appended.
// This enables per-instance event routing without authentication.
type SubjectField struct {
	FieldName  string      // e.g. "Recipient"
	Kind       SubjectKind // e.g. SubjectKindUser for datapages.SubjectUser
	SignalName string      // e.g. "instance_id" (from signal:"instance_id" tag)
}

type Event struct {
	Expr ast.Expr

	TypeName string
	Subject  string

	SubjectFields []SubjectField
}

// HasSubjectUser reports whether the event has a datapages.SubjectUser subject field.
func (e *Event) HasSubjectUser() bool {
	for _, sf := range e.SubjectFields {
		if sf.Kind.IsUser() {
			return true
		}
	}
	return false
}

// HasSubjectStateID reports whether the event has a datapages.SubjectStateID
// subject field. Like SubjectUser, SubjectStateID is resolved on the server
// side at stream connect — it uses the HMAC-validated Datapages-Instance
// header of the connecting tab, so only the tab whose state-id matches the
// dispatched value receives the event.
func (e *Event) HasSubjectStateID() bool {
	for _, sf := range e.SubjectFields {
		if sf.Kind.IsStateID() {
			return true
		}
	}
	return false
}

// IsPrivate reports whether the event targets specific users.
func (e *Event) IsPrivate() bool { return e.HasSubjectUser() }

// HasSubjectFields reports whether the event has any subject fields.
func (e *Event) HasSubjectFields() bool { return len(e.SubjectFields) > 0 }

// HasSignalSubjectFields reports whether the event has any
// signal-scoped subject fields (fields with a signal:"..." tag).
func (e *Event) HasSignalSubjectFields() bool {
	for _, sf := range e.SubjectFields {
		if sf.SignalName != "" {
			return true
		}
	}
	return false
}

// IsSignalScoped reports whether the event uses signal-based
// subject routing (has at least one signal-tagged subject field).
func (e *Event) IsSignalScoped() bool { return e.HasSignalSubjectFields() }

// IsStateIDScoped reports whether the event uses state-id-based
// subject routing (has a SubjectStateID field). At stream connect
// the server uses the validated Datapages-Instance header as the
// subject segment, so the event is delivered only to the tab whose
// state-id matches the dispatched value.
func (e *Event) IsStateIDScoped() bool { return e.HasSubjectStateID() }
