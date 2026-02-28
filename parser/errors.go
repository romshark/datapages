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

	"github.com/romshark/datapages/parser/internal/paramvalidation"
	"github.com/romshark/datapages/parser/internal/structtag"

	"golang.org/x/tools/go/packages"
)

var (
	ErrAppMissingTypeApp        = errors.New(`missing required type "App"`)
	ErrAppMissingPageIndex      = errors.New(`missing required page type "PageIndex"`)
	ErrSignatureMissingReq      = errors.New(`missing the *http.Request parameter`)
	ErrSignatureMultiErrRet     = errors.New(`multiple error return values`)
	ErrSignatureUnknownInput    = errors.New(`handler has unknown input parameter type`)
	ErrSignatureSecondArgNotSSE = errors.New(
		"event handler second argument must be *datastar.ServerSentEventGenerator",
	)
	ErrSignatureEvHandReturnMustBeError = errors.New(
		"event handler must return only error",
	)
	ErrSignatureEvHandFirstArgNotEvent = errors.New(
		`event handler first argument must be named "event"`,
	)
	ErrSignatureEvHandFirstArgTypeNotEvent = errors.New(
		"event handler first argument type must be an event type",
	)
	ErrSignatureGETMissingBody = errors.New(
		"GET handler must return body templ.Component",
	)
	ErrSignatureGETBodyWrongName = errors.New(
		"GET handler first templ.Component return must be named \"body\"",
	)
	ErrSignatureGETHeadWrongName = errors.New(
		"GET handler second templ.Component return must be named \"head\"",
	)

	ErrPageMissingFieldApp     = errors.New(`page is missing the "App *App" field`)
	ErrPageHasExtraFields      = errors.New(`page struct has unsupported fields`)
	ErrPageMissingGET          = errors.New(`page is missing the GET handler`)
	ErrPageConflictingGETEmbed = errors.New("conflicting GET handlers in embedded")
	ErrPageNameInvalid         = errors.New("page has invalid name")
	ErrPageMissingPathComm     = errors.New("page is missing path comment")
	ErrPageInvalidPathComm     = errors.New("page has invalid path comment")

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

	ErrEventFieldUnexported  = errors.New("event field must be exported")
	ErrEventFieldMissingTag  = errors.New("event field must have json tag")
	ErrEventFieldDuplicateTag = errors.New("event field has duplicate json tag value")

	ErrPathParamNotStruct    = paramvalidation.ErrPathParamNotStruct
	ErrPathFieldUnexported   = paramvalidation.ErrPathFieldUnexported
	ErrPathFieldMissingTag   = paramvalidation.ErrPathFieldMissingTag
	ErrPathFieldNotString    = paramvalidation.ErrPathFieldNotString
	ErrPathFieldNotInRoute   = paramvalidation.ErrPathFieldNotInRoute
	ErrPathMissingRouteVar   = paramvalidation.ErrPathMissingRouteVar
	ErrPathFieldDuplicateTag = paramvalidation.ErrPathFieldDuplicateTag

	ErrQueryParamNotStruct    = paramvalidation.ErrQueryParamNotStruct
	ErrQueryFieldUnexported   = paramvalidation.ErrQueryFieldUnexported
	ErrQueryFieldMissingTag   = paramvalidation.ErrQueryFieldMissingTag
	ErrQueryFieldDuplicateTag = paramvalidation.ErrQueryFieldDuplicateTag

	ErrQueryReflectSignalNotInSignals = structtag.ErrQueryReflectSignalNotInSignals

	ErrSignalsParamNotStruct    = paramvalidation.ErrSignalsParamNotStruct
	ErrSignalsFieldUnexported   = paramvalidation.ErrSignalsFieldUnexported
	ErrSignalsFieldMissingTag   = paramvalidation.ErrSignalsFieldMissingTag
	ErrSignalsFieldDuplicateTag = paramvalidation.ErrSignalsFieldDuplicateTag

	ErrDispatchParamNotFunc    = paramvalidation.ErrDispatchParamNotFunc
	ErrDispatchReturnCount     = paramvalidation.ErrDispatchReturnCount
	ErrDispatchMustReturnError = paramvalidation.ErrDispatchMustReturnError
	ErrDispatchNoParams        = paramvalidation.ErrDispatchNoParams
	ErrDispatchParamNotEvent   = paramvalidation.ErrDispatchParamNotEvent

	ErrSessionNotStruct     = errors.New("session type must be a struct")
	ErrSessionMissingUserID = errors.New(
		"session type must have a UserID string field",
	)
	ErrSessionMissingIssuedAt = errors.New(
		"session type must have an IssuedAt time.Time field",
	)
	ErrSessionParamNotSessionType = errors.New("session parameter type must be Session")
	ErrSessionTokenParamNotString = errors.New(
		"sessionToken parameter must be of type string",
	)

	ErrRedirectNotString             = errors.New("redirect must be a string")
	ErrRedirectStatusNotInt          = errors.New("redirectStatus must be an int")
	ErrRedirectStatusWithoutRedirect = errors.New("redirectStatus requires redirect")

	ErrNewSessionNotSessionType = errors.New("newSession must be of type Session")
	ErrCloseSessionNotBool      = errors.New("closeSession must be of type bool")
	ErrNewSessionWithSSE        = errors.New(
		"newSession cannot be used together with sse parameter",
	)
	ErrCloseSessionWithSSE = errors.New(
		"closeSession cannot be used together with sse parameter",
	)

	ErrEnableBgStreamNotBool = errors.New(
		"enableBackgroundStreaming must be of type bool",
	)
	ErrEnableBgStreamNotGET = errors.New(
		"enableBackgroundStreaming can only be used in GET handlers",
	)
	ErrDisableRefreshNotBool = errors.New(
		"disableRefreshAfterHidden must be of type bool",
	)
	ErrDisableRefreshNotGET = errors.New(
		"disableRefreshAfterHidden can only be used in GET handlers",
	)
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

func cleanPath(p string) string {
	if p == "/" {
		return p
	}
	return strings.TrimRight(p, "/")
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
	return fmt.Sprintf("%v: field %s in %s", ErrEventFieldMissingTag, e.FieldName, e.TypeName)
}

func (e *ErrorEventFieldMissingTag) Unwrap() error { return ErrEventFieldMissingTag }

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
