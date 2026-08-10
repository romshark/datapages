package model

import (
	"go/ast"
	"go/token"
	"go/types"
)

type App struct {
	Fset    *token.FileSet
	PkgPath string
	Expr    ast.Expr

	PageIndex    *Page
	PageError404 *Page
	PageError500 *Page

	RecoverError        ast.Expr    // Nullable.
	GlobalHeadGenerator *GlobalHead // Nullable.

	Session *SessionType // Nullable.

	Pages   []*Page
	Events  []*Event
	Actions []*Handler // App-level POST/PUT/PATCH/DELETE actions.

	// States are all declared StateXXX types in the source package,
	// keyed by type name. A state type may be referenced by at most
	// one concrete page (directly or through embedded abstract pages).
	States map[string]*StateType
}

type GlobalHead struct {
	Expr              ast.Expr
	InputSession      bool
	InputSessionToken bool
}

type SessionType struct {
	Expr ast.Expr
}

// StateType represents a per-page-instance server-side state type
// declared as `type StateXXX struct`.
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

	InputRequest      *Input
	InputStreamID     *Input
	InputSSE          *Input
	InputSessionToken *Input
	InputSession      *Input
	InputPath         *Input
	InputQuery        *Input
	InputSignals      *Input
	InputState        *InputState // state *StateXXX; nullable.
	InputStateID      *Input      // stateID string; nullable.
	InputDispatch     *InputDispatch
	OrderedInputs     []*Input // Inputs in user-defined order.

	OutputBody           *TemplComponent // templ.Component body (actions only)
	OutputRedirect       *Output
	OutputRedirectStatus *Output
	OutputNewSession     *Output
	OutputCloseSession   *Output
	OutputEnableBgStream *Output
	OutputDisableRefresh *Output
	OutputErr            *Output
	OrderedOutputs       []*Output // Outputs in user-defined order.
}

type InputDispatch struct {
	*Input
	EventTypeNames []string
}

// InputState wraps the state input with the referenced StateXXX type name.
//
// When IsTypeParam is true, StateTypeName holds the type-parameter name
// (e.g. "S") of the enclosing abstract page, not a concrete StateXXX.
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

	InputEvent        *Input
	InputSSE          *Input
	InputStreamID     *Input
	InputSessionToken *Input
	InputSession      *Input
	InputState        *InputState // state *StateXXX; nullable.
	InputStateID      *Input      // stateID string; nullable.
	OrderedInputs     []*Input    // Inputs in user-defined order.

	OutputErr *Output
}

// InputKind constants identify handler input parameter kinds.
const (
	InputKindRequest      = "request"
	InputKindStreamID     = "streamID"
	InputKindSSE          = "sse"
	InputKindSessionToken = "sessionToken"
	InputKindSession      = "session"
	InputKindPath         = "path"
	InputKindQuery        = "query"
	InputKindSignals      = "signals"
	InputKindDispatch     = "dispatch"
	InputKindEvent        = "event"
	InputKindState        = "state"
	InputKindStateID      = "stateID"
)

// OutputKind constants identify handler output return value kinds.
const (
	OutputKindBody           = "body"
	OutputKindHead           = "head"
	OutputKindRedirect       = "redirect"
	OutputKindRedirectStatus = "redirectStatus"
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

// SubjectField represents a Subject-prefixed field on an event type.
// The field name suffix (e.g. "User" from "SubjectUser") identifies the
// subject segment; values are appended to the NATS subject at dispatch time.
//
// SignalName, when non-empty, marks the field as signal-scoped:
// the SSE stream handler reads this signal from the client
// and subscribes to the subject with the signal value appended.
// This enables per-instance event routing without authentication.
type SubjectField struct {
	FieldName  string // e.g. "SubjectUser"
	Name       string // e.g. "User" (suffix after "Subject")
	SignalName string // e.g. "instance_id" (from signal:"instance_id" tag)
	Singular   bool   // true when the field type is string (not []string)
}

type Event struct {
	Expr ast.Expr

	TypeName string
	Subject  string

	SubjectFields []SubjectField
}

// HasSubjectUser reports whether the event has a SubjectUser field.
func (e *Event) HasSubjectUser() bool {
	for _, sf := range e.SubjectFields {
		if sf.Name == "User" {
			return true
		}
	}
	return false
}

// HasSubjectStateID reports whether the event has a SubjectStateID field.
// Like SubjectUser, SubjectStateID is a special subject field resolved on
// the server side at stream connect — it uses the HMAC-validated
// Datapages-Instance header of the connecting tab, so only the tab whose
// state-id matches the dispatched value receives the event.
func (e *Event) HasSubjectStateID() bool {
	for _, sf := range e.SubjectFields {
		if sf.Name == "StateID" {
			return true
		}
	}
	return false
}

// IsPrivate reports whether the event targets specific users
// (has a SubjectUser field).
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
