package datapagestest

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// page is a document with the signals that its data-bind and data-show attributes read.
// A tab keeps 1 page and applies every event it receives to it.
type page struct {
	doc *html.Node
	// signals is nil when they could not be decoded, and sigErr says why.
	signals map[string]any
	sigErr  error
	// computed maps the names of data-computed signals to their expressions.
	computed map[string]string
}

// newPage parses markup as a document and decodes its signals.
// A failure to decode the signals is reported when they are read.
func newPage(markup string) *page {
	p := &page{doc: parseHTML(markup)}
	p.signals, p.sigErr = decodeSignals(p.doc)
	if p.signals != nil {
		p.bindDefaults(p.doc)
	}
	return p
}

// bindDefaults creates the signals that the data-bind elements under root name
// and that do not exist yet, from the elements' values, as Datastar does.
func (p *page) bindDefaults(root *html.Node) {
	for n := range withDescendants(root) {
		if n.Type != html.ElementNode {
			continue
		}
		path, ok := bindPath(n)
		if !ok || path == "" {
			continue
		}
		if _, ok := getSignal(p.signals, path); ok {
			continue
		}
		var v any = p.value(n)
		if n.Data == "input" {
			switch inputType(n) {
			case "checkbox":
				_, checked := Attr(n, "checked")
				v = checked
			case "radio":
				if _, checked := Attr(n, "checked"); !checked {
					continue
				}
			}
		}
		setSignal(p.signals, path, v, true)
	}
}

// withDescendants yields n and its descendants.
func withDescendants(n *html.Node) func(func(*html.Node) bool) {
	return func(yield func(*html.Node) bool) {
		if !yield(n) {
			return
		}
		for d := range n.Descendants() {
			if !yield(d) {
				return
			}
		}
	}
}

// signal returns the signal at path, failing when the signals are unknown.
func (p *page) signal(path string) (any, error) {
	if p.signals == nil {
		return nil, p.sigErr
	}
	v, _ := getSignal(p.signals, path)
	return v, nil
}

// getSignal returns the value at the dotted path in m.
func getSignal(m map[string]any, path string) (any, bool) {
	keys := strings.Split(path, ".")
	for _, k := range keys[:len(keys)-1] {
		next, ok := m[k].(map[string]any)
		if !ok {
			return nil, false
		}
		m = next
	}
	v, ok := m[keys[len(keys)-1]]
	return v, ok
}

