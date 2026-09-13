package validate

import (
	"errors"
	"go/ast"
	"go/token"
	"strings"

	"github.com/romshark/datapages/internal/subject"
)

var (
	ErrPageTypeNameInvalid     = errors.New("invalid page type name")
	ErrActionMethodNameInvalid = errors.New("invalid action method name")
	ErrEventTypeNameInvalid    = errors.New("invalid event type name")
	ErrEventCommMissing        = errors.New("missing event subject comment")
	ErrEventCommInvalid        = errors.New("invalid event subject comment syntax")
	ErrEventSubjectInvalid     = errors.New("invalid event subject")
	ErrEventHandlerNameInvalid = errors.New("invalid event handler method name")
	ErrSignalTagNameInvalid    = errors.New("invalid signal tag name")
	ErrSignalNameInvalid       = errors.New("invalid signal name")
	ErrSignalPathInvalid       = errors.New("invalid signal path")
	ErrRouteVarNameInvalid     = errors.New("invalid route variable name")
)

// RouteVarName validates a route wildcard name as a name generated code can
// give a function parameter. The generated href and action builders take one
// parameter per wildcard, named by the route.
//
// net/http accepts more than Go does: "{type}" is a keyword and "{_}" is the
// blank identifier, which no expression can read. Both leave a generated file
// that does not parse or does not compile, neither of which names the route
// the user has to fix.
func RouteVarName(name string) error {
	if name == "_" || !token.IsIdentifier(name) {
		return ErrRouteVarNameInvalid
	}
	return nil
}

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
//	PATCHX...
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
	case strings.HasPrefix(name, "PATCH"):
		if isValidActionSuffix(name, 5) {
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
// Rejects: missing quotes, empty, unterminated,
// or any token the broker subject rules refuse.
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
	payload := s[1 : len(s)-1]
	if len(payload) == 0 {
		return ErrEventSubjectInvalid
	}
	// The generator concatenates the declared subject into the subscription
	// and the publish path unchanged. A wildcard, a space or an empty token in
	// it is only rejected by the NATS server, which answers an illegal SUB by
	// closing the connection the whole process shares.
	for token := range strings.SplitSeq(payload, subject.Separator) {
		if !subject.IsToken(token) {
			return ErrEventSubjectInvalid
		}
	}
	return nil
}

// EventSubjectComment validates an event subject comment.
//
// Expected header (must be the first doc line):
//
//	// EventFoo is "foo.bar"
//
// Errors:
//   - ErrEventCommMissing: no doc comment.
//   - ErrEventCommInvalid: doc exists, but header is wrong.
//   - ErrEventSubjectInvalid: header ok, but quoted subject invalid.
func EventSubjectComment(typeName string, doc *ast.CommentGroup) error {
	if doc == nil || len(doc.List) == 0 {
		return ErrEventCommMissing
	}

	first := cleanLine(doc.List[0].Text)

	// Any existing doc comment must start with the exact header for this symbol.
	rest, ok := CutEventIsPrefix(first, typeName)
	if !ok {
		return ErrEventCommInvalid
	}

	if err := EventSubjectCommentSubject(rest); err != nil {
		return ErrEventSubjectInvalid
	}

	return nil
}

func cleanLine(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "//")
	return strings.TrimSpace(s)
}

// CutEventIsPrefix checks whether line starts with typeName followed by
// whitespace, "is", and more whitespace, and returns the remainder (the
// subject portion) plus true. Extra spaces or tabs between the parts are
// tolerated. Returns ("", false) when the prefix does not match.
func CutEventIsPrefix(line, typeName string) (rest string, ok bool) {
	s, ok := strings.CutPrefix(line, typeName)
	if !ok || len(s) == 0 {
		return "", false
	}
	// Must have at least one whitespace after the type name.
	if s[0] != ' ' && s[0] != '\t' {
		return "", false
	}
	s = strings.TrimLeft(s, " \t")
	s, ok = strings.CutPrefix(s, "is")
	if !ok || len(s) == 0 {
		return "", false
	}
	// Must have at least one whitespace after "is".
	if s[0] != ' ' && s[0] != '\t' {
		return "", false
	}
	return strings.TrimLeft(s, " \t"), true
}

