package datapagestest

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// Element is an element of a page or of an event.
type Element struct {
	n    *html.Node
	p    *page
	desc string // the query that found the element, for failure messages
}

// Node returns the parsed element.
func (e *Element) Node() *html.Node { return e.n }

// Text returns the element's text with whitespace collapsed and trimmed.
func (e *Element) Text() string { return Text(e.n) }

// Attr returns the value of the attribute key and whether the element has it.
func (e *Element) Attr(key string) (string, bool) { return Attr(e.n, key) }

// Value returns the value of the control as the HTML sets it: its value
// attribute, the text of a textarea or the value of the selected option.
// A value Datastar binds to a signal is in the page's signals instead.
func (e *Element) Value() string { return value(e.n) }

// Checked reports whether the checkbox or radio has the checked attribute.
// For an element with an explicit role it reports aria-checked.
func (e *Element) Checked() bool { return checked(e.n) }

// Get returns the element inside e that q selects.
// It fails unless exactly 1 element matches.
func (e *Element) Get(t testing.TB, q Query) *Element {
	t.Helper()
	return getOne(t, e.p, e.n, q, e.desc)
}

// All returns the elements inside e that q selects, in document order.
func (e *Element) All(q Query) []*Element { return getAll(e.p, e.n, q, e.desc) }

// Tree returns the accessibility tree of the element's contents.
func (e *Element) Tree() Tree { return e.p.tree(e.n) }

// RequireTree requires the accessibility tree of the element's contents to be want,
// and prints the difference and the actual tree otherwise.
func (e *Element) RequireTree(t testing.TB, want Tree) {
	t.Helper()
	if g, w := e.Tree().String(), want.String(); g != w {
		t.Fatalf("%s: the accessibility tree differs:\n%s\nactual tree:\n%s",
			e.desc, lineDiff(w, g), g)
	}
}

func getOne(t testing.TB, p *page, root *html.Node, q Query, desc string) *Element {
	t.Helper()
	ns := p.find(root, q)
	if len(ns) != 1 {
		t.Fatalf("%s: %s, want 1\n%s", desc, matching(len(ns), q), p.outline(root))
	}
	return &Element{n: ns[0], p: p, desc: desc + ": " + q.String()}
}

// matching says how many elements match q, as in "no element matches role link".
func matching(n int, q Query) string {
	switch n {
	case 0:
		return "no element matches " + q.String()
	case 1:
		return "1 element matches " + q.String()
	}
	return fmt.Sprintf("%d elements match %s", n, q)
}

func getAll(p *page, root *html.Node, q Query, desc string) []*Element {
	ns := p.find(root, q)
	out := make([]*Element, len(ns))
	for i, n := range ns {
		out[i] = &Element{n: n, p: p, desc: desc + ": " + q.String()}
	}
	return out
}

// outline renders the accessibility tree under root for a failure message.
func (p *page) outline(root *html.Node) string {
	tree := p.tree(root)
	if len(tree) == 0 {
		return "no accessible content"
	}
	return "accessibility tree:\n" + excerpt(tree.String())
}

// Tree is a list of sibling nodes of an accessibility tree.
type Tree []Node

// Node is an element of an accessibility tree. A zero field means the element
// doesn't have that property, so Checked: false describes an unchecked box.
//
// Elements without a role, such as div and span, and hidden elements are not nodes.
// Their contents belong to the nearest node around them.
type Node struct {
	Role string // the implicit or explicit ARIA role; "text" for text beside elements
	Name string // the accessible name

	// Value is the text of a textbox or the label of the option a combobox shows.
	Value string

	// Text is the text of a node that contains only text, when it differs from Name.
	// Text beside elements is a child node with the role "text".
	Text string

	URL      string // the href of a link
	Level    int    // the level of a heading
	Checked  bool
	Disabled bool
	Expanded bool
	Selected bool
	Pressed  bool
	Children Tree
}

// String renders t as a Go composite literal.
func (t Tree) String() string {
	var b strings.Builder
	b.WriteString("datapagestest.Tree{\n")
	writeTree(&b, t, 1)
	b.WriteString("}")
	return b.String()
}

