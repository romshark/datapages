package datapagestest

import (
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// actionPattern matches a generated action call and the JavaScript string of
// its URL, whose escapes carry a quote of a route.
var actionPattern = regexp.MustCompile(`@(get|post|put|patch|delete|query)\('((?:[^'\\]|\\.)*)'`)

// call is a generated action call.
type call struct {
	method, path string
	form         bool              // the call sets the form content type
	headers      map[string]string // the headers the call sets
	payload      bool              // the call sets a payload, which the browser evaluates
	// include and exclude are the filterSignals patterns of the call, nil when unset.
	include, exclude *regexp.Regexp
}

var (
	formContent = regexp.MustCompile(`\bcontentType\s*:\s*['"]form['"]`)
	payloadKey  = regexp.MustCompile(`\bpayload\s*:`)
	headersKey  = regexp.MustCompile(`\bheaders\s*:\s*\{`)
	headerPair  = regexp.MustCompile(`'((?:[^'\\]|\\.)*)'\s*:\s*'((?:[^'\\]|\\.)*)'`)
	filterKey   = regexp.MustCompile(`\bfilterSignals\s*:\s*\{`)
	patternKey  = regexp.MustCompile(`\b(include|exclude)\s*:\s*/`)
)

// parseCall reads the method, URL and options of the generated action expr calls.
// It fails unless expr calls exactly 1, and on an invalid URL or signal filter.
func parseCall(t testing.TB, expr string) call {
	t.Helper()
	if n := len(actionPattern.FindAllStringIndex(expr, -1)); n != 1 {
		t.Fatalf("%q calls %d generated actions, want 1", expr, n)
	}
	m := actionPattern.FindStringSubmatchIndex(expr)
	c := call{
		method: strings.ToUpper(expr[m[2]:m[3]]),
		path:   unescapeJS(expr[m[4]:m[5]]),
	}
	if _, err := url.Parse(c.path); err != nil {
		t.Fatalf("%q calls an action on an unparsable URL: %v", expr, err)
	}
	opts := callOptions(expr[m[1]:])
	c.form = formContent.MatchString(opts)
	c.payload = payloadKey.MatchString(opts)
	if loc := filterKey.FindStringIndex(opts); loc != nil {
		filter := balanced(opts[loc[1]-1:])
		for _, p := range patternKey.FindAllStringSubmatchIndex(filter, -1) {
			start := p[1] - 1
			end := closing(filter, start, '/')
			if end < 0 {
				t.Fatalf("%q: unterminated filterSignals pattern", expr)
			}
			flags := end + 1
			for flags < len(filter) && 'a' <= filter[flags] && filter[flags] <= 'z' {
				flags++
			}
			re, err := jsRegexp(filter[start+1:end], filter[end+1:flags])
			if err != nil {
				t.Fatalf("%q: filterSignals %s: %v; pass the signals the call sends",
					expr, filter[p[2]:p[3]], err)
			}
			if filter[p[2]:p[3]] == "include" {
				c.include = re
			} else {
				c.exclude = re
			}
		}
	}
	if loc := headersKey.FindStringIndex(opts); loc != nil {
		headers := balanced(opts[loc[1]-1:])
		for _, p := range headerPair.FindAllStringSubmatch(headers, -1) {
			if c.headers == nil {
				c.headers = map[string]string{}
			}
			c.headers[unescapeJS(p[1])] = unescapeJS(p[2])
		}
	}
	return c
}

// jsRegexp compiles a JavaScript regex literal's pattern and flags. Flags other
// than i, m and s make no difference to a match test.
func jsRegexp(pattern, flags string) (*regexp.Regexp, error) {
	var set strings.Builder
	for _, f := range flags {
		if f == 'i' || f == 'm' || f == 's' {
			set.WriteRune(f)
		}
	}
	if set.Len() > 0 {
		pattern = "(?" + set.String() + ")" + pattern
	}
	return regexp.Compile(pattern)
}

// callOptions returns the object literal of a call's options from rest, the text
// after the call's URL, or "" when the call has none.
func callOptions(rest string) string {
	rest = strings.TrimLeft(rest, " \t\r\n")
	comma, ok := strings.CutPrefix(rest, ",")
	if !ok {
		return ""
	}
	return balanced(strings.TrimLeft(comma, " \t\r\n"))
}

// balanced returns the object literal at the start of s through its closing
// brace, skipping strings and regex literals, or "" when s holds none.
func balanced(s string) string {
	if !strings.HasPrefix(s, "{") {
		return ""
	}
	depth := 0
	var prev byte // the last byte that is not a space
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'' || c == '"' || c == '`':
			end := closing(s, i, c)
			if end < 0 {
				return ""
			}
			i = end
		case c == '/' && strings.IndexByte(":,{([", prev) >= 0:
			end := closing(s, i, '/')
			if end < 0 {
				return ""
			}
			i = end
		case c == '{':
			depth++
		case c == '}':
			if depth--; depth == 0 {
				return s[:i+1]
			}
		}
		if strings.IndexByte(" \t\r\n", s[i]) < 0 {
			prev = s[i]
		}
	}
	return ""
}

// closing returns the index of the delimiter that ends the string or regex
// literal starting at i, or -1.
func closing(s string, i int, delim byte) int {
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case delim:
			return j
		}
	}
	return -1
}

// unescapeJS decodes the escapes the action package writes into a JavaScript
// string: \\, \', \n and \r.
func unescapeJS(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