// SignalTagName validates a signal:"..." tag value, which references one client
// signal by the steps on the way to it.
//
// It's the rule [SignalPath] carries: the stream reads the signal out of what
// the client sends, under the name the client declared it by, which makes the
// two the same rule. A camel case name reaches this tag as it's written,
// since the value of an attribute keeps its case where the name of one does not.
func SignalTagName(name string) error {
	if SignalPath(name) != nil {
		return ErrSignalTagNameInvalid
	}
	return nil
}

// SignalName validates the name of one Datastar signal, which is what a
// json:"..." tag of a signals struct declares.
//
// The name is written into a data-signals attribute name and read back as
// $name. An attribute name cannot be escaped, and $name is code rather than a
// string. Anything else breaks the page.
//
// Valid: [A-Za-z_][A-Za-z0-9_]*, a JavaScript identifier.
//
// A hyphen is refused. Datastar reads one as the boundary of a camel case name,
// which an HTML parser lowercases the attribute for:
// "data-signals:my-signal" is the signal mySignal
// (https://data-star.dev/reference/attributes#data-signals).
// [ReflectSignalPath] holds the upper bound that follows for a reflected signal,
// whose name the generator writes into such an attribute.
//
// A double underscore is refused wherever it stands:
// Datastar reads it as the delimiter of an attribute modifier
// (https://data-star.dev/reference/attributes#data-bind).
//
// A period is refused here. It stands between the steps of a signal path,
// and a signals struct writes a path by nesting a struct: {"foo":{"bar":1}} is
// the signal foo.bar, while json:"foo.bar" is one key that happens to carry
// a period and that no client sends. See [SignalPath].
func SignalName(name string) error {
	if name == "" {
		return ErrSignalNameInvalid
	}
	if !isSignalNameLetter(name[0]) {
		return ErrSignalNameInvalid
	}
	if strings.Contains(name, "__") {
		return ErrSignalNameInvalid
	}
	for i := 1; i < len(name); i++ {
		c := name[i]
		if isSignalNameLetter(c) || (c >= '0' && c <= '9') {
			continue
		}
		return ErrSignalNameInvalid
	}
	return nil
}

// SignalPath validates a reference to one signal, which is what a
// reflectsignal:"..." tag carries. A nested signal is referenced by the steps
// on the way to it, separated by periods: "foo.bar" is the signal bar of the
// signals struct nested under foo.
//
// Valid: period-separated [SignalName] segments.
func SignalPath(path string) error {
	if path == "" {
		return ErrSignalPathInvalid
	}
	for segment := range strings.SplitSeq(path, ".") {
		if SignalName(segment) != nil {
			return ErrSignalPathInvalid
		}
	}
	return nil
}

// ReflectSignalPath validates the reference a reflectsignal:"..." tag carries.
//
// It is [SignalPath] with every step starting lower case. The generator writes
// a reflected signal into an attribute name, which an HTML parser lowercases,
// and the upper case of a step survives that only as the hyphen Datastar reads
// it back from: mySignal goes out as my-signal. A step starting upper case has
// no hyphen before it.
//
// A signal the page never reflects is free of this: [SignalName] takes it
// however the page declares it.
func ReflectSignalPath(path string) error {
	if SignalPath(path) != nil {
		return ErrSignalPathInvalid
	}
	for segment := range strings.SplitSeq(path, ".") {
		if c := segment[0]; c >= 'A' && c <= 'Z' {
			return ErrSignalPathInvalid
		}
	}
	return nil
}

// isSignalNameLetter reports whether c may stand in
// a signal name after the first character.
func isSignalNameLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
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
