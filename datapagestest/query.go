package datapagestest

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// Query selects elements of a page or an event for [Response.Get], [Event.Get]
// and [Element.Get]. [Role], [Label] and [HasText] find what a visitor perceives
// and skip hidden elements. [ID], [Tag] and [Where] find any element.
type Query struct {
	desc string
	// visible skips elements hidden from the visitor.
	visible bool
	match   func(p *page, n *html.Node) (bool, error)
	in      []Query
}

// String describes q for failure messages.
func (q Query) String() string { return q.desc }

// Role selects the elements with the ARIA role role, implicit or explicit,
// and the accessible name name. An empty name matches any name.
//
// Implicit roles follow HTML-AAM for native elements, such as button for
// <button> and link for <a href>. Date and time inputs have the role textbox.
// A custom element has a role only through its role attribute, since the role
// it sets in its shadow DOM is not part of the HTML.
//
// The accessible name follows W3C accname 1.2: aria-labelledby, aria-label,
// the native source such as <label>, alt and <legend>, the contents for
// roles named by their contents, title, and placeholder. A query with a name
// fails on an element of the role whose name is name with or without the text
// of the elements with data-show it crosses, since the browser decides.
func Role(role, name string) Query {
	name = collapse(name)
	desc := "role " + role
	if name != "" {
		desc += fmt.Sprintf(" %q", name)
	}
	return Query{
		desc:    desc,
		visible: true,
		match: func(p *page, n *html.Node) (bool, error) {
			if p.role(n) != role {
				return false, nil
			}
			if name == "" {
				return true, nil
			}
			return p.decide(n, name, (*page).name)
		},
	}
}

// Label selects the form controls labelled text by a <label>, aria-label or
// aria-labelledby. A placeholder or title is no label.
func Label(text string) Query {
	text = collapse(text)
	return Query{
		desc:    fmt.Sprintf("the control labelled %q", text),
		visible: true,
		match: func(p *page, n *html.Node) (bool, error) {
			if !p.isControl(n) {
				return false, nil
			}
			return p.decide(n, text, (*page).label)
		},
	}
}

// HasText selects the innermost elements whose text, as [Text] renders it, is text.
// For <button><span>Save</span></button> it selects the span. It fails on an
// element whose text is text with or without the elements with data-show in it.
func HasText(text string) Query {
	text = collapse(text)
	return Query{
		desc:    fmt.Sprintf("text %q", text),
		visible: true,
		match: func(p *page, n *html.Node) (bool, error) {
			for c := range n.ChildNodes() {
				if c.Type == html.ElementNode && Text(c) == text {
					return false, nil
				}
			}
			return p.decide(n, text, (*page).text)
		},
	}
}

// ID selects the element with the id attribute id.
func ID(id string) Query {
	return Query{
		desc:  fmt.Sprintf("id %q", id),
		match: func(_ *page, n *html.Node) (bool, error) { return hasID(n, id), nil },
	}
}

// Tag selects the elements named name, such as a custom element.
func Tag(name string) Query {
	name = strings.ToLower(name)
	return Query{
		desc:  "<" + name + ">",
		match: func(_ *page, n *html.Node) (bool, error) { return n.Data == name, nil },
	}
}

// Where selects the elements f reports true for, such as those with a class.
// what names them in failure messages. Like [ID] and [Tag], it finds hidden
// elements too.
func Where(what string, f func(*Element) bool) Query {
	return Query{
		desc: what,
		match: func(p *page, n *html.Node) (bool, error) {
			return f(&Element{n: n, p: p, desc: what}), nil
		},
	}
}

// In narrows q to the elements inside an element that container selects.
func (q Query) In(container Query) Query {
	q.in = append(slices.Clip(q.in), container)
	q.desc += " in " + container.desc
	return q
}

// Attr narrows q to the elements whose attribute key has the value value.
func (q Query) Attr(key, value string) Query {
	match := q.match
	q.match = func(p *page, n *html.Node) (bool, error) {
		if v, ok := Attr(n, key); !ok || v != value {
			return false, nil
		}
		return match(p, n)
	}
	q.desc += fmt.Sprintf(" with %s=%q", key, value)
	return q
}

// HasText narrows q to the elements whose whole text, as [Text] renders it, is text.
func (q Query) HasText(text string) Query {
	text = collapse(text)
	match := q.match
	q.match = func(p *page, n *html.Node) (bool, error) {
		if ok, err := p.decide(n, text, (*page).text); !ok || err != nil {
			return ok, err
		}
		return match(p, n)
	}
	q.desc += fmt.Sprintf(" with text %q", text)
	return q
}

// page is the document that queries search, which names and labels refer into.
type page struct {
	doc *html.Node
	// shown reports whether names and texts include the text of an element with
	// data-show. When nil, they fail on such an element instead, which a decided
	// name or text needs. crossed, when set, collects the elements it was asked about.
	shown   func(*html.Node) bool
	crossed *[]*html.Node
}

