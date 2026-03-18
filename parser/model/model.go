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

	Recover500          ast.Expr    // Nullable.
	GlobalHeadGenerator *GlobalHead // Nullable.

	Session *SessionType // Nullable.

	Pages   []*Page
	Events  []*Event
	Actions []*Handler // App-level POST/PUT/PATCH/DELETE actions.
}

type GlobalHead struct {
	Expr              ast.Expr
	InputSession      bool
	InputSessionToken bool
}

type SessionType struct {
	Expr ast.Expr
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
	EventHandlers []*EventHandler
	Embeds        []*AbstractPage
}

type AbstractPage struct {
	Expr     ast.Expr
	TypeName string

	Methods       []*Handler
	EventHandlers []*EventHandler
	Embeds        []*AbstractPage
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
	InputSSE          *Input
	InputSessionToken *Input
	InputSession      *Input
	InputPath         *Input
	InputQuery        *Input
	InputSignals      *Input
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

type EventHandler struct {
	Expr ast.Expr

	Name          string
	EventTypeName string

	InputEvent        *Input
	InputSSE          *Input
	InputSessionToken *Input
	InputSession      *Input
	OrderedInputs     []*Input // Inputs in user-defined order.

	OutputErr *Output
}

// InputKind constants identify handler input parameter kinds.
const (
	InputKindRequest      = "request"
	InputKindSSE          = "sse"
	InputKindSessionToken = "sessionToken"
	InputKindSession      = "session"
	InputKindPath         = "path"
	InputKindQuery        = "query"
	InputKindSignals      = "signals"
	InputKindDispatch     = "dispatch"
	InputKindEvent        = "event"
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
