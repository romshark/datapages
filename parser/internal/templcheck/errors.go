package templcheck

import (
	"errors"
	"fmt"
)

var (
	ErrHrefRelative           = errors.New("template uses relative href")
	ErrActionHardcoded        = errors.New("template uses hardcoded action")
	ErrFormAction             = errors.New("template uses form action attribute")
	ErrActionWrongPage        = errors.New("template uses action from another page")
	ErrActionContext          = errors.New("action helper used outside Datastar action context")
	ErrHrefContext            = errors.New("href helper used in Datastar action context")
	ErrHrefUnverifiable       = errors.New("href expression must use href package functions")
	ErrActionUnverifiable     = errors.New("action expression must use action package functions")
	ErrHrefExternalIsRelative = errors.New("href.External used with relative URL")
)

// ErrorHrefRelative is ErrHrefRelative with context.
type ErrorHrefRelative struct {
	URL string // e.g. "/login"
}

func (e *ErrorHrefRelative) Error() string {
	return fmt.Sprintf("%v: %s", ErrHrefRelative, e.URL)
}

func (e *ErrorHrefRelative) Unwrap() error { return ErrHrefRelative }

// ErrorActionHardcoded is ErrActionHardcoded with context.
type ErrorActionHardcoded struct {
	URL string // e.g. "/login/submit"
}

func (e *ErrorActionHardcoded) Error() string {
	return fmt.Sprintf("%v: %s", ErrActionHardcoded, e.URL)
}

func (e *ErrorActionHardcoded) Unwrap() error { return ErrActionHardcoded }

// ErrorFormAction is ErrFormAction with context.
type ErrorFormAction struct{}

func (e *ErrorFormAction) Error() string {
	return ErrFormAction.Error()
}

func (e *ErrorFormAction) Unwrap() error { return ErrFormAction }

// ErrorActionWrongPage is ErrActionWrongPage with context.
type ErrorActionWrongPage struct {
	ActionFunc string // e.g. "POSTPageProfileSave"
	PageType   string // e.g. "PageSettings" (the page whose template uses the action)
	OwnerPage  string // e.g. "PageProfile" or "App" (the page/app that owns the action)
}

func (e *ErrorActionWrongPage) Error() string {
	return fmt.Sprintf("%v: %s belongs to %s, used in %s",
		ErrActionWrongPage, e.ActionFunc, e.OwnerPage, e.PageType)
}

func (e *ErrorActionWrongPage) Unwrap() error { return ErrActionWrongPage }

// ErrorActionContext is ErrActionContext with context.
type ErrorActionContext struct {
	AttrName   string // e.g. "href"
	ActionFunc string // e.g. "POSTPageLoginSubmit"
}

func (e *ErrorActionContext) Error() string {
	return fmt.Sprintf("%v: %s in %s attribute",
		ErrActionContext, e.ActionFunc, e.AttrName)
}

func (e *ErrorActionContext) Unwrap() error { return ErrActionContext }

// ErrorHrefContext is ErrHrefContext with context.
type ErrorHrefContext struct {
	AttrName string // e.g. "data-on:click"
	HrefFunc string // e.g. "PageIndex"
}

func (e *ErrorHrefContext) Error() string {
	return fmt.Sprintf("%v: %s in %s attribute",
		ErrHrefContext, e.HrefFunc, e.AttrName)
}

func (e *ErrorHrefContext) Unwrap() error { return ErrHrefContext }

// ErrorHrefUnverifiable is ErrHrefUnverifiable with context.
type ErrorHrefUnverifiable struct {
	Expr string // the full expression value
}

func (e *ErrorHrefUnverifiable) Error() string {
	return fmt.Sprintf("%v: %s", ErrHrefUnverifiable, e.Expr)
}

func (e *ErrorHrefUnverifiable) Unwrap() error { return ErrHrefUnverifiable }

// ErrorActionUnverifiable is ErrActionUnverifiable with context.
type ErrorActionUnverifiable struct {
	Expr string // the full expression value
}

func (e *ErrorActionUnverifiable) Error() string {
	return fmt.Sprintf("%v: %s", ErrActionUnverifiable, e.Expr)
}

func (e *ErrorActionUnverifiable) Unwrap() error { return ErrActionUnverifiable }

// ErrorHrefExternalIsRelative is ErrHrefExternalIsRelative with context.
type ErrorHrefExternalIsRelative struct {
	URL string // the internal URL, e.g. "/login"
}

func (e *ErrorHrefExternalIsRelative) Error() string {
	return fmt.Sprintf("%v: %s", ErrHrefExternalIsRelative, e.URL)
}

func (e *ErrorHrefExternalIsRelative) Unwrap() error { return ErrHrefExternalIsRelative }
