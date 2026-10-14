package datapagestest

import (
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// Generated action URLs are percent-encoded. They contain no quote or backslash.
var actionPattern = regexp.MustCompile(`@(get|post|put|patch|delete|query)\('([^'\\]*)'`)

// ParseAction returns the method and URL of the first generated action in expr,
// ready for [Tab.Do]. It fails if expr has no action or its URL is invalid.
func ParseAction(t testing.TB, expr string) (method, path string) {
	t.Helper()
	m := actionPattern.FindStringSubmatch(expr)
	if m == nil {
		t.Fatalf("%q calls no action", expr)
	}
	if _, err := url.Parse(m[2]); err != nil {
		t.Fatalf("%q calls an action on an unparsable URL: %v", expr, err)
	}
	return strings.ToUpper(m[1]), m[2]
}
