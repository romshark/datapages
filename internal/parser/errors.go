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

	ErrPageMissingFieldApp     = errors.New(`page is missing the "App *App" field`)
	ErrPageHasExtraFields      = errors.New(`page struct has unsupported fields`)
	ErrPageMissingGET          = errors.New(`page is missing the GET handler`)
	ErrPageConflictingGETEmbed = errors.New("conflicting GET handlers in embedded")
	ErrPageNameInvalid         = errors.New("page has invalid name")
	ErrPageMissingPathComm     = errors.New("page is missing path comment")
	ErrPageInvalidPathComm     = errors.New("page has invalid path comment")
	ErrPageIndexPathMustBeRoot = errors.New(`PageIndex path must be "/"`)

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

	ErrEventSubjectDuplicate = errors.New(
		"duplicate event subject",
	)

	ErrEventSubjectOverlap = errors.New(
		"overlapping event subjects",
	)

	ErrEventSubjectDuplicateSignal = errors.New(
		"multiple event subject fields with the same signal tag",
	)

	ErrEventSubjectUserSignal = errors.New(
		"user-addressed subject field must not have a signal tag",
	)

	ErrDispatchDuplicate = errors.New(
		"multiple dispatchers for the same event type",
	)

	ErrEventSubjectPrefixedField = errors.New(
		"event field named like a subject field isn't typed as one",
	)

	ErrEventSubjectSignalInvalid = errors.New(
		"invalid signal tag value",
	)

	ErrStateParamNotPointer = errors.New(
		"state parameter must be a pointer to a named type",
	)
	ErrStateParamInvalidType = errors.New(
		"state parameter has invalid type",
	)
	ErrStateOnGET = errors.New(
		"state parameter is not allowed on GET handlers",
	)
	ErrStateDuplicate = errors.New(
		"handler has multiple state parameters",
	)
	ErrStateTypeArgPointer = errors.New(
		"type argument of an embedded abstract page must not be a pointer; " +
			"an abstract page takes its state as state *S",
	)
	ErrStateIDDuplicate = errors.New(
		"handler has multiple stateID parameters",
	)
	ErrStateConflict = errors.New(
		"page references multiple state types via its handlers and embeds",
	)
	ErrStateWithoutStream = errors.New(
		"page takes state but has no StreamOpen, StreamClose, or OnXXX handler " +
			"to anchor its lifecycle",
	)
	ErrStateAppActionUnbound = errors.New(
		"app-level action takes a state type that no page binds",
	)
	ErrStateIDParamNotString = errors.New(
		"stateID parameter must be of type string",
	)
	ErrStateIDWithoutState = errors.New(
		"stateID parameter requires the handler to also take state *T",
	)
	ErrSubjectStateIDWithSignal = errors.New(
		"SubjectStateID must not have a signal tag",
	)
	ErrSubjectStateIDWithoutState = errors.New(
		"event with SubjectStateID can only be handled by stateful pages",
	)
	ErrSubjectStateIDMixed = errors.New(
		"SubjectStateID must be the only subject field on the event",
	)
	ErrSubjectStateIDPageMixed = errors.New(
		"page handles a SubjectStateID event next to a private " +
			"or signal-scoped one",
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

// earliest position we can anchor "global" errors to (package statement).
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

// Best-effort parse for packages.Error.Pos which is typically "file:line:col".
func posFromPackagesError(pe packages.Error) token.Position {
	// keep it simple: split from the right so Windows drive letters don't break it
	// e.g. "C:\x\y\z.go:12:3"
	s := pe.Pos
	if s == "" {
		return token.Position{}
	}

	// last ":col"
	i := strings.LastIndexByte(s, ':')
	if i < 0 {
		return normPos(token.Position{Filename: s})
	}
	colStr := s[i+1:]
	s = s[:i]

	// last ":line"
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
		// deterministic tie-break
		if a.seq < b.seq {
			return -1
		}
		if a.seq > b.seq {
			return 1
		}
		return 0
	})
}

