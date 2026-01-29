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
)

var (
	ErrAppMissingTypeApp    = errors.New(`missing required type "App"`)
	ErrAppMissingPageIndex  = errors.New(`missing required page type "PageIndex"`)
	ErrSignatureMissingReq  = errors.New(`missing the *http.Request parameter`)
	ErrSignatureMultiErrRet = errors.New(`multiple error return values`)
	ErrPageMissingFieldApp  = errors.New(`page is missing the "App *App" field`)
	ErrPageHasExtraFields   = errors.New(`page struct has unsupported fields`)
	ErrPageMissingGET       = errors.New(`page is missing the GET handler`)
	ErrPageNameInvalid      = errors.New("page has invalid name")
	ErrPageMissingPathComm  = errors.New("page is missing path comment")
	ErrPageInvalidPathComm  = errors.New("page has invalid path comment")

	ErrActionNameInvalid      = errors.New("action has invalid name")
	ErrActionMissingPathComm  = errors.New("action handler is missing path comment")
	ErrActionInvalidPathComm  = errors.New("action handler has invalid path comment")
	ErrActionPathNotUnderPage = errors.New("action handler path is not under page path")

	ErrEventCommMissing           = errors.New("event type is missing subject comment")
	ErrEventCommInvalid           = errors.New("event type has invalid subject comment")
	ErrEventSubjectInvalid        = errors.New("event subject is invalid")
	ErrEvHandFirstArgNotEvent     = errors.New(`event handler first argument must be named "event"`)
	ErrEvHandFirstArgTypeNotEvent = errors.New("event handler first argument type must be an event type")
	ErrEvHandDuplicate            = errors.New("duplicate event handler for event")
	ErrEvHandReturnMustBeError    = errors.New("event handler must return only error")

	ErrEventFieldUnexported = errors.New("event field must be exported")
	ErrEventFieldMissingTag = errors.New("event field must have json tag")
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

// isUnderPage reports whether action is under page.
// Rules:
//   - page must be prefix of action
//   - boundary: either page=="/" OR next char after prefix is '/'
//   - disallow exact equality (action == page) to avoid colliding with GET route
func isUnderPage(page, action string) bool {
	page = cleanPath(page)
	action = cleanPath(action)

	if page == "" || action == "" {
		return false
	}
	if page == "/" {
		return strings.HasPrefix(action, "/")
	}
	if !strings.HasPrefix(action, page) {
		return false
	}
	if len(action) == len(page) {
		return false // disallow exact match
	}
	return action[len(page)] == '/'
}
