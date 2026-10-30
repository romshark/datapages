package datapagestest

import (
	"fmt"
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// Query selects elements of a page or an event for [Response.Get], [Event.Get]
// and [Element.Get]. [Role], [Label] and [HasText] find what a visitor perceives
// and skip hidden elements. [ID], [Tag] and [Where] find any element.
type Query struct {
	desc string
	// visible skips elements hidden from the visitor.
	visible bool
	match   func(p *page, n *html.Node) bool
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
// roles named by their contents, title, and placeholder.
func Role(role, name string) Query {
	name = collapse(name)
	desc := "role " + role
	if name != "" {
		desc += fmt.Sprintf(" %q", name)
	}
	return Query{
		desc:    desc,
		visible: true,
		match: func(p *page, n *html.Node) bool {
			return p.role(n) == role && (name == "" || p.name(n) == name)
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
		match: func(p *page, n *html.Node) bool {
			if !p.isControl(n) {
				return false
			}
			if s := p.authorName(n); s != "" {
				return s == text
			}
			return p.labels(n) == text
		},
	}
}

// HasText selects the innermost elements whose text, as [Text] renders it, is text.
// For <button><span>Save</span></button> it selects the span.
func HasText(text string) Query {
	text = collapse(text)
	return Query{
		desc:    fmt.Sprintf("text %q", text),
		visible: true,
		match: func(_ *page, n *html.Node) bool {
			if Text(n) != text {
				return false
			}
			for c := range n.ChildNodes() {
				if c.Type == html.ElementNode && Text(c) == text {
					return false
				}
			}
			return true
		},
	}
}

// ID selects the element with the id attribute id.
func ID(id string) Query {
	return Query{
		desc:  fmt.Sprintf("id %q", id),
		match: func(_ *page, n *html.Node) bool { return hasID(n, id) },
	}
}

// Tag selects the elements named name, such as a custom element.
func Tag(name string) Query {
	name = strings.ToLower(name)
	return Query{
		desc:  "<" + name + ">",
		match: func(_ *page, n *html.Node) bool { return n.Data == name },
	}
}

// Where selects the elements f reports true for, such as those with a class.
// what names them in failure messages.
func Where(what string, f func(*Element) bool) Query {
	return Query{
		desc: what,
		match: func(p *page, n *html.Node) bool {
			return f(&Element{n: n, p: p, desc: what})
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
	q.match = func(p *page, n *html.Node) bool {
		v, ok := Attr(n, key)
		return ok && v == value && match(p, n)
	}
	q.desc += fmt.Sprintf(" with %s=%q", key, value)
	return q
}

// HasText narrows q to the elements whose whole text, as [Text] renders it, is text.
func (q Query) HasText(text string) Query {
	text = collapse(text)
	match := q.match
	q.match = func(p *page, n *html.Node) bool {
		return Text(n) == text && match(p, n)
	}
	q.desc += fmt.Sprintf(" with text %q", text)
	return q
}

// page is the document that queries search, which names and labels refer into.
type page struct {
	doc *html.Node
}

// find returns the elements under root, root excluded, that q selects.
func (p *page) find(root *html.Node, q Query) []*html.Node {
	var out []*html.Node
	for n := range root.Descendants() {
		if p.matches(n, q) {
			out = append(out, n)
		}
	}
	return out
}

// matches reports whether q selects n.
func (p *page) matches(n *html.Node, q Query) bool {
	if n.Type != html.ElementNode || !q.match(p, n) || q.visible && p.hidden(n) {
		return false
	}
	for _, c := range q.in {
		found := false
		for a := n.Parent; a != nil && !found; a = a.Parent {
			found = p.matches(a, c)
		}
		if !found {
			return false
		}
	}
	return true
}

// collapse joins the whitespace-separated fields of s with single spaces.
func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }
