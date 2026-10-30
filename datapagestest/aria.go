package datapagestest

import (
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

type visibility int

const (
	shown visibility = iota
	hidden
	unknown // a data-show expression needs a browser
)

// visibility reports whether n and its ancestors up to the page root are shown.
// A hidden ancestor outweighs an unknown one. When unknown, it also returns the
// data-show expression.
func (p *page) visibility(n *html.Node) (visibility, string) {
	v, expr := shown, ""
	for a := n; a != nil && a != p.doc && a.Type == html.ElementNode; a = a.Parent {
		switch av, ae := p.ownVisibility(a); av {
		case hidden:
			return hidden, ""
		case unknown:
			if v == shown {
				v, expr = unknown, ae
			}
		}
	}
	return v, expr
}

// displayNone matches the inline styles that hide an element.
var displayNone = regexp.MustCompile(
	`(?i)(^|;)\s*(display\s*:\s*none|visibility\s*:\s*hidden)\b`,
)

// ownVisibility reports whether n hides itself, ignoring its ancestors.
// data-show decides over an inline style, since Datastar sets display from it.
func (p *page) ownVisibility(n *html.Node) (visibility, string) {
	switch n.Data {
	case "script", "style", "template", "head", "noscript", "title", "meta", "link":
		return hidden, ""
	case "input":
		if inputType(n) == "hidden" {
			return hidden, ""
		}
	case "dialog":
		if _, open := Attr(n, "open"); !open {
			return hidden, ""
		}
	}
	if _, ok := Attr(n, "hidden"); ok {
		return hidden, ""
	}
	if v, _ := Attr(n, "aria-hidden"); v == "true" {
		return hidden, ""
	}
	if expr, ok := Attr(n, "data-show"); ok {
		on, known := p.eval(expr)
		switch {
		case !known:
			return unknown, expr
		case !on:
			return hidden, ""
		}
		return shown, ""
	}
	if style, _ := Attr(n, "style"); displayNone.MatchString(style) {
		return hidden, ""
	}
	return shown, ""
}

// eval evaluates a data-show expression with [page.evalExpr].
func (p *page) eval(expr string) (value, known bool) {
	v, err := p.evalExpr(expr)
	if err != nil {
		return false, false
	}
	return truthy(v), true
}

var signalPath = regexp.MustCompile(`^[\w]+(\.[\w]+)*$`)

// truthy converts v as JavaScript converts a value to a boolean.
func truthy(v any) bool {
	switch v := v.(type) {
	case nil:
		return false
	case bool:
		return v
	case float64:
		return v != 0
	case string:
		return v != ""
	}
	return true
}

// inputType returns the type of an input element, text when it is missing or unknown.
func inputType(n *html.Node) string {
	t, _ := Attr(n, "type")
	t = strings.ToLower(strings.TrimSpace(t))
	switch t {
	case "button", "checkbox", "color", "date", "datetime-local", "email", "file",
		"hidden", "image", "month", "number", "password", "radio", "range", "reset",
		"search", "submit", "tel", "time", "url", "week":
		return t
	}
	return "text"
}

// role returns the role of n, which is the first token of its role attribute
// or the implicit role HTML-AAM gives the element. An element without a role,
// such as a div or a span, returns "".
func (p *page) role(n *html.Node) string {
	if n.Type != html.ElementNode {
		return ""
	}
	if r, ok := Attr(n, "role"); ok {
		if f := strings.Fields(strings.ToLower(r)); len(f) > 0 {
			if f[0] == "none" || f[0] == "presentation" {
				return ""
			}
			return f[0]
		}
	}
	switch n.Data {
	case "a", "area":
		if _, ok := Attr(n, "href"); ok {
			return "link"
		}
	case "article":
		return "article"
	case "aside":
		return "complementary"
	case "blockquote":
		return "blockquote"
	case "button":
		return "button"
	case "caption":
		return "caption"
	case "datalist":
		return "listbox"
	case "dd":
		return "definition"
	case "details", "fieldset", "optgroup":
		return "group"
	case "dialog":
		return "dialog"
	case "dt":
		return "term"
	case "figure":
		return "figure"
	case "footer":
		if !inSectioning(n) {
			return "contentinfo"
		}
	case "form":
		if p.authorName(n) != "" {
			return "form"
		}
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return "heading"
	case "header":
		if !inSectioning(n) {
			return "banner"
		}
	case "hr":
		return "separator"
	case "img":
		if alt, ok := Attr(n, "alt"); ok && alt == "" {
			return ""
		}
		return "img"
	case "input":
		switch inputType(n) {
		case "button", "image", "reset", "submit":
			return "button"
		case "checkbox":
			return "checkbox"
		case "radio":
			return "radio"
		case "range":
			return "slider"
		case "number":
			return "spinbutton"
		case "search":
			return "searchbox"
		case "color", "file", "hidden":
			return ""
		}
		return "textbox"
	case "li":
		return "listitem"
	case "main":
		return "main"
	case "math":
		return "math"
	case "menu", "ol", "ul":
		return "list"
	case "meter":
		return "meter"
	case "nav":
		return "navigation"
	case "option":
		return "option"
	case "output":
		return "status"
	case "p":
		return "paragraph"
	case "progress":
		return "progressbar"
	case "search":
		return "search"
	case "section":
		if p.authorName(n) != "" {
			return "region"
		}
	case "select":
		if _, multiple := Attr(n, "multiple"); multiple {
			return "listbox"
		}
		if size, err := strconv.Atoi(attr(n, "size")); err == nil && size > 1 {
			return "listbox"
		}
		return "combobox"
	case "table":
		return "table"
	case "tbody", "tfoot", "thead":
		return "rowgroup"
	case "td":
		return "cell"
	case "textarea":
		return "textbox"
	case "th":
		if attr(n, "scope") == "row" {
			return "rowheader"
		}
		return "columnheader"
	case "tr":
		return "row"
	}
	return ""
}

// inSectioning reports whether n is inside an element that scopes header and
// footer to itself.
func inSectioning(n *html.Node) bool {
	for a := n.Parent; a != nil; a = a.Parent {
		switch a.Data {
		case "article", "aside", "main", "nav", "section":
			return true
		}
	}
	return false
}

// nameFromContent lists the roles whose accessible name comes from their
// contents when nothing else names them.
var nameFromContent = map[string]bool{
	"button": true, "cell": true, "checkbox": true, "columnheader": true,
	"gridcell": true, "heading": true, "link": true, "menuitem": true,
	"menuitemcheckbox": true, "menuitemradio": true, "option": true, "radio": true,
	"row": true, "rowheader": true, "switch": true, "tab": true, "term": true,
	"tooltip": true, "treeitem": true,
}

// name returns the accessible name of n.
func (p *page) name(n *html.Node) string {
	if s := p.authorName(n); s != "" {
		return s
	}
	if s := p.nativeName(n); s != "" {
		return s
	}
	if nameFromContent[p.role(n)] {
		if s := collapse(p.contentText(n, nil)); s != "" {
			return s
		}
	}
	if s := collapse(attr(n, "title")); s != "" {
		return s
	}
	if n.Data == "textarea" || n.Data == "input" && takesText(inputType(n)) {
		return collapse(attr(n, "placeholder"))
	}
	return ""
}

// authorName returns the name aria-labelledby or aria-label give n.
func (p *page) authorName(n *html.Node) string {
	if ids, ok := Attr(n, "aria-labelledby"); ok {
		var parts []string
		for _, id := range strings.Fields(ids) {
			ref := ElementByID(p.doc, id)
			if ref == nil {
				continue
			}
			if s := collapse(attr(ref, "aria-label")); s != "" {
				parts = append(parts, s)
				continue
			}
			parts = append(parts, p.contentText(ref, nil))
		}
		if s := collapse(strings.Join(parts, " ")); s != "" {
			return s
		}
	}
	return collapse(attr(n, "aria-label"))
}

// nativeName returns the name HTML gives n through its labels or attributes.
func (p *page) nativeName(n *html.Node) string {
	switch n.Data {
	case "input":
		switch t := inputType(n); t {
		case "button", "reset", "submit":
			if v, ok := Attr(n, "value"); ok {
				return collapse(v)
			}
			switch t {
			case "submit":
				return "Submit"
			case "reset":
				return "Reset"
			}
			return ""
		case "image":
			if s := collapse(attr(n, "alt")); s != "" {
				return s
			}
		}
		return p.labels(n)
	case "button", "meter", "output", "progress", "select", "textarea":
		return p.labels(n)
	case "img", "area":
		return collapse(attr(n, "alt"))
	case "fieldset":
		return p.childText(n, "legend")
	case "table":
		return p.childText(n, "caption")
	case "figure":
		return p.childText(n, "figcaption")
	}
	return ""
}

// childText returns the text of the first child element of n named tag.
func (p *page) childText(n *html.Node, tag string) string {
	for c := range n.ChildNodes() {
		if c.Type == html.ElementNode && c.Data == tag {
			return collapse(p.contentText(c, nil))
		}
	}
	return ""
}

// labels returns the text of the <label> elements of the control n:
// those naming its id in for, and the label around it.
func (p *page) labels(n *html.Node) string {
	var parts []string
	var seen *html.Node
	if id := attr(n, "id"); id != "" {
		for l := range p.doc.Descendants() {
			if l.Type == html.ElementNode && l.Data == "label" && attr(l, "for") == id {
				parts = append(parts, p.contentText(l, n))
				seen = l
			}
		}
	}
	for a := n.Parent; a != nil; a = a.Parent {
		if a.Type == html.ElementNode && a.Data == "label" {
			if _, ok := Attr(a, "for"); !ok && a != seen {
				parts = append(parts, p.contentText(a, n))
			}
			break
		}
	}
	return collapse(strings.Join(parts, " "))
}

// contentText returns the text a visitor reads in n, skipping hidden elements and skip.
// An embedded control contributes its value, and an image its alt.
// Block elements separate their text from their neighbors'.
func (p *page) contentText(n, skip *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for c := range n.ChildNodes() {
			switch c.Type {
			case html.TextNode:
				b.WriteString(c.Data)
				continue
			case html.ElementNode:
			default:
				continue
			}
			if c == skip {
				continue
			}
			if v, _ := p.ownVisibility(c); v == hidden {
				continue
			}
			block := isBlock(c)
			if block {
				b.WriteByte(' ')
			}
			if s := collapse(attr(c, "aria-label")); s != "" {
				b.WriteString(s)
			} else {
				switch p.role(c) {
				case "textbox", "searchbox", "slider", "spinbutton":
					b.WriteString(p.value(c))
				case "combobox":
					if opt := p.selectedOption(c); opt != nil {
						b.WriteString(Text(opt))
					}
				case "img":
					b.WriteString(attr(c, "alt"))
				default:
					walk(c)
				}
			}
			if block {
				b.WriteByte(' ')
			}
		}
	}
	walk(n)
	return b.String()
}