// ErrorPageMissingFieldApp is ErrPageMissingFieldApp with suggestion context.
type ErrorPageMissingFieldApp struct {
	TypeName string // e.g. "PageProfile"
}

func (e *ErrorPageMissingFieldApp) Error() string {
	return fmt.Sprintf("%v: %s", ErrPageMissingFieldApp, e.TypeName)
}

func (e *ErrorPageMissingFieldApp) Unwrap() error { return ErrPageMissingFieldApp }

// ErrorActionPathNotUnderPage is ErrActionPathNotUnderPage with suggestion context.
type ErrorActionPathNotUnderPage struct {
	PagePath   string // e.g. "/profile/"
	Recv       string // e.g. "PageProfile"
	MethodName string // e.g. "POSTFoo"
}

func (e *ErrorActionPathNotUnderPage) Error() string {
	return fmt.Sprintf("%v: %s.%s", ErrActionPathNotUnderPage, e.Recv, e.MethodName)
}

func (e *ErrorActionPathNotUnderPage) Unwrap() error { return ErrActionPathNotUnderPage }

// ErrorPageMissingPathComm is ErrPageMissingPathComm with suggestion context.
type ErrorPageMissingPathComm struct {
	TypeName string // e.g. "PageProfile"
}

func (e *ErrorPageMissingPathComm) Error() string {
	return fmt.Sprintf("%v: %s", ErrPageMissingPathComm, e.TypeName)
}

func (e *ErrorPageMissingPathComm) Unwrap() error { return ErrPageMissingPathComm }

// ErrorActionMissingPathComm is ErrActionMissingPathComm with suggestion context.
type ErrorActionMissingPathComm struct {
	PagePath   string // e.g. "/profile/" (empty for App-level actions)
	Recv       string // e.g. "PageProfile" or "App"
	MethodName string // e.g. "POSTFoo"
}

func (e *ErrorActionMissingPathComm) Error() string {
	return fmt.Sprintf("%v: %s.%s", ErrActionMissingPathComm, e.Recv, e.MethodName)
}

func (e *ErrorActionMissingPathComm) Unwrap() error { return ErrActionMissingPathComm }

// ErrorPageMissingGET is ErrPageMissingGET with suggestion context.
type ErrorPageMissingGET struct {
	TypeName string // e.g. "PageProfile"
}

func (e *ErrorPageMissingGET) Error() string {
	return fmt.Sprintf("%v: %s", ErrPageMissingGET, e.TypeName)
}

func (e *ErrorPageMissingGET) Unwrap() error { return ErrPageMissingGET }

// ErrorPageInvalidPathComm is ErrPageInvalidPathComm with suggestion context.
type ErrorPageInvalidPathComm struct {
	TypeName string // e.g. "PageProfile"
}

func (e *ErrorPageInvalidPathComm) Error() string {
	return fmt.Sprintf("%v: %s", ErrPageInvalidPathComm, e.TypeName)
}

func (e *ErrorPageInvalidPathComm) Unwrap() error { return ErrPageInvalidPathComm }

// ErrorPageIndexPathMustBeRoot is ErrPageIndexPathMustBeRoot with suggestion context.
type ErrorPageIndexPathMustBeRoot struct {
	Route string // the invalid route, e.g. "/home"
}

func (e *ErrorPageIndexPathMustBeRoot) Error() string {
	return fmt.Sprintf("%v, got %q", ErrPageIndexPathMustBeRoot, e.Route)
}

func (e *ErrorPageIndexPathMustBeRoot) Unwrap() error {
	return ErrPageIndexPathMustBeRoot
}

// ErrorActionInvalidPathComm is ErrActionInvalidPathComm with suggestion context.
type ErrorActionInvalidPathComm struct {
	Recv       string // e.g. "PageProfile" or "App"
	MethodName string // e.g. "POSTFoo"
}

func (e *ErrorActionInvalidPathComm) Error() string {
	return fmt.Sprintf("%v: %s.%s", ErrActionInvalidPathComm, e.Recv, e.MethodName)
}

func (e *ErrorActionInvalidPathComm) Unwrap() error { return ErrActionInvalidPathComm }

// ErrorEventCommMissing is ErrEventCommMissing with suggestion context.
type ErrorEventCommMissing struct {
	TypeName string // e.g. "EventFoo"
}

func (e *ErrorEventCommMissing) Error() string {
	return fmt.Sprintf("%v: %s", ErrEventCommMissing, e.TypeName)
}

func (e *ErrorEventCommMissing) Unwrap() error { return ErrEventCommMissing }

// ErrorEventCommInvalid is ErrEventCommInvalid with suggestion context.
type ErrorEventCommInvalid struct {
	TypeName string // e.g. "EventFoo"
}

func (e *ErrorEventCommInvalid) Error() string {
	return fmt.Sprintf("%v: %s", ErrEventCommInvalid, e.TypeName)
}

func (e *ErrorEventCommInvalid) Unwrap() error { return ErrEventCommInvalid }

// ErrorEventFieldMissingTag is ErrEventFieldMissingTag with suggestion context.
type ErrorEventFieldMissingTag struct {
	FieldName string // e.g. "UserID"
	TypeName  string // e.g. "EventFoo"
}

func (e *ErrorEventFieldMissingTag) Error() string {
	return fmt.Sprintf("%v: field %s in %s",
		ErrEventFieldMissingTag, e.FieldName, e.TypeName)
}

func (e *ErrorEventFieldMissingTag) Unwrap() error { return ErrEventFieldMissingTag }

// ErrorEventFieldEmptyTag is ErrEventFieldEmptyTag with suggestion context.
type ErrorEventFieldEmptyTag struct {
	FieldName string // e.g. "UserID"
	TypeName  string // e.g. "EventFoo"
}

func (e *ErrorEventFieldEmptyTag) Error() string {
	return fmt.Sprintf("%v: field %s in %s",
		ErrEventFieldEmptyTag, e.FieldName, e.TypeName)
}

func (e *ErrorEventFieldEmptyTag) Unwrap() error { return ErrEventFieldEmptyTag }

// ErrorEventFieldDuplicateTag is ErrEventFieldDuplicateTag with suggestion context.
type ErrorEventFieldDuplicateTag struct {
	FieldName string // e.g. "UserID"
	TagValue  string // e.g. "user_id"
	TypeName  string // e.g. "EventFoo"
}

func (e *ErrorEventFieldDuplicateTag) Error() string {
	return fmt.Sprintf("%v: %q on field %s in %s",
		ErrEventFieldDuplicateTag, e.TagValue, e.FieldName, e.TypeName)
}

func (e *ErrorEventFieldDuplicateTag) Unwrap() error { return ErrEventFieldDuplicateTag }

// ErrorEventSubjectUserNoSession is ErrEventSubjectUserNoSession
// with suggestion context.
type ErrorEventSubjectUserNoSession struct {
	TypeName string // e.g. "EventFoo"
	PkgName  string // e.g. "app"
}

func (e *ErrorEventSubjectUserNoSession) Error() string {
	return fmt.Sprintf("%v: %s", ErrEventSubjectUserNoSession, e.TypeName)
}

func (e *ErrorEventSubjectUserNoSession) Unwrap() error {
	return ErrEventSubjectUserNoSession
}

// ErrorEventSubjectAfterPayload is ErrEventSubjectAfterPayload
// with suggestion context.
type ErrorEventSubjectAfterPayload struct {
	FieldName string // e.g. "SubjectUser"
	TypeName  string // e.g. "EventFoo"
}

