// Package graph renders a parsed application model as a graph: every page and
// abstract page with its handlers, the events those handlers dispatch and the
// events they subscribe to.
//
// [WriteDOT] writes Graphviz DOT source. [WriteHTML] writes a self-contained
// page around the SVG Graphviz lays out from it.
//
// Color names the kind of a thing: pages, abstract pages, events and the
// app-level actions each have one, and so does each of the three kinds of
// edge. The drawing carries no legend; the page names the kinds in its tree.
package graph

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/romshark/datapages/internal/parser/model"
)

// Colors name the kind of a node and the kind of an edge, from the GitHub
// Primer palette, which reads on a white and on a light gray background.
// Graphviz has no dark mode.
const (
	colorPage      = "#0969DA" // Pages.
	colorAbstract  = "#8250DF" // Abstract pages.
	colorEvent     = "#1A7F37" // Events.
	colorApp       = "#57606A" // App-level actions.
	colorDispatch  = "#BC4C00" // Handler -> event.
	colorSubscribe = "#1A7F37" // Event -> handler.
	colorEmbed     = "#8C959F" // Abstract page -> page.
	colorMuted     = "#57606A"
	colorText      = "#1F2328"
	colorCell      = "#FFFFFF"
)

// Node kinds.
const (
	kindPage     = "page"
	kindAbstract = "abstract"
	kindEvent    = "event"
	kindApp      = "app"
)

// Row kinds.
const (
	rowGET     = "get"
	rowStream  = "stream"
	rowAction  = "action"
	rowHandler = "handler"
)

// Edge kinds.
const (
	edgeDispatch  = "dispatch"
	edgeSubscribe = "subscribe"
	edgeEmbed     = "embed"
)

// Options configures the rendering.
type Options struct {
	// RankDir is the Graphviz rank direction, "TB" or "LR". Empty means "TB",
	// which puts the abstract pages on top, the pages below them and the
	// events in the last rank: every dispatch then points down and every
	// subscription up.
	RankDir string

	// HideLabel drops the graph label. The HTML page carries the package path
	// in its own header, where a label inside the drawing repeats it.
	HideLabel bool
}

// Source is where the application package declares a node or a row.
// Path is the file as it sits on the machine the graph was rendered on;
// File is what the page prints.
type Source struct {
	Path string `json:"path"`
	File string `json:"file"`
	Line int    `json:"line"`
	Col  int    `json:"col"`
}

// Field is one field of a struct a handler reads or an event carries.
type Field struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Wire string `json:"wire,omitempty"` // Name on the wire, from the struct tag.
	Note string `json:"note,omitempty"` // What the field is beyond plain data.
}

// Param is one input or one output of a handler.
type Param struct {
	Kind   string  `json:"kind"` // InputKind or OutputKind constant.
	Name   string  `json:"name,omitempty"`
	Type   string  `json:"type,omitempty"`
	Event  string  `json:"event,omitempty"` // Event type a dispatcher publishes.
	Fields []Field `json:"fields,omitempty"`
}

// Row is one line of a node: a handler, with the edges that start or end at it.
type Row struct {
	Port       string   `json:"port"`
	DOM        string   `json:"dom"` // The id of the SVG element.
	Kind       string   `json:"kind"`
	Text       string   `json:"text"`
	Name       string   `json:"name,omitempty"` // The Go method name.
	Method     string   `json:"method,omitempty"`
	Route      string   `json:"route,omitempty"`
	Source     *Source  `json:"source,omitempty"`
	Inputs     []Param  `json:"inputs,omitempty"`
	Outputs    []Param  `json:"outputs,omitempty"`
	Dispatches []string `json:"dispatches,omitempty"` // Event type names.
	Handles    string   `json:"handles,omitempty"`    // Event type name.
}

// Note is what a node carries beyond its name: the kind the page reads to pick
// an icon and a label for it, and the text the drawing prints for the kinds it
// draws.
type Note struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// Note kinds.
const (
	noteNoStream     = "no-stream"
	notePrivate      = "private"
	notePublic       = "public"
	noteSignalScoped = "signal-scoped"
)

// Node is a page, an abstract page, an event or the app-level actions.
type Node struct {
	ID     string  `json:"id"`  // e.g. "page:PageIndex".
	DOM    string  `json:"dom"` // The id of the SVG element.
	Kind   string  `json:"kind"`
	Name   string  `json:"name"`
	Badge  string  `json:"badge,omitempty"` // e.g. "index".
	Sub    string  `json:"sub,omitempty"`   // Route or subject.
	Notes  []Note  `json:"notes,omitempty"`
	Source *Source `json:"source,omitempty"`
	Color  string  `json:"color"`
	Rows   []Row   `json:"rows,omitempty"`

	// Fields is the payload of an event: what a dispatch carries.
	Fields []Field `json:"fields,omitempty"`
}

