package parser

import (
	"errors"
	"fmt"
	"go/token"
	"iter"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/romshark/datapages/internal/parser/internal/paramvalidation"
	"github.com/romshark/datapages/internal/parser/internal/templcheck"
	"github.com/romshark/datapages/internal/parser/validate"
)

var (
	ErrAppMissingTypeApp         = errors.New(`missing required type "App"`)
	ErrAppMissingPageIndex       = errors.New(`missing required page type "PageIndex"`)
	ErrSignatureMissingReq       = errors.New(`missing the *http.Request parameter`)
	ErrSignatureMissingStreamID  = errors.New(`missing the datapages.StreamID parameter`)
	ErrSignatureMultiErrRet      = errors.New(`multiple error return values`)
	ErrSignatureUnsupportedInput = errors.New(`unsupported input parameter`)
	ErrSignatureEvHandMissingSSE = errors.New(
		"event handler must have a datapages.SSE parameter",
	)
	ErrSignatureEvHandReturnMustBeError = errors.New(
		"event handler must return only error",
	)
	ErrSignatureEvHandMissingEvent = errors.New(
		"event handler must have a parameter of an event type",
	)
	ErrSignatureEvHandMultipleEvents = errors.New(
		"event handler must have exactly one parameter of an event type",
	)
	ErrSignatureGETMissingBody = errors.New(
		"GET handler must return body datapages.Component",
	)
	ErrSignatureActionHeadWithoutBody = errors.New(
		"action returning a head must return body datapages.Component",
	)
	ErrSignatureDuplicateOutput = errors.New(
		"duplicate return value",
	)

	ErrAppHeadMustTakeRequest = errors.New(
		"head must accept exactly one *http.Request parameter",
	)
	ErrAppHeadMustReturnHead = errors.New(
		"head must return exactly datapages.Head",
	)
	ErrAppHeadUnsupportedInput = errors.New("head has unsupported input parameter")

	ErrAppRecoverErrorInvalidSignature = errors.New(
		`"RecoverError" must have signature ` +
			`(error, datapages.SSE) error`,
	)

	ErrTypeParams = errors.New("type parameters are not supported")

	ErrPageNotStruct           = errors.New("page type must be a struct type")
	ErrPageMissingFieldApp     = errors.New(`page is missing the "App *App" field`)
	ErrPageHasExtraFields      = errors.New(`page struct has unsupported fields`)
	ErrPageMissingGET          = errors.New(`page is missing the GET handler`)
	ErrPageConflictingGETEmbed = errors.New("conflicting GET handlers in embedded")
	ErrPageNameInvalid         = errors.New("page has invalid name")
	ErrPageMissingPathComm     = errors.New("page is missing path comment")
	ErrPageInvalidPathComm     = errors.New("page has invalid path comment")
	ErrPageIndexPathMustBeRoot = errors.New(`PageIndex path must be "/"`)

	ErrAppUnsupportedMethod = errors.New(
		"unsupported method on App; App takes Head, RecoverError and " +
			"POST*/PUT*/PATCH*/DELETE* (actions). " +
			"GET, On* and StreamOpen/StreamClose belong on a page",
	)

	ErrUnsupportedMethod = errors.New(
		"unsupported public method on page type; " +
			"use GET, POST*/PUT*/PATCH*/DELETE* (actions), " +
			"On* (event handlers), or StreamOpen/StreamClose (stream hooks)",
	)

	ErrActionNameMissing      = errors.New("action handler must have a name")
	ErrActionNameInvalid      = errors.New("action has invalid name")
	ErrActionMissingPathComm  = errors.New("action handler is missing path comment")
	ErrActionInvalidPathComm  = errors.New("action handler has invalid path comment")
	ErrActionPathNotUnderPage = errors.New("action handler path is not under page path")

	ErrEventCommMissing     = errors.New("event type is missing subject comment")
	ErrEventCommInvalid     = errors.New("event type has invalid subject comment")
	ErrEventSubjectInvalid  = errors.New("event subject is invalid")
	ErrEvHandDuplicate      = errors.New("duplicate event handler for event")
	ErrEvHandDuplicateEmbed = errors.New("duplicate event handler for event in embedded")

	ErrEventFieldUnexported = errors.New("event field must be exported")
	ErrEventFieldMissingTag = errors.New("event field must have json tag")
	ErrEventFieldEmptyTag   = errors.New(
		"event field json tag must have a non-empty name",
	)
	ErrEventFieldDuplicateTag = errors.New("event field has duplicate json tag value")

	ErrPathParamNotStruct       = paramvalidation.ErrPathParamNotStruct
	ErrPathFieldUnexported      = paramvalidation.ErrPathFieldUnexported
	ErrPathFieldMissingTag      = paramvalidation.ErrPathFieldMissingTag
	ErrPathFieldUnsupportedType = paramvalidation.ErrPathFieldUnsupportedType
	ErrPathFieldNotInRoute      = paramvalidation.ErrPathFieldNotInRoute
	ErrPathMissingRouteVar      = paramvalidation.ErrPathMissingRouteVar
	ErrPathFieldDuplicateTag    = paramvalidation.ErrPathFieldDuplicateTag
	ErrPathFieldEmptyTag        = paramvalidation.ErrPathFieldEmptyTag

	ErrQueryParamNotStruct       = paramvalidation.ErrQueryParamNotStruct
	ErrQueryFieldUnexported      = paramvalidation.ErrQueryFieldUnexported
	ErrQueryFieldMissingTag      = paramvalidation.ErrQueryFieldMissingTag
	ErrQueryFieldDuplicateTag    = paramvalidation.ErrQueryFieldDuplicateTag
	ErrQueryFieldEmptyTag        = paramvalidation.ErrQueryFieldEmptyTag
	ErrQueryFieldUnsupportedType = paramvalidation.ErrQueryFieldUnsupportedType

	ErrQueryReflectSignalNotInSignals = paramvalidation.ErrQueryReflectSignalNotInSignals

	ErrFieldTypeUnexported = paramvalidation.ErrFieldTypeUnexported

	ErrSignalsParamNotStruct    = paramvalidation.ErrSignalsParamNotStruct
	ErrSignalsFieldUnexported   = paramvalidation.ErrSignalsFieldUnexported
	ErrSignalsFieldMissingTag   = paramvalidation.ErrSignalsFieldMissingTag
	ErrSignalsFieldDuplicateTag = paramvalidation.ErrSignalsFieldDuplicateTag
	ErrSignalsFieldEmptyTag     = paramvalidation.ErrSignalsFieldEmptyTag

	ErrDispatchParamNotEvent = paramvalidation.ErrDispatchParamNotEvent

	ErrSessionTypeConflict = errors.New(
		"all handlers must use the same datapages.Session[Data] instantiation",
	)

	ErrNewSessionWithSSE = errors.New(
		"newSession cannot be used together with sse parameter",
	)
	ErrCloseSessionWithSSE = errors.New(
		"closeSession cannot be used together with sse parameter",
	)
	ErrSSEOnAppMethod = errors.New(
		"the sse parameter is only allowed on page methods",
	)

	ErrEnableBgStreamNotGET = errors.New(
		"enableBackgroundStreaming can only be used in GET handlers",
	)
	ErrDisableRefreshNotGET = errors.New(
		"disableRefreshAfterHidden can only be used in GET handlers",
	)

	ErrSignatureUnsupportedOutput = errors.New(
		"unsupported output return value",
	)
	ErrSignatureStreamHookReturnMustBeError = errors.New(
		"stream hook must return only error",
	)
	ErrStreamHookDuplicateEmbed = errors.New(
		"conflicting stream hook in embedded",
	)

	ErrEventSubjectUserNoSession = errors.New(
		"event addressing users requires a Session type",
	)

	ErrEventSubjectAfterPayload = errors.New(
		"subject field must be defined before payload fields",
	)

	ErrRouteConflict = errors.New("conflicting route")

	ErrRouteWildcardStream = errors.New(
		"page route ending in a wildcard cannot have a stream",
	)

	ErrRouteVarNameInvalid = validate.ErrRouteVarNameInvalid

	ErrEventSubjectDuplicate = errors.New("duplicate event subject")

	ErrEventSubjectOverlap = errors.New("overlapping event subjects")

	ErrEventSubjectDuplicateSignal = errors.New(
		"multiple event subject fields with the same signal tag",
	)

	ErrEventSubjectUserSignal = errors.New(
		"user-addressed subject field must not have a signal tag",
	)

	ErrDispatchDuplicate = errors.New(
		"multiple dispatchers for the same event type",
	)

	ErrEventTypeNameConflict = errors.New("two events of the same type name")

	ErrEventDeclUnreadable = errors.New("event declaration cannot be read")

	ErrEventSubjectPrefixedField = errors.New(
		"event field named like a subject field isn't typed as one",
	)

	ErrEventSubjectSignalInvalid = errors.New("invalid signal tag value")

	ErrEventSubjectDerivedType = errors.New(
		"event subject field must name datapages.Subject or datapages.SubjectUser",
	)

	ErrTemplHrefRelative                 = templcheck.ErrHrefRelative
	ErrTemplActionHardcoded              = templcheck.ErrActionHardcoded
	ErrTemplFormAction                   = templcheck.ErrFormAction
	ErrTemplActionWrongPage              = templcheck.ErrActionWrongPage
	ErrTemplActionContext                = templcheck.ErrActionContext
	ErrTemplHrefContext                  = templcheck.ErrHrefContext
	ErrTemplHrefUnverifiable             = templcheck.ErrHrefUnverifiable
	ErrTemplActionUnverifiable           = templcheck.ErrActionUnverifiable
	ErrTemplActionUnverifiableWithPrefix = templcheck.ErrActionUnverifiableWithPrefix
	ErrTemplActionUnverifiableWithSuffix = templcheck.ErrActionUnverifiableWithSuffix
	ErrTemplHrefExternalIsRelative       = templcheck.ErrHrefExternalIsRelative
)