func (e *ErrorEventSubjectAfterPayload) Error() string {
	return fmt.Sprintf("%v: %s in %s",
		ErrEventSubjectAfterPayload, e.FieldName, e.TypeName)
}

func (e *ErrorEventSubjectAfterPayload) Unwrap() error {
	return ErrEventSubjectAfterPayload
}

// ErrorRouteConflict is ErrRouteConflict with the pattern that could not be
// registered and what the router said about it.
type ErrorRouteConflict struct {
	Pattern string
	Owner   string
	Reason  string
}

func (e *ErrorRouteConflict) Error() string {
	return fmt.Sprintf("%v: %s cannot serve %q: %s",
		ErrRouteConflict, e.Owner, e.Pattern, e.Reason)
}

func (e *ErrorRouteConflict) Unwrap() error { return ErrRouteConflict }

// ErrorRouteWildcardStream is ErrRouteWildcardStream with the page.
// The stream endpoint sits under the page route. A {name...} wildcard matches
// the rest of the path, which leaves nothing for the endpoint to sit in.
type ErrorRouteWildcardStream struct {
	TypeName string
	Route    string
}

func (e *ErrorRouteWildcardStream) Error() string {
	return fmt.Sprintf("%v: %s is %q", ErrRouteWildcardStream, e.TypeName, e.Route)
}

func (e *ErrorRouteWildcardStream) Unwrap() error { return ErrRouteWildcardStream }

// ErrorEventSubjectDuplicate is ErrEventSubjectDuplicate with the two types
// that share the subject. A subject is the case an inbound event is matched by,
// which two events cannot share.
type ErrorEventSubjectDuplicate struct {
	Subject       string
	TypeName      string
	FirstTypeName string
}

func (e *ErrorEventSubjectDuplicate) Error() string {
	return fmt.Sprintf("%v: %s declares %q, already declared by %s",
		ErrEventSubjectDuplicate, e.TypeName, e.Subject, e.FirstTypeName)
}

func (e *ErrorEventSubjectDuplicate) Unwrap() error { return ErrEventSubjectDuplicate }

// ErrorEventSubjectOverlap is ErrEventSubjectOverlap with the two types whose
// subjects cover a common subject. An event with subject fields occupies
// everything below its own, which leaves no subject there for another event.
type ErrorEventSubjectOverlap struct {
	Subject       string
	TypeName      string
	FirstSubject  string
	FirstTypeName string
}

func (e *ErrorEventSubjectOverlap) Error() string {
	return fmt.Sprintf("%v: %s declares %q, which overlaps %q of %s",
		ErrEventSubjectOverlap, e.TypeName, e.Subject,
		e.FirstSubject, e.FirstTypeName)
}

func (e *ErrorEventSubjectOverlap) Unwrap() error { return ErrEventSubjectOverlap }

// ErrorEventSubjectDuplicateSignal is ErrEventSubjectDuplicateSignal
// with suggestion context.
type ErrorEventSubjectDuplicateSignal struct {
	FieldName      string // e.g. "SubjectFoo" (second occurrence)
	FirstFieldName string // e.g. "SubjectBar" (first occurrence)
	SignalName     string // e.g. "instance_id"
	TypeName       string // e.g. "EventCalcUpdated"
}

func (e *ErrorEventSubjectDuplicateSignal) Error() string {
	return fmt.Sprintf("%v: %s has duplicate signal %q in %s (already used by %s)",
		ErrEventSubjectDuplicateSignal, e.FieldName,
		e.SignalName, e.TypeName, e.FirstFieldName)
}

func (e *ErrorEventSubjectDuplicateSignal) Unwrap() error {
	return ErrEventSubjectDuplicateSignal
}

// ErrorEventSubjectUserSignal is ErrEventSubjectUserSignal
// with suggestion context.
type ErrorEventSubjectUserSignal struct {
	TypeName string // e.g. "EventChat"
}