// Edge connects a handler row to an event, or an abstract page to a page.
type Edge struct {
	DOM      string `json:"dom"`
	Kind     string `json:"kind"`
	From     string `json:"from"`
	FromPort string `json:"fromPort,omitempty"`
	To       string `json:"to"`
	ToPort   string `json:"toPort,omitempty"`
	Color    string `json:"color"`
}

// Graph is the whole model as the renderers need it.
type Graph struct {
	Label string `json:"label"`
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Build turns the application model into the graph both renderers draw.
func Build(m *model.App, name string) Graph {
	g := Graph{Label: name}
	if m.PkgPath != "" {
		g.Label = m.PkgPath
	}

	origin := embedOrigins(m)
	evTypes := eventTypes(m)

	if len(m.Actions) > 0 {
		rows := make([]Row, 0, len(m.Actions))
		for i, a := range m.Actions {
			rows = append(rows, actionRow(m.Fset, fmt.Sprintf("a%d", i), a))
		}
		g.Nodes = append(g.Nodes, newNode(
			"app", kindApp, m.PkgName, colorApp, "app-level actions",
			sourceOf(m.Fset, m.Expr), rows,
		))
	}
	for _, p := range m.Pages {
		g.Nodes = append(g.Nodes, pageNode(m.Fset, p, origin))
	}
	for _, ap := range abstractPages(m) {
		g.Nodes = append(g.Nodes, abstractNode(m.Fset, ap))
	}

	known := map[string]bool{}
	for _, e := range m.Events {
		known[e.TypeName] = true
		g.Nodes = append(g.Nodes, eventNode(m.Fset, e, evTypes[e.TypeName]))
	}

	// Edges follow the nodes: an event a handler dispatches but the model does
	// not declare cannot occur in a model that parsed without errors, but is
	// drawn anyway so that a partial model loses no edge.
	referenced := map[string]bool{}
	for _, n := range g.Nodes {
		for _, r := range n.Rows {
			for _, ev := range r.Dispatches {
				referenced[ev] = true
				g.Edges = append(g.Edges, Edge{
					Kind: edgeDispatch, From: n.ID, FromPort: r.Port,
					To: "event:" + ev, Color: colorDispatch,
				})
			}
			if r.Handles != "" {
				referenced[r.Handles] = true
				g.Edges = append(g.Edges, Edge{
					Kind: edgeSubscribe, From: "event:" + r.Handles,
					To: n.ID, ToPort: r.Port, Color: colorSubscribe,
				})
			}
		}
	}
	for _, name := range slices.Sorted(maps.Keys(referenced)) {
		if known[name] {
			continue
		}
		g.Nodes = append(g.Nodes, newNode(
			"event:"+name, kindEvent, name, colorMuted, "undeclared", nil, nil,
		))
	}

	for _, ap := range abstractPages(m) {
		for _, sub := range ap.Embeds {
			g.Edges = append(g.Edges, Edge{
				Kind: edgeEmbed, From: "abstract:" + sub.TypeName,
				To: "abstract:" + ap.TypeName, Color: colorEmbed,
			})
		}
	}
	for _, p := range m.Pages {
		for _, ap := range p.Embeds {
			g.Edges = append(g.Edges, Edge{
				Kind: edgeEmbed, From: "abstract:" + ap.TypeName,
				To: "page:" + p.TypeName, Color: colorEmbed,
			})
		}
	}
	for i := range g.Edges {
		g.Edges[i].DOM = fmt.Sprintf("e_%d", i)
	}
	for i, n := range g.Nodes {
		for j, r := range n.Rows {
			g.Nodes[i].Rows[j].DOM = rowDOM(n, r)
		}
	}
	return g
}

func newNode(id, kind, name, color, sub string, src *Source, rows []Row) Node {
	return Node{
		ID: id, DOM: domID("n_" + id), Kind: kind, Name: name,
		Color: color, Sub: sub, Source: src, Rows: rows,
	}
}

// sourceOf locates the declaration e comes from. It reports nil for a model
// element the parser left without an expression, which is what a partial model
// and a hand-built one carry.
func sourceOf(fset *token.FileSet, e ast.Expr) *Source {
	if fset == nil || e == nil || !e.Pos().IsValid() {
		return nil
	}
	p := fset.Position(e.Pos())
	if p.Filename == "" {
		return nil
	}
	return &Source{
		Path: p.Filename, File: filepath.Base(p.Filename),
		Line: p.Line, Col: p.Column,
	}
}

// pageNode draws what the page type itself declares. What it inherits stays on
// the node of the abstract page that declares it: the parser promotes those
// handlers onto every embedding page, and drawing them per page turns one
// subscription of an abstract page into one edge per page that embeds it.
func pageNode(fset *token.FileSet, p *model.Page, origin map[any]string) Node {
	n := newNode("page:"+p.TypeName, kindPage, p.TypeName, colorPage, p.Route,
		sourceOf(fset, p.Expr), nil)
	switch p.PageSpecialization {
	case model.PageTypeIndex:
		n.Badge = "index"
	case model.PageTypeError404:
		n.Badge = "404"
	case model.PageTypeError500:
		n.Badge = "500"
	}

	if p.GET != nil && origin[p.GET.Handler] == "" {
		n.Rows = append(n.Rows, getRow(fset, p.GET.Handler, p.Route))
	}
	n.Rows = append(n.Rows, handlerRows(
		fset, p.StreamOpen, p.StreamClose, p.Actions, p.EventHandlers, origin,
	)...)

	if p.StreamOpen == nil && p.StreamClose == nil && len(p.EventHandlers) == 0 {
		n.Notes = append(n.Notes, Note{Kind: noteNoStream, Text: "no SSE stream"})
	}
	return n
}

func abstractNode(fset *token.FileSet, ap *model.AbstractPage) Node {
	n := newNode("abstract:"+ap.TypeName, kindAbstract, ap.TypeName,
		colorAbstract, "abstract page", sourceOf(fset, ap.Expr), nil)
	var methods []*model.Handler
	for _, h := range ap.Methods {
		if h.HTTPMethod == "GET" {
			n.Rows = append(n.Rows, getRow(fset, h, h.Route))
			continue
		}
		methods = append(methods, h)
	}
	n.Rows = append(n.Rows, handlerRows(
		fset, ap.StreamOpen, ap.StreamClose, methods, ap.EventHandlers, nil,
	)...)
	return n
}

// handlerRows renders the stream hooks, the actions and the event handlers a
// page or an abstract page declares. Handlers listed in origin are inherited
// and belong on the node of the abstract page they come from.
func handlerRows(
	fset *token.FileSet,
	streamOpen, streamClose *model.Handler,
	actions []*model.Handler,
	eventHandlers []*model.EventHandler,
	origin map[any]string,
) []Row {
	var rows []Row
	for _, h := range []struct {
		name string
		h    *model.Handler
	}{{"StreamOpen", streamOpen}, {"StreamClose", streamClose}} {
		if h.h == nil || origin[h.h] != "" {
			continue
		}
		in, out := handlerIO(h.h)
		rows = append(rows, Row{
			Port: "s" + h.name, Kind: rowStream, Text: h.name, Name: h.h.Name,
			Source: sourceOf(fset, h.h.Expr),
			Inputs: in, Outputs: out, Dispatches: dispatched(h.h),
		})
	}
	for i, a := range actions {
		if origin[a] != "" {
			continue
		}
		rows = append(rows, actionRow(fset, fmt.Sprintf("a%d", i), a))
	}
	for i, eh := range eventHandlers {
		if origin[eh] != "" {
			continue
		}
		in, out := eventHandlerIO(eh)
		rows = append(rows, Row{
			Port:    fmt.Sprintf("e%d", i),
			Kind:    rowHandler,
			Text:    eh.Name,
			Name:    eh.Name,
			Source:  sourceOf(fset, eh.Expr),
			Inputs:  in,
			Outputs: out,
			Handles: eh.EventTypeName,
		})
	}
	return rows
}

func actionRow(fset *token.FileSet, port string, a *model.Handler) Row {
	text := a.HTTPMethod + " " + a.Route
	if a.Route == "" {
		text = a.HTTPMethod + " " + a.Name
	}
	in, out := handlerIO(a)
	return Row{
		Port: port, Kind: rowAction, Text: text, Name: a.Name,
		Method: a.HTTPMethod, Route: a.Route, Source: sourceOf(fset, a.Expr),
		Inputs: in, Outputs: out, Dispatches: dispatched(a),
	}
}

func getRow(fset *token.FileSet, h *model.Handler, route string) Row {
	in, out := handlerIO(h)
	return Row{
		Port: "get", Kind: rowGET, Text: "GET", Name: h.Name,
		Method: "GET", Route: route, Source: sourceOf(fset, h.Expr),
		Inputs: in, Outputs: out, Dispatches: dispatched(h),
	}
}

func eventNode(fset *token.FileSet, e *model.Event, t types.Type) Node {
	n := newNode("event:"+e.TypeName, kindEvent, e.TypeName, colorEvent,
		subjectPattern(e), sourceOf(fset, e.Expr), nil)
	n.Fields = eventFields(e, t)
	if e.IsPrivate() {
		n.Notes = append(n.Notes, Note{
			Kind: notePrivate, Text: "private, requires a session",
		})
	} else {
		n.Notes = append(n.Notes, Note{
			Kind: notePublic, Text: "public, no session required",
		})
	}
	if e.IsSignalScoped() {
		n.Notes = append(n.Notes, Note{Kind: noteSignalScoped, Text: "signal-scoped"})
	}
	return n
}

// subjectPattern renders the subject a dispatch publishes to, with one
// placeholder per subject field in field order.
func subjectPattern(e *model.Event) string {
	var b strings.Builder
	b.WriteString(e.Subject)
	for _, f := range e.SubjectFields {
		b.WriteByte('.')
		switch {
		case f.Kind.IsUser():
			b.WriteString("{user}")
		case f.SignalName != "":
			b.WriteString("{signal:" + f.SignalName + "}")
		default:
			b.WriteString("{" + f.FieldName + "}")
		}
	}
	return b.String()
}

func dispatched(h *model.Handler) []string {
	if h == nil || len(h.InputDispatches) == 0 {
		return nil
	}
	out := make([]string, 0, len(h.InputDispatches))
	for _, d := range h.InputDispatches {
		out = append(out, d.EventTypeName)
	}
	return out
}

// abstractPages lists every abstract page the model reaches, in the order the
// pages embed them.
func abstractPages(m *model.App) []*model.AbstractPage {
	var out []*model.AbstractPage
	seen := map[*model.AbstractPage]bool{}
	var walk func(ap *model.AbstractPage)
	walk = func(ap *model.AbstractPage) {
		if seen[ap] {
			return
		}
		seen[ap] = true
		out = append(out, ap)
		for _, sub := range ap.Embeds {
			walk(sub)
		}
	}
	for _, p := range m.Pages {
		for _, ap := range p.Embeds {
			walk(ap)
		}
	}
	return out
}

// embedOrigins maps every handler an abstract page owns to that page's type
// name. The parser promotes them onto the embedding page, where they are
// otherwise indistinguishable from the page's own.
func embedOrigins(m *model.App) map[any]string {
	origin := map[any]string{}
	for _, ap := range abstractPages(m) {
		for _, h := range ap.Methods {
			origin[h] = ap.TypeName
		}
		if ap.StreamOpen != nil {
			origin[ap.StreamOpen] = ap.TypeName
		}
		if ap.StreamClose != nil {
			origin[ap.StreamClose] = ap.TypeName
		}
		for _, eh := range ap.EventHandlers {
			origin[eh] = ap.TypeName
		}
	}
	return origin
}

// domID turns a node id into an identifier that HTML, CSS and Graphviz all
// accept unquoted.
func domID(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// rowDOM is the id of the row cell in the DOT source. Graphviz prefixes the id
// of the element it generates for a cell with "a_".
func rowDOM(n Node, r Row) string { return "r_" + n.DOM + "_" + domID(r.Port) }

// WriteDOT writes the DOT source of m to w. name titles the graph.
func WriteDOT(w io.Writer, m *model.App, name string, opt Options) error {
	var b bytes.Buffer
	writeDOT(&b, Build(m, name), name, opt)
	_, err := w.Write(b.Bytes())
	return err
}

func writeDOT(b *bytes.Buffer, g Graph, name string, opt Options) {
	rankDir := opt.RankDir
	if rankDir == "" {
		rankDir = "TB"
	}

	fmt.Fprintf(b, "digraph %q {\n", name)
	fmt.Fprintf(b, "  rankdir=%s;\n", rankDir)
	b.WriteString("  nodesep=0.35;\n")
	b.WriteString("  ranksep=1.4;\n")
	b.WriteString("  splines=spline;\n")
	b.WriteString("  bgcolor=\"transparent\";\n")
	if !opt.HideLabel {
		fmt.Fprintf(b,
			"  labelloc=t; fontname=%q; fontsize=14; fontcolor=%q; label=%q;\n",
			"Helvetica-Bold", colorText, g.Label)
	}
	b.WriteString("  node [shape=plain, fontname=\"Helvetica\", fontsize=11];\n")
	b.WriteString("  edge [fontname=\"Helvetica\", fontsize=9];\n\n")

	for _, n := range g.Nodes {
		writeNode(b, n)
	}
	b.WriteString("\n")
	for _, e := range g.Edges {
		writeEdge(b, e)
	}

	// Pinning the events to the last rank keeps every dispatch pointing one
	// way and every subscription the other, which is what makes an app with
	// more than a handful of events readable.
	var events, abstracts []string
	for _, n := range g.Nodes {
		switch n.Kind {
		case kindEvent:
			events = append(events, n.ID)
		case kindAbstract:
			abstracts = append(abstracts, n.ID)
		}
	}
	writeRank(b, "max", events)
	writeRank(b, "same", abstracts)

	b.WriteString("}\n")
}

func writeRank(b *bytes.Buffer, rank string, ids []string) {
	if len(ids) == 0 {
		return
	}
	fmt.Fprintf(b, "  { rank=%s;", rank)
	for _, id := range ids {
		fmt.Fprintf(b, " %q;", id)
	}
	b.WriteString(" }\n")
}

func writeNode(b *bytes.Buffer, n Node) {
	fmt.Fprintf(b, "  %q [id=%q, label=<\n", n.ID, n.DOM)
	b.WriteString(`    <TABLE BORDER="0" CELLBORDER="1" CELLSPACING="0" CELLPADDING="5">` + "\n")

	header := "<B>" + esc(n.Name) + "</B>"
	if n.Badge != "" {
		header += ` <FONT POINT-SIZE="9">(` + esc(n.Badge) + ")</FONT>"
	}
	fmt.Fprintf(b, "      <TR><TD BGCOLOR=%q><FONT COLOR=\"#FFFFFF\">%s</FONT></TD></TR>\n",
		n.Color, header)

	if n.Sub != "" {
		fmt.Fprintf(b,
			"      <TR><TD BGCOLOR=%q ALIGN=\"LEFT\"><FONT COLOR=%q POINT-SIZE=\"9\">%s"+
				"</FONT></TD></TR>\n",
			colorCell, colorMuted, esc(n.Sub))
	}
	for _, note := range n.Notes {
		// The drawing marks what departs from the default. Printing "public"
		// would add a line to nearly every event node to say nothing.
		if note.Kind == notePublic {
			continue
		}
		fmt.Fprintf(b,
			"      <TR><TD BGCOLOR=%q ALIGN=\"LEFT\"><FONT COLOR=%q POINT-SIZE=\"9\">"+
				"<I>%s</I></FONT></TD></TR>\n",
			colorCell, colorMuted, esc(note.Text))
	}
	// A cell needs an HREF for Graphviz to give the element it generates an id.
	// The page cancels the navigation the link would do.
	for _, r := range n.Rows {
		id := r.DOM
		fmt.Fprintf(b,
			"      <TR><TD PORT=%q ID=%q HREF=\"#\" BGCOLOR=%q ALIGN=\"LEFT\">"+
				"<FONT COLOR=%q>%s</FONT></TD></TR>\n",
			r.Port, id, colorCell, colorText, esc(r.Text))
	}
	b.WriteString("    </TABLE>>];\n")
}

func writeEdge(b *bytes.Buffer, e Edge) {
	switch e.Kind {
	case edgeDispatch:
		fmt.Fprintf(b,
			"  %q:%q:e -> %q [id=%q, color=%q, style=dashed, penwidth=1.3];\n",
			e.From, e.FromPort, e.To, e.DOM, e.Color)
	case edgeSubscribe:
		fmt.Fprintf(b,
			"  %q -> %q:%q:w [id=%q, color=%q, penwidth=1.3];\n",
			e.From, e.To, e.ToPort, e.DOM, e.Color)
	case edgeEmbed:
		fmt.Fprintf(b,
			"  %q -> %q [id=%q, color=%q, style=dashed, arrowhead=onormal, "+
				"penwidth=1.1];\n",
			e.From, e.To, e.DOM, e.Color)
	}
}

// esc escapes text for a Graphviz HTML-like label.
var escaper = strings.NewReplacer(
	"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;",
)

func esc(s string) string { return escaper.Replace(s) }