func normPos(pos token.Position) token.Position {
	if pos.Filename != "" {
		pos.Filename = filepath.Base(pos.Filename)
	}
	return pos
}

func posLess(a, b token.Position) bool {
	az, bz := a.Filename == "", b.Filename == ""
	if az != bz {
		return !az // known < unknown
	}
	if a.Filename != b.Filename {
		return a.Filename < b.Filename
	}
	if a.Line != b.Line {
		return a.Line < b.Line
	}
	return a.Column < b.Column
}

// earliestPkgPos returns the package statement an error without a position of
// its own is anchored to.
func earliestPkgPos(pkg *packages.Package) token.Position {
	best := token.Position{}
	for _, f := range pkg.Syntax {
		p := normPos(pkg.Fset.Position(f.Package))
		if best.Filename == "" || posLess(p, best) {
			best = p
		}
	}
	return best
}

// posFromPackagesError reads the "file:line:col" of a [packages.Error].
// A field it cannot read stays zero.
//
// The parse cuts from the right, hence a Windows drive letter in
// "C:\x\y\z.go:12:3" is not mistaken for the line.
func posFromPackagesError(pe packages.Error) token.Position {
	s := pe.Pos
	if s == "" {
		return token.Position{}
	}

	i := strings.LastIndexByte(s, ':')
	if i < 0 {
		return normPos(token.Position{Filename: s})
	}
	colStr := s[i+1:]
	s = s[:i]

	j := strings.LastIndexByte(s, ':')
	if j < 0 {
		return normPos(token.Position{Filename: s})
	}
	lineStr := s[j+1:]
	file := s[:j]

	line, _ := strconv.Atoi(lineStr)
	col, _ := strconv.Atoi(colStr)
	return normPos(token.Position{Filename: file, Line: line, Column: col})
}