// isBlock reports whether n is an element the browser lays out as a block,
// whose text does not run into the text around it.
func isBlock(n *html.Node) bool {
	switch n.Data {
	case "address", "article", "aside", "blockquote", "br", "dd", "details",
		"dialog", "div", "dl", "dt", "fieldset", "figcaption", "figure", "footer",
		"form", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hr", "li", "main",
		"nav", "ol", "p", "pre", "section", "summary", "table", "td", "th", "tr",
		"ul":
		return true
	}
	return false
}

// takesText reports whether an input of type t takes typed text.
func takesText(t string) bool {
	switch t {
	case "email", "number", "password", "search", "tel", "text", "url":
		return true
	}
	return false
}

// isControl reports whether n is a form control, which [Label] selects.
func (p *page) isControl(n *html.Node) bool {
	switch n.Data {
	case "button", "meter", "output", "progress", "select", "textarea":
		return true
	case "input":
		return inputType(n) != "hidden"
	}
	switch p.role(n) {
	case "checkbox", "combobox", "listbox", "radio", "searchbox", "slider",
		"spinbutton", "switch", "textbox":
		return true
	}
	return false
}

// bindPath returns the signal that n binds with data-bind.
func bindPath(n *html.Node) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == "data-bind" {
			return strings.TrimSpace(a.Val), true
		}
		if rest, ok := strings.CutPrefix(a.Key, "data-bind:"); ok {
			name, mods := splitModifiers(rest)
			return convertCase(name, mods["case"]), true
		}
	}
	return "", false
}

