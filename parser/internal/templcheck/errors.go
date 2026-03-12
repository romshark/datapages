package templcheck

import (
	"errors"
	"fmt"
)

var (
	ErrHardcodedHref        = errors.New("template uses hardcoded app-internal href")
	ErrHardcodedAction      = errors.New("template uses hardcoded app-internal action")
	ErrActionWrongPage      = errors.New("template uses action from another page")
	ErrActionContext        = errors.New("action helper used outside Datastar action context")
	ErrHrefContext          = errors.New("href helper used in Datastar action context")
	ErrHrefUnverifiable     = errors.New("href expression must use href package functions")
	ErrExternalWithInternal = errors.New("href.External used with app-internal URL")
)

// ErrorHardcodedHref is ErrHardcodedHref with context.
type ErrorHardcodedHref struct {
	URL string // e.g. "/login"
}

func (e *ErrorHardcodedHref) Error() string {
	return fmt.Sprintf("%v: %s", ErrHardcodedHref, e.URL)
}

func (e *ErrorHardcodedHref) Unwrap() error { return ErrHardcodedHref }

// ErrorHardcodedAction is ErrHardcodedAction with context.
type ErrorHardcodedAction struct {
	URL string // e.g. "/login/submit"
}

func (e *ErrorHardcodedAction) Error() string {
	return fmt.Sprintf("%v: %s", ErrHardcodedAction, e.URL)
}

func (e *ErrorHardcodedAction) Unwrap() error { return ErrHardcodedAction }

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

// ErrorExternalWithInternal is ErrExternalWithInternal with context.
type ErrorExternalWithInternal struct {
	URL string // the internal URL, e.g. "/login"
}

func (e *ErrorExternalWithInternal) Error() string {
	return fmt.Sprintf("%v: %s", ErrExternalWithInternal, e.URL)
}

func (e *ErrorExternalWithInternal) Unwrap() error { return ErrExternalWithInternal }
