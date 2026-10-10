package datapagestest

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// parseHTML parses markup as a document. html.Parse fails only on a read error,
// which a strings.Reader never returns.
func parseHTML(markup string) *html.Node {
	doc, _ := html.Parse(strings.NewReader(markup))
	return doc
}

// ElementByID returns the first element with id in the tree under root, root
// included, or nil when there is none.
func ElementByID(root *html.Node, id string) *html.Node {
	if hasID(root, id) {
		return root
	}
	for n := range root.Descendants() {
		if hasID(n, id) {
			return n
		}
	}
	return nil
}

func hasID(n *html.Node, id string) bool {
	if n.Type != html.ElementNode {
		return false
	}
	v, ok := Attr(n, "id")
	return ok && v == id
}

// Text returns the text content of n with each run of whitespace collapsed to one
// space and none at either end.
func Text(n *html.Node) string {
	var b strings.Builder
	if n.Type == html.TextNode {
		b.WriteString(n.Data)
	}
	for d := range n.Descendants() {
		if d.Type == html.TextNode {
			b.WriteString(d.Data)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// Attr returns the value of the attribute key of n and whether n has it.
// A boolean attribute such as disabled has an empty value.
func Attr(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

// Signals returns the signals the page's data-signals attributes declare, named the
// way Datastar names them: data-signals:new-title sets newTitle, a dot nests, the
// __case modifier changes the conversion and __ifmissing keeps a value set before.
// Later attributes override earlier ones.
//
// It decodes literal values only: JSON, and the single-quoted JavaScript strings the
// generator writes. It fails the test on any other expression, whose value only
// Datastar in a browser can compute.
func (r *Response) Signals(t testing.TB) map[string]any {
	t.Helper()
	out := map[string]any{}
	for n := range r.Document().Descendants() {
		if n.Type != html.ElementNode {
			continue
		}
		for _, a := range n.Attr {
			key, ok := strings.CutPrefix(a.Key, "data-signals")
			if !ok || (key != "" && key[0] != ':' && !strings.HasPrefix(key, "__")) {
				continue
			}
			name, mods := splitModifiers(strings.TrimPrefix(key, ":"))
			ifMissing := mods["ifmissing"] != nil
			if name == "" {
				var m map[string]any
				if err := json.Unmarshal([]byte(a.Val), &m); err != nil {
					t.Fatalf("%s: %s=%q is not a JSON object: %v", r.desc, a.Key, a.Val, err)
				}
				mergeSignals(out, m, ifMissing)
				continue
			}
			v, err := literal(a.Val)
			if err != nil {
				t.Fatalf("%s: %s=%q: %v", r.desc, a.Key, a.Val, err)
			}
			setSignal(out, convertCase(name, mods["case"]), v, ifMissing)
		}
	}
	return out
}

// splitModifiers splits an attribute key such as "new-title__case.kebab" into its name
// and its modifiers with their tags, the way Datastar does.
func splitModifiers(key string) (string, map[string][]string) {
	parts := strings.Split(key, "__")
	mods := map[string][]string{}
	for _, m := range parts[1:] {
		name, tags, _ := strings.Cut(m, ".")
		mods[name] = append([]string{}, strings.Split(tags, ".")...)
	}
	return parts[0], mods
}

// convertCase converts an attribute key to a signal name. Datastar applies each tag of
// __case in turn, camel when there is none, and leaves the key as it is for a tag it
// doesn't know, such as kebab.
func convertCase(name string, tags []string) string {
	if len(tags) == 0 {
		tags = []string{"camel"}
	}
	for _, tag := range tags {
		switch tag {
		case "camel":
			name = camel(name)
		case "snake":
			name = strings.ReplaceAll(name, "-", "_")
		case "pascal":
			if name != "" {
				name = strings.ToUpper(name[:1]) + camel(name[1:])
			}
		}
	}
	return name
}

// camel replaces each hyphen before a lowercase letter with that letter in uppercase.
func camel(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '-' && i+1 < len(s) && 'a' <= s[i+1] && s[i+1] <= 'z' {
			b.WriteByte(s[i+1] - 'a' + 'A')
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// setSignal sets the signal at a dotted path, creating the objects it nests in.
func setSignal(out map[string]any, path string, v any, ifMissing bool) {
	keys := strings.Split(path, ".")
	for _, k := range keys[:len(keys)-1] {
		next, ok := out[k].(map[string]any)
		if !ok {
			next = map[string]any{}
			out[k] = next
		}
		out = next
	}
	last := keys[len(keys)-1]
	if _, exists := out[last]; exists && ifMissing {
		return
	}
	out[last] = v
}

// mergeSignals merges the signals of a data-signals object into out, object by object.
func mergeSignals(out, m map[string]any, ifMissing bool) {
	for k, v := range m {
		if vm, ok := v.(map[string]any); ok {
			if om, ok := out[k].(map[string]any); ok {
				mergeSignals(om, vm, ifMissing)
				continue
			}
		}
		if _, exists := out[k]; exists && ifMissing {
			continue
		}
		out[k] = v
	}
}

var errNotLiteral = errors.New(
	"not a literal; datapagestest decodes JSON and single-quoted strings only",
)

// literal decodes a data-signals value that is a literal.
func literal(expr string) (any, error) {
	s := strings.TrimSpace(expr)
	var v any
	if err := json.Unmarshal([]byte(s), &v); err == nil {
		return v, nil
	}
	if str, ok := singleQuoted(s); ok {
		return str, nil
	}
	return nil, errNotLiteral
}

// singleQuoted decodes a JavaScript string literal in single quotes.
// An unescaped quote inside means the value is an expression, such as 'a' + 'b'.
func singleQuoted(s string) (string, bool) {
	if len(s) < 2 || s[0] != '\'' || s[len(s)-1] != '\'' {
		return "", false
	}
	s = s[1 : len(s)-1]
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\'':
			return "", false
		case '\\':
		default:
			b.WriteByte(c)
			continue
		}
		i++
		if i == len(s) {
			return "", false
		}
		switch s[i] {
		case '\'', '"', '\\', '/':
			b.WriteByte(s[i])
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'u':
			r, n, ok := unicodeEscape(s[i+1:])
			if !ok {
				return "", false
			}
			b.WriteRune(r)
			i += n
		default:
			return "", false
		}
	}
	return b.String(), true
}

// unicodeEscape decodes the hex digits after \u, joining a surrogate pair written as
// two escapes. It returns the rune and how many bytes of s it read.
func unicodeEscape(s string) (rune, int, bool) {
	hex4 := func(s string) (rune, bool) {
		if len(s) < 4 {
			return 0, false
		}
		v, err := strconv.ParseUint(s[:4], 16, 16)
		return rune(v), err == nil
	}
	r, ok := hex4(s)
	if !ok {
		return 0, 0, false
	}
	if !utf16.IsSurrogate(r) {
		return r, 4, true
	}
	if len(s) < 10 || s[4:6] != `\u` {
		return utf8.RuneError, 4, true
	}
	lo, ok := hex4(s[6:])
	if !ok {
		return 0, 0, false
	}
	return utf16.DecodeRune(r, lo), 10, true
}