// value returns the value of the control n as the DOM reports it:
// its bound signal, its value attribute, the text of a textarea
// or the value of the selected option.
func (p *page) value(n *html.Node) string {
	checkable := n.Data == "input" &&
		(inputType(n) == "checkbox" || inputType(n) == "radio")
	signalIsValue := !checkable && n.Data != "select" && p.signals != nil
	if path, ok := bindPath(n); ok && signalIsValue {
		if v, ok := getSignal(p.signals, path); ok {
			return signalString(v)
		}
	}
	switch n.Data {
	case "textarea":
		var b strings.Builder
		for c := range n.ChildNodes() {
			if c.Type == html.TextNode {
				b.WriteString(c.Data)
			}
		}
		return b.String()
	case "select":
		if opt := p.selectedOption(n); opt != nil {
			return optionValue(opt)
		}
		return ""
	case "input":
		if v, ok := Attr(n, "value"); ok {
			return v
		}
		if checkable {
			return "on"
		}
		return ""
	}
	return attr(n, "value")
}

// checked reports whether n is checked by its bound signal,
// its checked attribute or aria-checked.
func (p *page) checked(n *html.Node) bool {
	if n.Data == "input" {
		t := inputType(n)
		if t != "checkbox" && t != "radio" {
			return false
		}
		if path, ok := bindPath(n); ok && p.signals != nil {
			v, _ := getSignal(p.signals, path)
			switch {
			case t == "radio":
				return signalString(v) == p.value(n)
			case isList(v):
				for _, e := range v.([]any) {
					if signalString(e) == p.value(n) {
						return true
					}
				}
				return false
			}
			return truthy(v)
		}
		_, ok := Attr(n, "checked")
		return ok
	}
	return attr(n, "aria-checked") == "true"
}

