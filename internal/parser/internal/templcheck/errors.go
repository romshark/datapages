package templcheck

import (
	"errors"
	"fmt"
)

var (
	ErrHrefRelative                 = errors.New("template uses relative href")
	ErrActionHardcoded              = errors.New("template uses hardcoded action")
	ErrFormAction                   = errors.New("template uses form action attribute")
	ErrActionWrongPage              = errors.New("template uses action from another page")
	ErrActionContext                = errors.New("action helper used outside Datastar action context")
	ErrHrefContext                  = errors.New("href helper used in Datastar action context")
	ErrHrefUnverifiable             = errors.New("href expression must use href package functions")
	ErrActionUnverifiable           = errors.New("action expression must use action package functions")
	ErrActionUnverifiableWithPrefix = errors.New("action call must not be concatenated with a prefix")
	ErrActionUnverifiableWithSuffix = errors.New("action call must not be concatenated with a suffix")
	ErrHrefExternalIsRelative       = errors.New("href.External used with relative URL")
)

// HrefRelativeError is ErrHrefRelative with context.
type HrefRelativeError struct {
	URL string // e.g. "/login"
}

func (e *HrefRelativeError) Error() string {
	return fmt.Sprintf("%v: %s", ErrHrefRelative, e.URL)
}

func (e *HrefRelativeError) Unwrap() error { return ErrHrefRelative }

// ActionHardcodedError is ErrActionHardcoded with context.
type ActionHardcodedError struct {
	URL string // e.g. "/login/submit"
}

func (e *ActionHardcodedError) Error() string {
	return fmt.Sprintf("%v: %s", ErrActionHardcoded, e.URL)
}

func (e *ActionHardcodedError) Unwrap() error { return ErrActionHardcoded }

// FormActionError is ErrFormAction with context.
type FormActionError struct{}

func (e *FormActionError) Error() string {
	return ErrFormAction.Error()
}

func (e *FormActionError) Unwrap() error { return ErrFormAction }

// ActionWrongPageError is ErrActionWrongPage with context.
type ActionWrongPageError struct {
	ActionFunc string // e.g. "POSTPageProfileSave"
	PageType   string // e.g. "PageSettings" (the page whose template uses the action)
	OwnerPage  string // e.g. "PageProfile" or "App" (the page/app that owns the action)
}

func (e *ActionWrongPageError) Error() string {
	return fmt.Sprintf("%v: %s belongs to %s, used in %s",
		ErrActionWrongPage, e.ActionFunc, e.OwnerPage, e.PageType)
}

func (e *ActionWrongPageError) Unwrap() error { return ErrActionWrongPage }

// ActionContextError is ErrActionContext with context.
type ActionContextError struct {
	AttrName   string // e.g. "href"
	ActionFunc string // e.g. "POSTPageLoginSubmit"
}

func (e *ActionContextError) Error() string {
	return fmt.Sprintf("%v: %s in %s attribute",
		ErrActionContext, e.ActionFunc, e.AttrName)
}

func (e *ActionContextError) Unwrap() error { return ErrActionContext }

// HrefContextError is ErrHrefContext with context.
type HrefContextError struct {
	AttrName string // e.g. "data-on:click"
	HrefFunc string // e.g. "PageIndex"
}

func (e *HrefContextError) Error() string {
	return fmt.Sprintf("%v: %s in %s attribute",
		ErrHrefContext, e.HrefFunc, e.AttrName)
}

func (e *HrefContextError) Unwrap() error { return ErrHrefContext }

// HrefUnverifiableError is ErrHrefUnverifiable with context.
type HrefUnverifiableError struct {
	Expr string // the full expression value
}

func (e *HrefUnverifiableError) Error() string {
	return fmt.Sprintf("%v: %s", ErrHrefUnverifiable, e.Expr)
}

func (e *HrefUnverifiableError) Unwrap() error { return ErrHrefUnverifiable }

// ActionUnverifiableError is ErrActionUnverifiable with context.
type ActionUnverifiableError struct {
	Expr string // the full expression value
}

func (e *ActionUnverifiableError) Error() string {
	return fmt.Sprintf("%v: %s", ErrActionUnverifiable, e.Expr)
}

func (e *ActionUnverifiableError) Unwrap() error { return ErrActionUnverifiable }

// ActionUnverifiableWithPrefixError is ErrActionUnverifiableWithPrefix with context.
type ActionUnverifiableWithPrefixError struct {
	Expr       string // the full expression value
	ActionFunc string // e.g. "POSTPageIndexCalculate"
	Prefix     string // the prefix expression source, e.g. `"$_fresh = true; "`
}

func (e *ActionUnverifiableWithPrefixError) Error() string {
	return fmt.Sprintf("%v: %s", ErrActionUnverifiableWithPrefix, e.Expr)
}

func (e *ActionUnverifiableWithPrefixError) Unwrap() error {
	return ErrActionUnverifiableWithPrefix
}

// ActionUnverifiableWithSuffixError is ErrActionUnverifiableWithSuffix with context.
type ActionUnverifiableWithSuffixError struct {
	Expr       string // the full expression value
	ActionFunc string // e.g. "POSTPageIndexCalculate"
	Suffix     string // the suffix expression source, e.g. `"; $count++"`
}

func (e *ActionUnverifiableWithSuffixError) Error() string {
	return fmt.Sprintf("%v: %s", ErrActionUnverifiableWithSuffix, e.Expr)
}

func (e *ActionUnverifiableWithSuffixError) Unwrap() error {
	return ErrActionUnverifiableWithSuffix
}

// HrefExternalIsRelativeError is ErrHrefExternalIsRelative with context.
type HrefExternalIsRelativeError struct {
	URL string // the internal URL, e.g. "/login"
}

func (e *HrefExternalIsRelativeError) Error() string {
	return fmt.Sprintf("%v: %s", ErrHrefExternalIsRelative, e.URL)
}

func (e *HrefExternalIsRelativeError) Unwrap() error { return ErrHrefExternalIsRelative }