type errorEntry struct {
	pos token.Position
	seq uint64
	err error
}

func (e errorEntry) Error() string {
	return fmt.Sprintf("at %s:%d:%d: %v",
		e.pos.Filename, e.pos.Line, e.pos.Column, e.err)
}

func (e errorEntry) Unwrap() error { return e.err }

type Errors struct {
	errs []errorEntry
	seq  uint64
}

func (e *Errors) Error() string {
	l := len(e.errs)
	if l == 0 {
		return ""
	}
	return fmt.Sprintf("%d error(s) in source package", l)
}

func (e *Errors) Err(err error) {
	e.ErrAt(token.Position{}, err)
}

func (e *Errors) Entry(i int) (token.Position, error) {
	if i >= len(e.errs) {
		return token.Position{}, nil
	}
	en := e.errs[i]
	return en.pos, en.err
}

func (e *Errors) All() iter.Seq2[int, error] {
	return func(yield func(int, error) bool) {
		for i, e := range e.errs {
			if !yield(i, e) {
				break
			}
		}
	}
}

func (e *Errors) Len() int { return len(e.errs) }

func (e *Errors) ErrAt(pos token.Position, err error) {
	if err == nil {
		return
	}
	e.seq++
	e.errs = append(e.errs, errorEntry{
		pos: normPos(pos),
		seq: e.seq,
		err: err,
	})
}

func sortErrors(e *Errors) {
	if e == nil {
		return
	}
	slices.SortFunc(e.errs, func(a, b errorEntry) int {
		az, bz := a.pos.Filename == "", b.pos.Filename == ""
		if az != bz {
			if az {
				return 1
			}
			return -1
		}
		if a.pos.Filename != b.pos.Filename {
			if a.pos.Filename < b.pos.Filename {
				return -1
			}
			return 1
		}
		if a.pos.Line != b.pos.Line {
			if a.pos.Line < b.pos.Line {
				return -1
			}
			return 1
		}
		if a.pos.Column != b.pos.Column {
			if a.pos.Column < b.pos.Column {
				return -1
			}
			return 1
		}
		// Two errors at one position keep the order they were reported in.
		if a.seq < b.seq {
			return -1
		}
		if a.seq > b.seq {
			return 1
		}
		return 0
	})
}

// PageMissingFieldAppError is [ErrPageMissingFieldApp] with suggestion context.
type PageMissingFieldAppError struct {
	TypeName string // e.g. "PageProfile"
}

func (e *PageMissingFieldAppError) Error() string {
	return fmt.Sprintf("%v: %s", ErrPageMissingFieldApp, e.TypeName)
}

func (e *PageMissingFieldAppError) Unwrap() error { return ErrPageMissingFieldApp }

// ActionPathNotUnderPageError is [ErrActionPathNotUnderPage] with suggestion context.
type ActionPathNotUnderPageError struct {
	PagePath   string // e.g. "/profile/"
	Recv       string // e.g. "PageProfile"
	MethodName string // e.g. "POSTFoo"
}

func (e *ActionPathNotUnderPageError) Error() string {
	return fmt.Sprintf("%v: %s.%s", ErrActionPathNotUnderPage, e.Recv, e.MethodName)
}

func (e *ActionPathNotUnderPageError) Unwrap() error { return ErrActionPathNotUnderPage }

// PageMissingPathCommError is [ErrPageMissingPathComm] with suggestion context.
type PageMissingPathCommError struct {
	TypeName string // e.g. "PageProfile"
}

func (e *PageMissingPathCommError) Error() string {
	return fmt.Sprintf("%v: %s", ErrPageMissingPathComm, e.TypeName)
}

func (e *PageMissingPathCommError) Unwrap() error { return ErrPageMissingPathComm }

// ActionMissingPathCommError is [ErrActionMissingPathComm] with suggestion context.
type ActionMissingPathCommError struct {
	PagePath   string // e.g. "/profile/" (empty for App-level actions)
	Recv       string // e.g. "PageProfile" or "App"
	MethodName string // e.g. "POSTFoo"
}

func (e *ActionMissingPathCommError) Error() string {
	return fmt.Sprintf("%v: %s.%s", ErrActionMissingPathComm, e.Recv, e.MethodName)
}

func (e *ActionMissingPathCommError) Unwrap() error { return ErrActionMissingPathComm }

// PageMissingGETError is [ErrPageMissingGET] with suggestion context.
type PageMissingGETError struct {
	TypeName string // e.g. "PageProfile"
}

func (e *PageMissingGETError) Error() string {
	return fmt.Sprintf("%v: %s", ErrPageMissingGET, e.TypeName)
}

func (e *PageMissingGETError) Unwrap() error { return ErrPageMissingGET }

// PageInvalidPathCommError is [ErrPageInvalidPathComm] with suggestion context.
type PageInvalidPathCommError struct {
	TypeName string // e.g. "PageProfile"
}

func (e *PageInvalidPathCommError) Error() string {
	return fmt.Sprintf("%v: %s", ErrPageInvalidPathComm, e.TypeName)
}

func (e *PageInvalidPathCommError) Unwrap() error { return ErrPageInvalidPathComm }

// PageIndexPathMustBeRootError is [ErrPageIndexPathMustBeRoot] with suggestion context.
type PageIndexPathMustBeRootError struct {
	Route string // the invalid route, e.g. "/home"
}

func (e *PageIndexPathMustBeRootError) Error() string {
	return fmt.Sprintf("%v, got %q", ErrPageIndexPathMustBeRoot, e.Route)
}

func (e *PageIndexPathMustBeRootError) Unwrap() error { return ErrPageIndexPathMustBeRoot }

// ActionInvalidPathCommError is [ErrActionInvalidPathComm] with suggestion context.
type ActionInvalidPathCommError struct {
	Recv       string // e.g. "PageProfile" or "App"
	MethodName string // e.g. "POSTFoo"
}

func (e *ActionInvalidPathCommError) Error() string {
	return fmt.Sprintf("%v: %s.%s", ErrActionInvalidPathComm, e.Recv, e.MethodName)
}

func (e *ActionInvalidPathCommError) Unwrap() error { return ErrActionInvalidPathComm }

// EventCommMissingError is [ErrEventCommMissing] with suggestion context.
type EventCommMissingError struct {
	TypeName string // e.g. "EventFoo"
}

func (e *EventCommMissingError) Error() string {
	return fmt.Sprintf("%v: %s", ErrEventCommMissing, e.TypeName)
}

func (e *EventCommMissingError) Unwrap() error { return ErrEventCommMissing }

// EventCommInvalidError is [ErrEventCommInvalid] with suggestion context.
type EventCommInvalidError struct {
	TypeName string // e.g. "EventFoo"
}

func (e *EventCommInvalidError) Error() string {
	return fmt.Sprintf("%v: %s", ErrEventCommInvalid, e.TypeName)
}

func (e *EventCommInvalidError) Unwrap() error { return ErrEventCommInvalid }

// EventFieldMissingTagError is [ErrEventFieldMissingTag] with suggestion context.
type EventFieldMissingTagError struct {
	FieldName string // e.g. "UserID"
	TypeName  string // e.g. "EventFoo"
}

func (e *EventFieldMissingTagError) Error() string {
	return fmt.Sprintf("%v: field %s in %s", ErrEventFieldMissingTag, e.FieldName, e.TypeName)
}

func (e *EventFieldMissingTagError) Unwrap() error { return ErrEventFieldMissingTag }

// EventFieldEmptyTagError is [ErrEventFieldEmptyTag] with suggestion context.
type EventFieldEmptyTagError struct {
	FieldName string // e.g. "UserID"
	TypeName  string // e.g. "EventFoo"
}

