package validate

import (
	"errors"
	"go/ast"
	"strings"
)

var (
	ErrPageTypeNameInvalid           = errors.New("invalid page type name")
	ErrActionMethodNameInvalid       = errors.New("invalid action method name")
	ErrEventTypeNameInvalid          = errors.New("invalid event type name")
	ErrEventSubjectMissing           = errors.New("missing event subject comment")
	ErrEventSubjectInvalidSyntax     = errors.New("invalid event subject comment syntax")
	ErrEventSubjectInvalid           = errors.New("invalid event subject")
	ErrEventHandlerMethodNameInvalid = errors.New("invalid event handler method name")
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

// EventSubjectComment validates event subject comments in the exact form:
//
//	// EventFoo is "foo.bar"
//
// Returns nil if valid, otherwise a sentinel error:
//   - ErrEventSubjectMissing (no matching subject line found)
//   - ErrEventSubjectInvalidSyntax (matching attempt exists but wrong syntax)
//   - ErrEventSubjectInvalid (matching syntax but invalid subject payload)
func EventSubjectComment(typeName string, doc *ast.CommentGroup) error {
	if doc == nil {
		return ErrEventSubjectMissing
	}
	prefix := typeName + " "
	want := typeName + " is "

	for _, c := range doc.List {
		txt := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))

		if !strings.HasPrefix(txt, prefix) {
			continue
		}
		// Attempt exists for this symbol.
		if !strings.HasPrefix(txt, want) {
			return ErrEventSubjectInvalidSyntax
		}
		rest := strings.TrimSpace(strings.TrimPrefix(txt, want))
		if err := EventSubjectCommentSubject(rest); err != nil {
			return ErrEventSubjectInvalid
		}
		return nil
	}

	return ErrEventSubjectMissing
}

// EventHandlerMethodName validates event handler method names:
// "On" + Uppercase letter + [A-Za-z0-9]*.
func EventHandlerMethodName(name string) error {
	s, ok := strings.CutPrefix(name, "On")
	if !ok || s == "" {
		return ErrEventHandlerMethodNameInvalid
	}
	c0 := s[0]
	if c0 < 'A' || c0 > 'Z' {
		return ErrEventHandlerMethodNameInvalid
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') ||
			(c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') {
			continue
		}
		return ErrEventHandlerMethodNameInvalid
	}
	return nil
}
