package validate

import (
	"errors"
	"go/ast"
	"strings"
)

var (
	ErrPageTypeNameInvalid     = errors.New("invalid page type name")
	ErrActionMethodNameInvalid = errors.New("invalid action method name")
	ErrEventTypeNameInvalid    = errors.New("invalid event type name")
	ErrEventCommMissing        = errors.New("missing event subject comment")
	ErrEventCommInvalid        = errors.New("invalid event subject comment syntax")
	ErrEventSubjectInvalid     = errors.New("invalid event subject")
	ErrEventHandlerNameInvalid = errors.New("invalid event handler method name")
)

// PageTypeName validates page type names: "Page" + Uppercase letter + [A-Za-z0-9]*.
func PageTypeName(name string) error {
	s, ok := strings.CutPrefix(name, "Page")
	if !ok || s == "" {
		return ErrPageTypeNameInvalid
	}
	r0 := s[0]
	if r0 < 'A' || r0 > 'Z' {
		return ErrPageTypeNameInvalid
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') ||
			(c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') {
			continue
		}
		return ErrPageTypeNameInvalid
	}
	return nil
}

// ActionMethodName validates action handler method names:
//
//	POSTX...
//	PUTX...
//	DELETEX...
//
// where X is [A-Z], followed by [A-Za-z0-9]*.
func ActionMethodName(name string) error {
	isValidActionSuffix := func(name string, prefixLen int) bool {
		if len(name) <= prefixLen {
			return false
		}
		s := name[prefixLen:]
		c0 := s[0]
		if c0 < 'A' || c0 > 'Z' {
			return false
		}
		for i := 1; i < len(s); i++ {
			c := s[i]
			if (c >= 'A' && c <= 'Z') ||
				(c >= 'a' && c <= 'z') ||
				(c >= '0' && c <= '9') {
				continue
			}
			return false
		}
		return true
	}

	switch {
	case strings.HasPrefix(name, "POST"):
		if isValidActionSuffix(name, 4) {
			return nil
		}
	case strings.HasPrefix(name, "PUT"):
		if isValidActionSuffix(name, 3) {
			return nil
		}
	case strings.HasPrefix(name, "DELETE"):
		if isValidActionSuffix(name, 6) {
			return nil
		}
	}
	return ErrActionMethodNameInvalid
}

// EventTypeName validates event type names: "Event" + Uppercase letter + [A-Za-z0-9]*.
func EventTypeName(name string) error {
	s, ok := strings.CutPrefix(name, "Event")
	if !ok || s == "" {
		return ErrEventTypeNameInvalid
	}
	r0 := s[0]
	if r0 < 'A' || r0 > 'Z' {
		return ErrEventTypeNameInvalid
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') ||
			(c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') {
			continue
		}
		return ErrEventTypeNameInvalid
	}
	return nil
}

// EventSubjectCommentSubject validates the raw subject comment payload for an event.
// Accepts: `"foo.bar"`.
// Rejects: missing quotes, empty, or unterminated.
func EventSubjectCommentSubject(s string) error {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return ErrEventSubjectInvalid
	}
	if s[0] != '"' {
		return ErrEventSubjectInvalid
	}
	if s[len(s)-1] != '"' {
		return ErrEventSubjectInvalid
	}
	if len(s[1:len(s)-1]) == 0 {
		return ErrEventSubjectInvalid
	}
	return nil
}

// EventSubjectComment validates an event subject comment.
//
// Expected header (must be the first doc line):
//
//	// EventFoo is "foo.bar"
//
// If there are more doc lines, an empty comment line is mandatory:
//
//	// EventFoo is "foo.bar"
//	//
//	// description...
//
// Errors:
//   - ErrEventCommMissing: no doc comment.
//   - ErrEventCommInvalid: doc exists, but header/spacing is wrong.
//   - ErrEventSubjectInvalid: header ok, but quoted subject invalid.
func EventSubjectComment(typeName string, doc *ast.CommentGroup) error {
	if doc == nil || len(doc.List) == 0 {
		return ErrEventCommMissing
	}

	first := cleanLine(doc.List[0].Text)
	want := typeName + ` is `

	// Any existing doc comment must start with the exact header for this symbol.
	if !strings.HasPrefix(first, want) {
		return ErrEventCommInvalid
	}

	rest := strings.TrimSpace(strings.TrimPrefix(first, want))
	if err := EventSubjectCommentSubject(rest); err != nil {
		return ErrEventSubjectInvalid
	}

	// Mandatory empty comment line between header and description.
	if len(doc.List) > 1 {
		second := cleanLine(doc.List[1].Text)
		if second != "" {
			return ErrEventCommInvalid
		}
	}

	return nil
}

func cleanLine(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "//")
	return strings.TrimSpace(s)
}

// EventHandlerMethodName validates event handler method names:
// "On" + Uppercase letter + [A-Za-z0-9]*.
func EventHandlerMethodName(name string) error {
	s, ok := strings.CutPrefix(name, "On")
	if !ok || s == "" {
		return ErrEventHandlerNameInvalid
	}
	c0 := s[0]
	if c0 < 'A' || c0 > 'Z' {
		return ErrEventHandlerNameInvalid
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') ||
			(c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') {
			continue
		}
		return ErrEventHandlerNameInvalid
	}
	return nil
}