func (e *EventFieldEmptyTagError) Error() string {
	return fmt.Sprintf("%v: field %s in %s", ErrEventFieldEmptyTag, e.FieldName, e.TypeName)
}

func (e *EventFieldEmptyTagError) Unwrap() error { return ErrEventFieldEmptyTag }

// EventFieldDuplicateTagError is [ErrEventFieldDuplicateTag] with suggestion context.
type EventFieldDuplicateTagError struct {
	FieldName string // e.g. "UserID"
	TagValue  string // e.g. "user_id"
	TypeName  string // e.g. "EventFoo"
}

func (e *EventFieldDuplicateTagError) Error() string {
	return fmt.Sprintf("%v: %q on field %s in %s",
		ErrEventFieldDuplicateTag, e.TagValue, e.FieldName, e.TypeName)
}

func (e *EventFieldDuplicateTagError) Unwrap() error { return ErrEventFieldDuplicateTag }

// EventSubjectUserNoSessionError is [ErrEventSubjectUserNoSession]
// with suggestion context.
type EventSubjectUserNoSessionError struct {
	TypeName string // e.g. "EventFoo"
	PkgName  string // e.g. "app"
}

func (e *EventSubjectUserNoSessionError) Error() string {
	return fmt.Sprintf("%v: %s", ErrEventSubjectUserNoSession, e.TypeName)
}

func (e *EventSubjectUserNoSessionError) Unwrap() error {
	return ErrEventSubjectUserNoSession
}

// EventSubjectAfterPayloadError is [ErrEventSubjectAfterPayload]
// with suggestion context.
type EventSubjectAfterPayloadError struct {
	FieldName string // e.g. "SubjectUser"
	TypeName  string // e.g. "EventFoo"
}

func (e *EventSubjectAfterPayloadError) Error() string {
	return fmt.Sprintf("%v: %s in %s",
		ErrEventSubjectAfterPayload, e.FieldName, e.TypeName)
}

func (e *EventSubjectAfterPayloadError) Unwrap() error {
	return ErrEventSubjectAfterPayload
}

// RouteConflictError is [ErrRouteConflict] with the pattern that could not be
// registered and what the router said about it.
type RouteConflictError struct {
	Pattern string
	Owner   string
	Reason  string
}

func (e *RouteConflictError) Error() string {
	return fmt.Sprintf("%v: %s cannot serve %q: %s",
		ErrRouteConflict, e.Owner, e.Pattern, e.Reason)
}

func (e *RouteConflictError) Unwrap() error { return ErrRouteConflict }

// RouteWildcardStreamError is [ErrRouteWildcardStream] with the page.
// The stream endpoint sits under the page route. A {name...} wildcard matches
// the rest of the path, which leaves nothing for the endpoint to sit in.
type RouteWildcardStreamError struct {
	TypeName string
	Route    string
}

func (e *RouteWildcardStreamError) Error() string {
	return fmt.Sprintf("%v: %s is %q", ErrRouteWildcardStream, e.TypeName, e.Route)
}

func (e *RouteWildcardStreamError) Unwrap() error { return ErrRouteWildcardStream }

// RouteVarNameInvalidError is [ErrRouteVarNameInvalid] with the wildcard,
// the route it sits in and what claims that route.
// See [validate.RouteVarName] for the rule.
type RouteVarNameInvalidError struct {
	Owner string // "PageFoo", "PageFoo.POSTBar" or "App.POSTBar"
	Route string
	Var   string
}

func (e *RouteVarNameInvalidError) Error() string {
	return fmt.Sprintf("%v: {%s} in %s route %q",
		ErrRouteVarNameInvalid, e.Var, e.Owner, e.Route)
}

func (e *RouteVarNameInvalidError) Unwrap() error { return ErrRouteVarNameInvalid }

// EventSubjectDuplicateError is [ErrEventSubjectDuplicate] with the two types
// that share the subject. A subject is the case an inbound event is matched by,
// which two events cannot share.
type EventSubjectDuplicateError struct {
	Subject       string
	TypeName      string
	FirstTypeName string
}

