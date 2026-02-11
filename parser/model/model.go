package model

import (
	"go/ast"
	"go/token"
	"go/types"
)

type App struct {
	Fset *token.FileSet
	Expr ast.Expr

	PageIndex    *Page
	PageError404 *Page
	PageError500 *Page

	Recover500          ast.Expr // Nullable.
	GlobalHeadGenerator ast.Expr // Nullable.

	Session *SessionType // Nullable.

	Pages  []*Page
	Events []*Event
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

	OutputRedirect       *Output
	OutputRedirectStatus *Output
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

	OutputErr *Output
}

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