func isList(v any) bool {
	_, ok := v.([]any)
	return ok
}

// options returns the option elements of a select, including those in groups.
func options(sel *html.Node) []*html.Node {
	var out []*html.Node
	for n := range sel.Descendants() {
		if n.Type == html.ElementNode && n.Data == "option" {
			out = append(out, n)
		}
	}
	return out
}

func optionValue(opt *html.Node) string {
	if v, ok := Attr(opt, "value"); ok {
		return v
	}
	return Text(opt)
}

// selectedOption returns the option a single-choice select shows:
// the one its bound signal names, the one marked selected or the first.
func (p *page) selectedOption(sel *html.Node) *html.Node {
	opts := options(sel)
	if len(opts) == 0 {
		return nil
	}
	if path, ok := bindPath(sel); ok && p.signals != nil {
		if v, ok := getSignal(p.signals, path); ok {
			for _, o := range opts {
				if optionValue(o) == signalString(v) {
					return o
				}
			}
		}
	}
	for _, o := range opts {
		if _, ok := Attr(o, "selected"); ok {
			return o
		}
	}
	return opts[0]
}

// selected reports whether the option or the element with aria-selected is selected.
func (p *page) selected(n *html.Node) bool {
	if n.Data != "option" {
		return attr(n, "aria-selected") == "true"
	}
	var sel *html.Node
	for a := n.Parent; a != nil; a = a.Parent {
		if a.Data == "select" {
			sel = a
			break
		}
	}
	if sel == nil {
		_, ok := Attr(n, "selected")
		return ok
	}
	if _, multiple := Attr(sel, "multiple"); multiple {
		_, ok := Attr(n, "selected")
		return ok
	}
	return p.selectedOption(sel) == n
}

// disabled reports whether n is disabled by its disabled attribute,
// a disabled fieldset around it or aria-disabled.
func disabled(n *html.Node) bool {
	if attr(n, "aria-disabled") == "true" {
		return true
	}
	switch n.Data {
	case "button", "fieldset", "input", "optgroup", "option", "select", "textarea":
	default:
		return false
	}
	if _, ok := Attr(n, "disabled"); ok {
		return true
	}
	for a := n.Parent; a != nil; a = a.Parent {
		if a.Type == html.ElementNode && a.Data == "fieldset" {
			if _, ok := Attr(a, "disabled"); ok {
				return true
			}
		}
	}
	return false
}

// attr returns the value of the attribute key of n, "" when n lacks it.
func attr(n *html.Node, key string) string {
	v, _ := Attr(n, key)
	return v
}