func (e *EventSubjectDuplicateError) Error() string {
	return fmt.Sprintf("%v: %s declares %q, already declared by %s",
		ErrEventSubjectDuplicate, e.TypeName, e.Subject, e.FirstTypeName)
}

func (e *EventSubjectDuplicateError) Unwrap() error { return ErrEventSubjectDuplicate }

// EventSubjectOverlapError is [ErrEventSubjectOverlap] with the two types whose
// subjects cover a common subject. An event with subject fields occupies
// everything below its own, which leaves no subject there for another event.
type EventSubjectOverlapError struct {
	Subject       string
	TypeName      string
	FirstSubject  string
	FirstTypeName string
}

func (e *EventSubjectOverlapError) Error() string {
	return fmt.Sprintf("%v: %s declares %q, which overlaps %q of %s",
		ErrEventSubjectOverlap, e.TypeName, e.Subject,
		e.FirstSubject, e.FirstTypeName)
}

func (e *EventSubjectOverlapError) Unwrap() error { return ErrEventSubjectOverlap }

// EventSubjectDuplicateSignalError is [ErrEventSubjectDuplicateSignal]
// with suggestion context.
type EventSubjectDuplicateSignalError struct {
	FieldName      string // e.g. "SubjectFoo" (second occurrence)
	FirstFieldName string // e.g. "SubjectBar" (first occurrence)
	SignalName     string // e.g. "instance_id"
	TypeName       string // e.g. "EventCalcUpdated"
}

func (e *EventSubjectDuplicateSignalError) Error() string {
	return fmt.Sprintf("%v: %s has duplicate signal %q in %s (already used by %s)",
		ErrEventSubjectDuplicateSignal, e.FieldName, e.SignalName, e.TypeName, e.FirstFieldName)
}

func (e *EventSubjectDuplicateSignalError) Unwrap() error {
	return ErrEventSubjectDuplicateSignal
}

// EventSubjectUserSignalError is [ErrEventSubjectUserSignal]
// with suggestion context.
type EventSubjectUserSignalError struct {
	TypeName string // e.g. "EventChat"
}

func (e *EventSubjectUserSignalError) Error() string {
	return fmt.Sprintf("%v: in %s", ErrEventSubjectUserSignal, e.TypeName)
}

func (e *EventSubjectUserSignalError) Unwrap() error {
	return ErrEventSubjectUserSignal
}

// DispatchDuplicateError is [ErrDispatchDuplicate] with the handler context.
type DispatchDuplicateError struct {
	Recv          string
	MethodName    string
	EventTypeName string
}

func (e *DispatchDuplicateError) Error() string {
	return fmt.Sprintf("%v: %s in %s.%s",
		ErrDispatchDuplicate, e.EventTypeName, e.Recv, e.MethodName)
}

func (e *DispatchDuplicateError) Unwrap() error { return ErrDispatchDuplicate }

// EventSubjectPrefixedFieldError is [ErrEventSubjectPrefixedField]
// with the field and type context.
type EventSubjectPrefixedFieldError struct {
	FieldName string
	TypeName  string
}

func (e *EventSubjectPrefixedFieldError) Error() string {
	return fmt.Sprintf("%v: field %s in %s",
		ErrEventSubjectPrefixedField, e.FieldName, e.TypeName)
}

func (e *EventSubjectPrefixedFieldError) Unwrap() error {
	return ErrEventSubjectPrefixedField
}

// EventSubjectDerivedTypeError is [ErrEventSubjectDerivedType] with the field,
// the type it names and the segment type that type is declared from.
type EventSubjectDerivedTypeError struct {
	FieldName       string // e.g. "To"
	TypeName        string // e.g. "EventDirect"
	DeclTypeName    string // e.g. "UserID"
	SubjectTypeName string // e.g. "datapages.SubjectUser"
}

