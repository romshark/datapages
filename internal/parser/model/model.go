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
	PageOffline  *Page

	RecoverError        *RecoverError // Nullable.
	GlobalHeadGenerator *GlobalHead   // Nullable.

	Session *SessionType // Nullable.

	// Assets is the static file serving the app package declares.
	// The zero value serves no files.
	Assets Assets

	Pages   []*Page
	Events  []*Event
	Actions []*Handler // App-level POST/PUT/PATCH/DELETE actions.
}

// Assets is the static file serving an embed.FS variable of the app package
// declares by naming a URL path in its doc comment.
type Assets struct {
	// URLPrefix is the URL path the files are served under.
	URLPrefix string

	// Dir is the directory the files are read from,
	// relative to the app package.
	Dir string
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

type PageSpecialization int8

const (
	_ PageSpecialization = iota
	PageTypeIndex
	PageTypeError404
	PageTypeError500
	PageTypeOffline
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
}

type AbstractPage struct {
	Expr     ast.Expr
	TypeName string

	Methods       []*Handler
	StreamOpen    *Handler
	StreamClose   *Handler
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

	InputRequest  *Input
	InputStreamID *Input
	InputSSE      *Input
	InputSession  *Input
	InputPath     *Input
	InputQuery    *Input
	InputSignals  *Input
	// InputDispatches are the datapages.Dispatcher[EventXXX] parameters,
	// in user-defined order. One dispatcher publishes one event type.
	InputDispatches []*InputDispatch
	InputPageCache  *Input   // datapages.PageCacheWriter handle.
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

type EventHandler struct {
	Expr ast.Expr

	Name          string
	EventTypeName string

	InputEvent    *Input
	InputSSE      *Input
	InputStreamID *Input
	InputSession  *Input
	OrderedInputs []*Input // Inputs in user-defined order.

	OutputErr *Output
}

// InputKind constants identify handler input parameter kinds.
const (
	InputKindRequest   = "request"
	InputKindStreamID  = "streamID"
	InputKindSSE       = "sse"
	InputKindSession   = "session"
	InputKindPath      = "path"
	InputKindQuery     = "query"
	InputKindSignals   = "signals"
	InputKindDispatch  = "dispatch"
	InputKindPageCache = "pageCache"
	InputKindEvent     = "event"
	InputKindErr       = "err"
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
)

// IsSubject reports whether k is any subject segment kind.
func (k SubjectKind) IsSubject() bool { return k != SubjectKindNone }

// IsUser reports whether k carries the ID of the user the event is addressed to,
// which makes its stream require authentication.
func (k SubjectKind) IsUser() bool { return k == SubjectKindUser }

// String returns the datapages type name of k.
func (k SubjectKind) String() string {
	switch k {
	case SubjectKindValue:
		return "datapages.Subject"
	case SubjectKindUser:
		return "datapages.SubjectUser"
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