func writeTree(b *strings.Builder, t Tree, depth int) {
	indent := strings.Repeat("\t", depth)
	for _, n := range t {
		b.WriteString(indent)
		b.WriteByte('{')
		var fields []string
		str := func(name, v string) {
			if v != "" {
				fields = append(fields, name+": "+strconv.Quote(v))
			}
		}
		flag := func(name string, v bool) {
			if v {
				fields = append(fields, name+": true")
			}
		}
		str("Role", n.Role)
		str("Name", n.Name)
		str("Value", n.Value)
		str("Text", n.Text)
		str("URL", n.URL)
		if n.Level != 0 {
			fields = append(fields, "Level: "+strconv.Itoa(n.Level))
		}
		flag("Checked", n.Checked)
		flag("Disabled", n.Disabled)
		flag("Expanded", n.Expanded)
		flag("Selected", n.Selected)
		flag("Pressed", n.Pressed)
		b.WriteString(strings.Join(fields, ", "))
		if len(n.Children) > 0 {
			if len(fields) > 0 {
				b.WriteString(", ")
			}
			b.WriteString("Children: datapagestest.Tree{\n")
			writeTree(b, n.Children, depth+1)
			b.WriteString(indent)
			b.WriteByte('}')
		}
		b.WriteString("},\n")
	}
}

// tree builds the accessibility tree of the contents of root.
func (p *page) tree(root *html.Node) Tree {
	b := treeBuilder{p: p}
	b.walk(root)
	b.flush()
	return b.out
}

type treeBuilder struct {
	p    *page
	out  Tree
	text strings.Builder // text not yet added as a node
}

func (b *treeBuilder) walk(n *html.Node) {
	for c := range n.ChildNodes() {
		switch c.Type {
		case html.TextNode:
			b.text.WriteString(c.Data)
			continue
		case html.ElementNode:
		default:
			continue
		}
		if hidesItself(c) {
			continue
		}
		block := isBlock(c)
		if block {
			b.text.WriteByte(' ')
		}
		if role := b.p.role(c); role != "" {
			b.flush()
			b.out = append(b.out, b.p.node(c, role))
		} else {
			b.walk(c)
		}
		if block {
			b.text.WriteByte(' ')
		}
	}
}

// flush adds the pending text as a node.
func (b *treeBuilder) flush() {
	if s := collapse(b.text.String()); s != "" {
		b.out = append(b.out, Node{Role: "text", Text: s})
	}
	b.text.Reset()
}

// node builds the node of n, which has role.
func (p *page) node(n *html.Node, role string) Node {
	node := Node{
		Role:     role,
		Name:     p.name(n),
		Disabled: disabled(n),
		Expanded: attr(n, "aria-expanded") == "true",
		Pressed:  attr(n, "aria-pressed") == "true",
	}
	switch role {
	case "checkbox", "radio", "switch", "menuitemcheckbox", "menuitemradio":
		node.Checked = checked(n)
	case "option", "tab", "row", "gridcell", "treeitem":
		node.Selected = selected(n)
	case "heading":
		node.Level = headingLevel(n)
	case "link":
		node.URL = attr(n, "href")
	}
	switch role {
	case "textbox", "searchbox", "slider", "spinbutton":
		node.Value = value(n)
		return node
	case "combobox":
		if n.Data == "select" {
			if opt := selectedOption(n); opt != nil {
				node.Value = Text(opt)
			}
			return node
		}
	case "img", "separator", "progressbar", "meter":
		return node
	}
	kids := p.tree(n)
	if len(kids) == 1 && kids[0].Role == "text" && len(kids[0].Children) == 0 {
		if kids[0].Text != node.Name {
			node.Text = kids[0].Text
		}
		return node
	}
	node.Children = kids
	return node
}

func headingLevel(n *html.Node) int {
	if l, err := strconv.Atoi(attr(n, "aria-level")); err == nil {
		return l
	}
	if len(n.Data) == 2 && n.Data[0] == 'h' && '1' <= n.Data[1] && n.Data[1] <= '6' {
		return int(n.Data[1] - '0')
	}
	return 2 // the default level of role heading
}

// lineDiff renders the lines of want and got that differ,
// marking removed lines with "-" and added ones with "+".
func lineDiff(want, got string) string {
	a, b := strings.Split(want, "\n"), strings.Split(got, "\n")
	// lcs[i][j] is the length of the longest common subsequence of a[i:] and b[j:].
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out strings.Builder
	line := func(mark, s string) {
		out.WriteString(mark)
		out.WriteString(s)
		out.WriteByte('\n')
	}
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			line("  ", a[i])
			i, j = i+1, j+1
		case i < len(a) && (j == len(b) || lcs[i+1][j] >= lcs[i][j+1]):
			line("- ", a[i])
			i++
		default:
			line("+ ", b[j])
			j++
		}
	}
	return strings.TrimSuffix(out.String(), "\n")
}
