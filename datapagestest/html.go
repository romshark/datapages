package datapagestest

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// parseHTML ignores the impossible read error from strings.Reader.
func parseHTML(markup string) *html.Node {
	doc, _ := html.Parse(strings.NewReader(markup))
	return doc
}

// ElementByID returns the first element with id under root, including root itself.
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

// Text returns n's text with whitespace collapsed and trimmed.
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

// Signals decodes the page's data-signals attributes using Datastar's dot nesting,
// __case and __ifmissing rules. Later attributes override earlier ones.
//
// It accepts JSON and generator-written single-quoted strings.
// Expressions requiring a browser fail the test.
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
					t.Fatalf("%s: %s=%q is not a JSON object: %v",
						r.desc, a.Key, a.Val, err)
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

// PublicSignals filters [Response.Signals] as Datastar does by default. It removes
// paths containing an underscore-prefixed name and objects left empty by filtering.
func (r *Response) PublicSignals(t testing.TB) map[string]any {
	t.Helper()
	return publicSignals(r.Signals(t))
}

func publicSignals(m map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		if strings.HasPrefix(k, "_") {
			continue
		}
		if obj, ok := v.(map[string]any); ok {
			if obj = publicSignals(obj); len(obj) == 0 {
				continue
			}
			v = obj
		}
		out[k] = v
	}
	return out
}

// splitModifiers separates a Datastar attribute name from its modifiers and tags.
func splitModifiers(key string) (string, map[string][]string) {
	parts := strings.Split(key, "__")
	mods := map[string][]string{}
	for _, m := range parts[1:] {
		name, tags, _ := strings.Cut(m, ".")
		mods[name] = append([]string{}, strings.Split(tags, ".")...)
	}
	return parts[0], mods
}

// convertCase applies __case tags in order, defaults to camel and ignores unknown tags.
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

// setSignal creates parent maps for a dotted path.
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

// mergeSignals recursively merges a data-signals object.
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
	"not a literal; datapagestest decodes only JSON and the single-quoted strings " +
		"the generator writes",
)

// literal decodes a literal data-signals value.
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

// singleQuoted decodes the single-quoted strings written by
// [github.com/romshark/datapages/runtime/htmlattr.SignalString].
// It rejects other escapes and expressions.
func singleQuoted(s string) (string, bool) {
	if len(s) < 2 || s[0] != '\'' || s[len(s)-1] != '\'' {
		return "", false
	}
	s = s[1 : len(s)-1]
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\'':
			return "", false
		case '\\':
			i++
			if i == len(s) {
				return "", false
			}
			switch s[i] {
			case '\\', '\'':
				b.WriteByte(s[i])
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			default:
				return "", false
			}
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), true
}