func (e *EventSubjectDerivedTypeError) Error() string {
	return fmt.Sprintf("%v: field %s in %s names %s, declared from %s",
		ErrEventSubjectDerivedType,
		e.FieldName, e.TypeName, e.DeclTypeName, e.SubjectTypeName)
}

func (e *EventSubjectDerivedTypeError) Unwrap() error {
	return ErrEventSubjectDerivedType
}

// EventSubjectSignalInvalidError is [ErrEventSubjectSignalInvalid]
// with suggestion context.
type EventSubjectSignalInvalidError struct {
	FieldName  string // e.g. "SubjectInstance"
	SignalName string // the invalid tag value
	TypeName   string // e.g. "EventCalc"
}

func (e *EventSubjectSignalInvalidError) Error() string {
	return fmt.Sprintf("%v: %s has signal %q in %s",
		ErrEventSubjectSignalInvalid, e.FieldName, e.SignalName, e.TypeName)
}

func (e *EventSubjectSignalInvalidError) Unwrap() error {
	return ErrEventSubjectSignalInvalid
}

// Type aliases for templ-check error types defined in the templcheck subpackage.
type (
	TemplHrefRelativeError                 = templcheck.HrefRelativeError
	TemplActionHardcodedError              = templcheck.ActionHardcodedError
	TemplFormActionError                   = templcheck.FormActionError
	TemplActionWrongPageError              = templcheck.ActionWrongPageError
	TemplActionContextError                = templcheck.ActionContextError
	TemplHrefContextError                  = templcheck.HrefContextError
	TemplHrefUnverifiableError             = templcheck.HrefUnverifiableError
	TemplActionUnverifiableError           = templcheck.ActionUnverifiableError
	TemplActionUnverifiableWithPrefixError = templcheck.ActionUnverifiableWithPrefixError
	TemplActionUnverifiableWithSuffixError = templcheck.ActionUnverifiableWithSuffixError
	TemplHrefExternalIsRelativeError       = templcheck.HrefExternalIsRelativeError
)

// SignatureUnsupportedInputError is [ErrSignatureUnsupportedInput] with context.
type SignatureUnsupportedInputError struct {
	ParamName  string // e.g. "b"
	ParamType  string // e.g. "*http.Request"
	Recv       string // e.g. "PageFoo"
	MethodName string // e.g. "GET"
	// CandidateNames lists the handler inputs the parameter's type could stand for.
	CandidateNames []string
}

func (e *SignatureUnsupportedInputError) Error() string {
	return fmt.Sprintf("%v %s %s in %s.%s",
		ErrSignatureUnsupportedInput, e.ParamName, e.ParamType, e.Recv, e.MethodName)
}

func (e *SignatureUnsupportedInputError) Unwrap() error {
	return ErrSignatureUnsupportedInput
}

// EventTypeNameConflictError is [ErrEventTypeNameConflict] with context.
// Generated code names an event by its type name alone, which is why one
// application cannot take part in two events of one name.
type EventTypeNameConflictError struct {
	TypeName     string // e.g. "EventUpdated"
	PkgPath      string // The package of the event named second.
	FirstPkgPath string // The package of the event named first.
}

func (e *EventTypeNameConflictError) Error() string {
	return fmt.Sprintf("%v: %s is declared in %s and in %s",
		ErrEventTypeNameConflict, e.TypeName, e.FirstPkgPath, e.PkgPath)
}

func (e *EventTypeNameConflictError) Unwrap() error {
	return ErrEventTypeNameConflict
}

// EventDeclUnreadableError is [ErrEventDeclUnreadable] with context.
// It names an event type whose declaration the loaded packages do not carry.
type EventDeclUnreadableError struct {
	TypeName string // e.g. "EventUpdated"
	PkgPath  string // The package declaring it.
}

func (e *EventDeclUnreadableError) Error() string {
	return fmt.Sprintf("%v: %s in %s",
		ErrEventDeclUnreadable, e.TypeName, e.PkgPath)
}

func (e *EventDeclUnreadableError) Unwrap() error { return ErrEventDeclUnreadable }