// allShown returns p with the text of every element with data-show included,
// as failure messages show it.
func (p *page) allShown() *page {
	return &page{doc: p.doc, shown: func(*html.Node) bool { return true }}
}

// maxConditional caps the elements with data-show whose combinations decide
// tries: 2^6 names or texts per element.
const maxConditional = 6

// decide reports whether of(n), a name or text, is want. When elements with
// data-show change it, it tries every combination of them shown and hidden.
// No combination giving want is no match, and every combination giving it is a
// match. Otherwise the browser decides, and decide fails.
func (p *page) decide(
	n *html.Node, want string, of func(*page, *html.Node) (string, error),
) (bool, error) {
	got, err := of(p, n)
	if err == nil {
		return got == want, nil
	}
	// The text outside the elements with data-show is in every combination, in
	// order. When want lacks it, no combination can give want.
	none, _ := of(&page{doc: p.doc, shown: func(*html.Node) bool { return false }}, n)
	if !inOrder(none, want) {
		return false, nil
	}
	var crossed []*html.Node
	_, _ = of(&page{doc: p.doc, shown: func(*html.Node) bool { return true }, crossed: &crossed}, n)
	crossed = unique(crossed)
	if len(crossed) > maxConditional {
		return false, &undecided{n: n, crossed: crossed}
	}
	matches := 0
	for mask := range 1 << len(crossed) {
		on := map[*html.Node]bool{}
		for i, c := range crossed {
			on[c] = mask>>i&1 == 1
		}
		if s, _ := of(&page{doc: p.doc, shown: func(c *html.Node) bool { return on[c] }}, n); s == want {
			matches++
		}
	}
	switch matches {
	case 0:
		return false, nil
	case 1 << len(crossed):
		return true, nil
	}
	return false, &undecided{n: n, crossed: crossed}
}

// inOrder reports whether the characters of part other than spaces appear in s
// in the same order.
func inOrder(part, s string) bool {
	i := 0
	for _, r := range part {
		if unicode.IsSpace(r) {
			continue
		}
		j := strings.IndexRune(s[i:], r)
		if j < 0 {
			return false
		}
		i += j + utf8.RuneLen(r)
	}
	return true
}

// unique removes repeated nodes from ns and keeps their first order.
func unique(ns []*html.Node) []*html.Node {
	seen := map[*html.Node]bool{}
	out := ns[:0]
	for _, n := range ns {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

// undecided reports a name or text that elements with data-show decide.
type undecided struct {
	n       *html.Node
	crossed []*html.Node
}

func (e *undecided) Error() string {
	exprs := make([]string, len(e.crossed))
	for i, c := range e.crossed {
		exprs[i] = strconv.Quote(attr(c, "data-show"))
	}
	if len(e.crossed) > maxConditional {
		return fmt.Sprintf("%s depends on %d elements with data-show, more than the %d tried",
			describeNode(e.n), len(e.crossed), maxConditional)
	}
	return fmt.Sprintf("%s depends on data-show %s, which the browser evaluates",
		describeNode(e.n), strings.Join(exprs, ", "))
}

// text returns the text of n as [Text] renders it. It fails on an element with
// data-show in it, unless p decides which of them show.
func (p *page) text(n *html.Node) (string, error) {
	var b strings.Builder
	var err error
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for c := range n.ChildNodes() {
			switch c.Type {
			case html.TextNode:
				b.WriteString(c.Data)
			case html.ElementNode:
				if !p.include(c, &err) {
					continue
				}
				walk(c)
			}
		}
	}
	walk(n)
	return collapse(b.String()), err
}

// include reports whether a name or text includes the element c. For an element
// with data-show it asks p.shown, or sets *err when p decides nothing.
func (p *page) include(c *html.Node, err *error) bool {
	expr, ok := Attr(c, "data-show")
	switch {
	case !ok:
		return true
	case p.shown == nil:
		if *err == nil {
			*err = &conditional{n: c, expr: expr}
		}
		return true
	}
	if p.crossed != nil {
		*p.crossed = append(*p.crossed, c)
	}
	return p.shown(c)
}

// find returns the elements under root, root excluded, that q selects.
func (p *page) find(root *html.Node, q Query) ([]*html.Node, error) {
	var out []*html.Node
	for n := range root.Descendants() {
		ok, err := p.matches(n, q)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", q, err)
		}
		if ok {
			out = append(out, n)
		}
	}
	return out, nil
}

// matches reports whether q selects n. It checks the containers of q first,
// so that a narrowed query computes no names outside them.
func (p *page) matches(n *html.Node, q Query) (bool, error) {
	if n.Type != html.ElementNode || q.visible && p.hidden(n) {
		return false, nil
	}
	for _, c := range q.in {
		found := false
		for a := n.Parent; a != nil && !found; a = a.Parent {
			ok, err := p.matches(a, c)
			if err != nil {
				return false, err
			}
			found = ok
		}
		if !found {
			return false, nil
		}
	}
	return q.match(p, n)
}

// collapse joins the whitespace-separated fields of s with single spaces.
func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }
