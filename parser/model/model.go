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
	Actions []*Handler // App-level POST/PUT/DELETE actions.
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
	InputOrder        []string // InputKind constants in user-defined order.

	OutputBody           *TemplComponent // templ.Component body (actions only)
	OutputRedirect       *Output
	OutputRedirectStatus *Output
	OutputNewSession     *Output
	OutputCloseSession   *Output
	OutputEnableBgStream *Output
	OutputDisableRefresh *Output
	OutputErr            *Output
}

type InputDispatch struct {
	Expr           ast.Expr
	Name           string
	Type           Type
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
	InputSignals      *Input
	InputOrder        []string // InputKind constants in user-defined order.

	OutputErr *Output
}

// InputKind constants identify handler input parameters for InputOrder.
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

type Input struct {
	Expr ast.Expr

	Name string
	Type Type
}

type Output struct {
	Expr ast.Expr

	Name string

	Type Type
}

type Type struct {
	Resolved types.Type
	TypeExpr ast.Expr
}

type Event struct {
	Expr ast.Expr

	TypeName string
	Subject  string

	HasTargetUserIDs bool
}