func (e *ErrorEventSubjectUserSignal) Error() string {
	return fmt.Sprintf("%v: in %s", ErrEventSubjectUserSignal, e.TypeName)
}

func (e *ErrorEventSubjectUserSignal) Unwrap() error {
	return ErrEventSubjectUserSignal
}

// ErrorDispatchDuplicate is ErrDispatchDuplicate with the handler context.
type ErrorDispatchDuplicate struct {
	Recv          string
	MethodName    string
	EventTypeName string
}

func (e *ErrorDispatchDuplicate) Error() string {
	return fmt.Sprintf("%v: %s in %s.%s",
		ErrDispatchDuplicate, e.EventTypeName, e.Recv, e.MethodName)
}

func (e *ErrorDispatchDuplicate) Unwrap() error { return ErrDispatchDuplicate }

// ErrorEventSubjectPrefixedField is ErrEventSubjectPrefixedField
// with the field and type context.
type ErrorEventSubjectPrefixedField struct {
	FieldName string
	TypeName  string
}

func (e *ErrorEventSubjectPrefixedField) Error() string {
	return fmt.Sprintf("%v: field %s in %s",
		ErrEventSubjectPrefixedField, e.FieldName, e.TypeName)
}

func (e *ErrorEventSubjectPrefixedField) Unwrap() error {
	return ErrEventSubjectPrefixedField
}

// ErrorEventSubjectSignalInvalid is ErrEventSubjectSignalInvalid
// with suggestion context.
type ErrorEventSubjectSignalInvalid struct {
	FieldName  string // e.g. "SubjectInstance"
	SignalName string // the invalid tag value
	TypeName   string // e.g. "EventCalc"
}

func (e *ErrorEventSubjectSignalInvalid) Error() string {
	return fmt.Sprintf("%v: %s has signal %q in %s",
		ErrEventSubjectSignalInvalid, e.FieldName, e.SignalName, e.TypeName)
}

func (e *ErrorEventSubjectSignalInvalid) Unwrap() error {
	return ErrEventSubjectSignalInvalid
}

// Type aliases for templ-check error types defined in the templcheck subpackage.
type (
	ErrorTemplHrefRelative                 = templcheck.ErrorHrefRelative
	ErrorTemplActionHardcoded              = templcheck.ErrorActionHardcoded
	ErrorTemplFormAction                   = templcheck.ErrorFormAction
	ErrorTemplActionWrongPage              = templcheck.ErrorActionWrongPage
	ErrorTemplActionContext                = templcheck.ErrorActionContext
	ErrorTemplHrefContext                  = templcheck.ErrorHrefContext
	ErrorTemplHrefUnverifiable             = templcheck.ErrorHrefUnverifiable
	ErrorTemplActionUnverifiable           = templcheck.ErrorActionUnverifiable
	ErrorTemplActionUnverifiableWithPrefix = templcheck.ErrorActionUnverifiableWithPrefix
	ErrorTemplActionUnverifiableWithSuffix = templcheck.ErrorActionUnverifiableWithSuffix
	ErrorTemplHrefExternalIsRelative       = templcheck.ErrorHrefExternalIsRelative
)

// ErrorSignatureUnsupportedInput is ErrSignatureUnsupportedInput with context.
type ErrorSignatureUnsupportedInput struct {
	ParamName  string // e.g. "b"
	ParamType  string // e.g. "*http.Request"
	Recv       string // e.g. "PageFoo"
	MethodName string // e.g. "GET"
	// CandidateNames lists the handler inputs the parameter's type could stand for.
	CandidateNames []string
}

func (e *ErrorSignatureUnsupportedInput) Error() string {
	return fmt.Sprintf("%v %s %s in %s.%s",
		ErrSignatureUnsupportedInput, e.ParamName, e.ParamType, e.Recv, e.MethodName)
}

func (e *ErrorSignatureUnsupportedInput) Unwrap() error {
	return ErrSignatureUnsupportedInput
}
