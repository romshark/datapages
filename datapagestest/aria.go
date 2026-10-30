package datapagestest

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// hidden reports whether n or one of its ancestors up to the page root hides
// itself in the HTML.
func (p *page) hidden(n *html.Node) bool {
	for a := n; a != nil && a != p.doc && a.Type == html.ElementNode; a = a.Parent {
		if hidesItself(a) {
			return true
		}
	}
	return false
}

// displayNone matches the inline styles that hide an element.
var displayNone = regexp.MustCompile(
	`(?i)(^|;)\s*(display\s*:\s*none|visibility\s*:\s*hidden)\b`,
)

// hidesItself reports whether the HTML of n hides it by its tag, the hidden
// attribute, aria-hidden or an inline style. The inline style of an element with
// data-show only covers the time before Datastar runs, so it hides nothing.
func hidesItself(n *html.Node) bool {
	switch n.Data {
	case "script", "style", "template", "head", "noscript", "title", "meta", "link":
		return true
	case "input":
		return inputType(n) == "hidden"
	case "dialog":
		if _, open := Attr(n, "open"); !open {
			return true
		}
	}
	if _, ok := Attr(n, "hidden"); ok {
		return true
	}
	if v, _ := Attr(n, "aria-hidden"); v == "true" {
		return true
	}
	if _, ok := Attr(n, "data-show"); ok {
		return false
	}
	style, _ := Attr(n, "style")
	return displayNone.MatchString(style)
}

// conditional reports text or a name that depends on whether an element with
// data-show is shown, which only the browser decides.
type conditional struct {
	n    *html.Node
	expr string
}

func (e *conditional) Error() string {
	return fmt.Sprintf("%s shows only if data-show=%q, which the browser evaluates",
		describeNode(e.n), e.expr)
}

// describeNode renders the start tag of n, such as <button class="primary">.
func describeNode(n *html.Node) string {
	var b strings.Builder
	b.WriteByte('<')
	b.WriteString(n.Data)
	for _, a := range n.Attr {
		fmt.Fprintf(&b, " %s=%q", a.Key, a.Val)
	}
	b.WriteByte('>')
	return b.String()
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
		if s, _ := p.authorName(n); s != "" {
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
		if s, _ := p.authorName(n); s != "" {
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

// label returns the name a <label>, aria-label or aria-labelledby gives n.
func (p *page) label(n *html.Node) (string, error) {
	if s, err := p.authorName(n); err != nil || s != "" {
		return s, err
	}
	return p.labels(n)
}

// name returns the accessible name of n. It fails when the name depends on an
// element with data-show.
func (p *page) name(n *html.Node) (string, error) {
	if s, err := p.authorName(n); err != nil || s != "" {
		return s, err
	}
	if s, err := p.nativeName(n); err != nil || s != "" {
		return s, err
	}
	if nameFromContent[p.role(n)] {
		s, err := p.contentText(n, nil)
		if s = collapse(s); err != nil || s != "" {
			return s, err
		}
	}
	if s := collapse(attr(n, "title")); s != "" {
		return s, nil
	}
	if n.Data == "textarea" || n.Data == "input" && takesText(inputType(n)) {
		return collapse(attr(n, "placeholder")), nil
	}
	return "", nil
}

// authorName returns the name aria-labelledby or aria-label give n.
func (p *page) authorName(n *html.Node) (string, error) {
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
			s, err := p.contentText(ref, nil)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		}
		if s := collapse(strings.Join(parts, " ")); s != "" {
			return s, nil
		}
	}
	return collapse(attr(n, "aria-label")), nil
}

// nativeName returns the name HTML gives n through its labels or attributes.
func (p *page) nativeName(n *html.Node) (string, error) {
	switch n.Data {
	case "input":
		switch t := inputType(n); t {
		case "button", "reset", "submit":
			if v, ok := Attr(n, "value"); ok {
				return collapse(v), nil
			}
			switch t {
			case "submit":
				return "Submit", nil
			case "reset":
				return "Reset", nil
			}
			return "", nil
		case "image":
			if s := collapse(attr(n, "alt")); s != "" {
				return s, nil
			}
		}
		return p.labels(n)
	case "button", "meter", "output", "progress", "select", "textarea":
		return p.labels(n)
	case "img", "area":
		return collapse(attr(n, "alt")), nil
	case "fieldset":
		return p.childText(n, "legend")
	case "table":
		return p.childText(n, "caption")
	case "figure":
		return p.childText(n, "figcaption")
	}
	return "", nil
}

// childText returns the text of the first child element of n named tag.
func (p *page) childText(n *html.Node, tag string) (string, error) {
	for c := range n.ChildNodes() {
		if c.Type == html.ElementNode && c.Data == tag {
			s, err := p.contentText(c, nil)
			return collapse(s), err
		}
	}
	return "", nil
}

// labels returns the text of the <label> elements of the control n, which are
// those naming its id in for and the label around it.
func (p *page) labels(n *html.Node) (string, error) {
	var labels []*html.Node
	if id := attr(n, "id"); id != "" {
		for l := range p.doc.Descendants() {
			if l.Type == html.ElementNode && l.Data == "label" && attr(l, "for") == id {
				labels = append(labels, l)
			}
		}
	}
	for a := n.Parent; a != nil; a = a.Parent {
		if a.Type == html.ElementNode && a.Data == "label" {
			if _, ok := Attr(a, "for"); !ok {
				labels = append(labels, a)
			}
			break
		}
	}
	var parts []string
	for _, l := range labels {
		s, err := p.contentText(l, n)
		if err != nil {
			return "", err
		}
		parts = append(parts, s)
	}
	return collapse(strings.Join(parts, " ")), nil
}

// contentText returns the text a visitor reads in n, skipping hidden elements
// and skip. An embedded control contributes its value, and an image its alt.
// Block elements separate their text from their neighbors'. It fails on an
// element with data-show below n, whose text may or may not show, unless p
// decides which of them show.
func (p *page) contentText(n, skip *html.Node) (string, error) {
	var b strings.Builder
	var err error
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
			if c == skip || hidesItself(c) {
				continue
			}
			if !p.include(c, &err) {
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
					b.WriteString(value(c))
				case "combobox":
					if opt := selectedOption(c); opt != nil {
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
	return b.String(), err
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

// value returns the value of the control n as the HTML sets it: its value
// attribute, the text of a textarea or the value of the selected option.
func value(n *html.Node) string {
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
		if opt := selectedOption(n); opt != nil {
			return optionValue(opt)
		}
		return ""
	case "input":
		if v, ok := Attr(n, "value"); ok {
			return v
		}
		if t := inputType(n); t == "checkbox" || t == "radio" {
			return "on"
		}
		return ""
	}
	return attr(n, "value")
}

// checked reports whether the checkbox or radio n has the checked attribute, or
// another element aria-checked="true".
func checked(n *html.Node) bool {
	if n.Data != "input" {
		return attr(n, "aria-checked") == "true"
	}
	if t := inputType(n); t != "checkbox" && t != "radio" {
		return false
	}
	_, ok := Attr(n, "checked")
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

// selectedOption returns the option a single-choice select shows, which is the
// first one marked selected or else the first.
func selectedOption(sel *html.Node) *html.Node {
	opts := options(sel)
	if len(opts) == 0 {
		return nil
	}
	for _, o := range opts {
		if _, ok := Attr(o, "selected"); ok {
			return o
		}
	}
	return opts[0]
}

// selected reports whether the option or the element with aria-selected is selected.
func selected(n *html.Node) bool {
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
	return selectedOption(sel) == n
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