// signalString renders a signal as a form field or a control's value.
func signalString(v any) string {
	switch v := v.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// normalize converts v to the types encoding/json decodes into an any.
func normalize(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	err = json.Unmarshal(b, &out)
	return out, err
}

// apply applies a Datastar event as the browser does. It returns a warning
// for a patch that Datastar skips and an error for an event it cannot apply.
func (p *page) apply(ev Event) (warning string, err error) {
	switch ev.Type {
	case PatchSignals:
		return "", p.patchSignals(ev)
	case PatchElements:
		return p.patchElements(ev)
	}
	return "", nil
}

func (p *page) patchSignals(ev Event) error {
	var m map[string]any
	if err := json.Unmarshal([]byte(ev.Data["signals"]), &m); err != nil {
		return fmt.Errorf("decoding the signals: %w", err)
	}
	if p.signals == nil {
		return nil
	}
	mergePatch(p.signals, m, ev.Data["onlyIfMissing"] == "true")
	return nil
}

// mergePatch applies a JSON merge patch (RFC 7386), as Datastar applies a signal patch.
// With ifMissing it only adds what is missing.
func mergePatch(out, patch map[string]any, ifMissing bool) {
	for k, v := range patch {
		if v == nil {
			if !ifMissing {
				delete(out, k)
			}
			continue
		}
		if vm, ok := v.(map[string]any); ok {
			om, ok := out[k].(map[string]any)
			if !ok {
				if _, exists := out[k]; exists && ifMissing {
					continue
				}
				om = map[string]any{}
				out[k] = om
			}
			mergePatch(om, vm, ifMissing)
			continue
		}
		if _, exists := out[k]; exists && ifMissing {
			continue
		}
		out[k] = v
	}
}

// errNoSelector mirrors Datastar, which needs a selector for a mode that does
// not replace the elements it receives.
var errNoSelector = errors.New("the mode needs a selector")

func (p *page) patchElements(ev Event) (string, error) {
	mode := ev.Data["mode"]
	if mode == "" {
		mode = "outer"
	}
	switch mode {
	case "outer", "inner", "replace", "prepend", "append", "before", "after",
		"remove":
	default:
		return "", fmt.Errorf("unknown mode %q", mode)
	}
	sel := ev.Data["selector"]
	nodes, err := fragment(ev.Data["elements"])
	if err != nil {
		return "", err
	}
	if sel == "" {
		if mode != "outer" && mode != "replace" && mode != "remove" {
			return "", fmt.Errorf("mode %s: %w", mode, errNoSelector)
		}
		var missing []string
		for _, n := range nodes {
			if n.Type != html.ElementNode {
				continue
			}
			id := attr(n, "id")
			if id == "" {
				return "", fmt.Errorf("%s has no id and the event no selector",
					describeNode(n))
			}
			target := ElementByID(p.doc, id)
			if target == nil {
				missing = append(missing, "#"+id)
				continue
			}
			p.patch(mode, target, []*html.Node{n})
		}
		if missing != nil {
			return fmt.Sprintf("no element matches %s; Datastar skips it",
				strings.Join(missing, ", ")), nil
		}
		return "", nil
	}
	targets, err := p.selectAll(sel)
	if err != nil {
		return "", err
	}
	if len(targets) == 0 {
		return fmt.Sprintf("no element matches %s; Datastar skips it", sel), nil
	}
	for i, target := range targets {
		ns := nodes
		if i > 0 {
			ns = cloneNodes(nodes)
		}
		p.patch(mode, target, ns)
	}
	return "", nil
}

// patch applies 1 mode to target. Morphing keeps elements marked data-ignore-morph,
// and the signals of data-signals attributes that the patch added or changed apply.
func (p *page) patch(mode string, target *html.Node, nodes []*html.Node) {
	parent := target.Parent
	var old map[string]map[string]string
	if mode == "outer" || mode == "inner" {
		old = signalAttrsByID(target)
	}
	switch mode {
	case "remove":
		if parent != nil {
			parent.RemoveChild(target)
		}
		return
	case "outer", "replace":
		if mode == "outer" && hasAttr(target, "data-ignore-morph") {
			return
		}
		if parent == nil {
			return
		}
		for _, n := range nodes {
			parent.InsertBefore(n, target)
		}
		parent.RemoveChild(target)
	case "inner":
		if hasAttr(target, "data-ignore-morph") {
			return
		}
		for c := target.FirstChild; c != nil; c = target.FirstChild {
			target.RemoveChild(c)
		}
		for _, n := range nodes {
			target.AppendChild(n)
		}
	case "prepend":
		first := target.FirstChild
		for _, n := range nodes {
			target.InsertBefore(n, first)
		}
	case "append":
		for _, n := range nodes {
			target.AppendChild(n)
		}
	case "before":
		if parent == nil {
			return
		}
		for _, n := range nodes {
			parent.InsertBefore(n, target)
		}
	case "after":
		if parent == nil {
			return
		}
		next := target.NextSibling
		for _, n := range nodes {
			parent.InsertBefore(n, next)
		}
	}
	if old != nil {
		keepIgnored(target, nodes)
	}
	for _, n := range nodes {
		p.applySignalAttrs(n, old)
	}
}

// keepIgnored puts the elements of the old subtree marked data-ignore-morph
// back in place of the new elements with their ids.
func keepIgnored(oldRoot *html.Node, nodes []*html.Node) {
	ignored := map[string]*html.Node{}
	for n := range oldRoot.Descendants() {
		if n.Type == html.ElementNode && hasAttr(n, "data-ignore-morph") {
			if id := attr(n, "id"); id != "" {
				ignored[id] = n
			}
		}
	}
	if len(ignored) == 0 {
		return
	}
	for _, root := range nodes {
		var swap [][2]*html.Node
		for n := range root.Descendants() {
			if n.Type != html.ElementNode {
				continue
			}
			if o := ignored[attr(n, "id")]; o != nil {
				swap = append(swap, [2]*html.Node{n, o})
			}
		}
		for _, s := range swap {
			if s[1].Parent != nil {
				s[1].Parent.RemoveChild(s[1])
			}
			s[0].Parent.InsertBefore(s[1], s[0])
			s[0].Parent.RemoveChild(s[0])
		}
	}
}

// signalAttrsByID returns the data-signals attributes of the elements under
// root, root included, that have an id.
func signalAttrsByID(root *html.Node) map[string]map[string]string {
	out := map[string]map[string]string{}
	for n := range withDescendants(root) {
		id := attr(n, "id")
		if n.Type != html.ElementNode || id == "" {
			continue
		}
		for _, a := range n.Attr {
			if strings.HasPrefix(a.Key, "data-signals") {
				if out[id] == nil {
					out[id] = map[string]string{}
				}
				out[id][a.Key] = a.Val
			}
		}
	}
	return out
}

// applySignalAttrs applies the data-signals attributes under root that differ
// from old, which maps element ids to the attributes before a morph.
// An attribute that needs a browser makes the signals unknown.
func (p *page) applySignalAttrs(root *html.Node, old map[string]map[string]string) {
	if p.signals == nil {
		return
	}
	for n := range withDescendants(root) {
		if n.Type != html.ElementNode {
			continue
		}
		prev := old[attr(n, "id")]
		for _, a := range n.Attr {
			if v, ok := prev[a.Key]; ok && v == a.Val {
				continue
			}
			if err := applySignalAttr(p.signals, a); err != nil {
				p.signals, p.sigErr = nil, err
				return
			}
		}
	}
	p.bindDefaults(root)
}

// fragment parses the elements of an event as Datastar does,
// inside a <template>, which keeps table rows and cells.
func fragment(markup string) ([]*html.Node, error) {
	return html.ParseFragment(strings.NewReader(markup), &html.Node{
		Type: html.ElementNode, Data: "template", DataAtom: atom.Template,
	})
}

func cloneNodes(nodes []*html.Node) []*html.Node {
	out := make([]*html.Node, len(nodes))
	for i, n := range nodes {
		out[i] = cloneNode(n)
	}
	return out
}

func cloneNode(n *html.Node) *html.Node {
	c := &html.Node{
		Type: n.Type, DataAtom: n.DataAtom, Data: n.Data, Namespace: n.Namespace,
		Attr: append([]html.Attribute(nil), n.Attr...),
	}
	for ch := range n.ChildNodes() {
		c.AppendChild(cloneNode(ch))
	}
	return c
}

func hasAttr(n *html.Node, key string) bool {
	_, ok := Attr(n, key)
	return ok
}

// selectAll returns the elements a selector matches. It supports the 2 forms
// Datapages applications use: #id and a tag name such as body.
func (p *page) selectAll(sel string) ([]*html.Node, error) {
	s := strings.TrimSpace(sel)
	if id, ok := strings.CutPrefix(s, "#"); ok {
		id, rest := cssIdent(id)
		if id == "" || rest != "" {
			return nil, unsupportedSelector(sel)
		}
		if n := ElementByID(p.doc, id); n != nil {
			return []*html.Node{n}, nil
		}
		return nil, nil
	}
	tag, rest := cssIdent(s)
	if tag == "" || rest != "" {
		return nil, unsupportedSelector(sel)
	}
	tag = strings.ToLower(tag)
	var out []*html.Node
	for n := range p.doc.Descendants() {
		if n.Type == html.ElementNode && n.Data == tag {
			out = append(out, n)
		}
	}
	return out, nil
}

func unsupportedSelector(sel string) error {
	return fmt.Errorf("selector %q: datapagestest supports #id and tag selectors", sel)
}

// cssIdent reads a CSS identifier with its escapes from the start of s and
// returns it unescaped with the rest of s.
func cssIdent(s string) (ident, rest string) {
	var b strings.Builder
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == '\\':
			i++
			if i == len(s) {
				return b.String(), ""
			}
			j := i
			for j < len(s) && j-i < 6 && isHex(s[j]) {
				j++
			}
			if j > i {
				r, _ := strconv.ParseUint(s[i:j], 16, 32)
				b.WriteRune(rune(r))
				i = j
				if i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n') {
					i++
				}
				continue
			}
			b.WriteByte(s[i])
			i++
		case c >= 0x80 || c == '-' || c == '_' ||
			'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9':
			b.WriteByte(c)
			i++
		default:
			return b.String(), s[i:]
		}
	}
	return b.String(), ""
}

func isHex(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}
