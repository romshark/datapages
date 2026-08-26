package graph_test

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/token"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/graph"
	"github.com/romshark/datapages/internal/parser/model"
)

// testModel builds a model covering everything the graph draws: an index page,
// a plain page, handlers inherited from an abstract page, a public, a private
// and a signal-scoped event, app-level actions and a dispatch of an event the
// model does not declare.
func testModel() *model.App {
	ping := &model.Handler{Name: "POSTPing", HTTPMethod: "POST", Route: "/ping"}
	inherited := &model.EventHandler{
		Name: "PostArchived", EventTypeName: "EventPostArchived",
	}
	base := &model.AbstractPage{
		TypeName:      "Base",
		Methods:       []*model.Handler{ping},
		EventHandlers: []*model.EventHandler{inherited},
	}

	send := &model.Handler{
		Name: "POSTSend", HTTPMethod: "POST", Route: "/send",
		InputDispatches: []*model.InputDispatch{
			{EventTypeName: "EventMessagingSent"},
			{EventTypeName: "EventGhost"},
		},
	}

	index := &model.Page{
		TypeName:           "PageIndex",
		Route:              "/",
		PageSpecialization: model.PageTypeIndex,
		GET: &model.HandlerGET{
			Handler: &model.Handler{Name: "GET", HTTPMethod: "GET", Route: "/"},
		},
		// The parser promotes what an embed owns onto the page.
		Actions:       []*model.Handler{send, ping},
		EventHandlers: []*model.EventHandler{inherited},
		Embeds:        []*model.AbstractPage{base},
	}
	other := &model.Page{
		TypeName: "PageOther",
		Route:    "/other",
		GET: &model.HandlerGET{
			Handler: &model.Handler{Name: "GET", HTTPMethod: "GET", Route: "/other"},
		},
		StreamOpen: &model.Handler{Name: "StreamOpen"},
	}

	// A page with neither stream hooks nor event handlers has no SSE stream.
	static := &model.Page{
		TypeName: "PageStatic",
		Route:    "/static",
		GET: &model.HandlerGET{
			Handler: &model.Handler{Name: "GET", HTTPMethod: "GET", Route: "/static"},
		},
	}

	return &model.App{
		PkgPath: "example.com/m/app",
		PkgName: "app",
		Pages:   []*model.Page{index, other, static},
		Actions: []*model.Handler{
			{Name: "POSTSignOut", HTTPMethod: "POST", Route: "/sign-out"},
		},
		Events: []*model.Event{
			{TypeName: "EventPostArchived", Subject: "posts.archived"},
			{
				TypeName: "EventMessagingSent", Subject: "messaging.sent",
				SubjectFields: []model.SubjectField{
					{FieldName: "Recipient", Kind: model.SubjectKindUser},
				},
			},
			{
				TypeName: "EventTick", Subject: "tick",
				SubjectFields: []model.SubjectField{
					{
						FieldName: "Instance", Kind: model.SubjectKindValue,
						SignalName: "instance_id",
					},
				},
			},
		},
	}
}

func TestWriteDOT(t *testing.T) {
	var b bytes.Buffer
	require.NoError(t, graph.WriteDOT(&b, testModel(), "app", graph.Options{}))
	dot := b.String()

	for name, want := range map[string]string{
		"graph label is the package path":   `label="example.com/m/app"`,
		"index page badge":                  `<B>PageIndex</B> <FONT POINT-SIZE="9">(index)</FONT>`,
		"page route":                        `>/other</FONT>`,
		"app node":                          `<B>app</B>`,
		"app action":                        `>POST /sign-out</FONT>`,
		"stream hook":                       `>StreamOpen</FONT>`,
		"page without a stream":             `<I>no SSE stream</I>`,
		"public event":                      `>posts.archived</FONT>`,
		"user subject":                      `>messaging.sent.{user}</FONT>`,
		"signal subject":                    `>tick.{signal:instance_id}</FONT>`,
		"private event note":                `<I>private, requires a session</I>`,
		"signal-scoped note":                `<I>signal-scoped</I>`,
		"abstract page node":                `"abstract:Base" [id="n_abstract_Base", label=<`,
		"inherited action on the abstract":  `>POST /ping</FONT>`,
		"inherited handler on the abstract": `>PostArchived</FONT>`,
		"embed edge":                        `"abstract:Base" -> "page:PageIndex" [id="e_3", color="#8C959F", style=dashed`,
		"dispatch edge":                     `"page:PageIndex":"a0":e -> "event:EventMessagingSent" [id="e_0", color="#BC4C00", style=dashed`,
		"subscribe edge":                    `"event:EventPostArchived" -> "abstract:Base":"e0":w [id="e_2", color="#1A7F37"`,
		"events pinned to the last rank":    `{ rank=max; "event:EventPostArchived"; "event:EventMessagingSent"; "event:EventTick"; "event:EventGhost"; }`,
		"abstract pages share a rank":       `{ rank=same; "abstract:Base"; }`,
		"top to bottom by default":          "rankdir=TB;",
		"page color":                        `<TD BGCOLOR="#0969DA"><FONT COLOR="#FFFFFF"><B>PageIndex</B>`,
		"abstract page color":               `<TD BGCOLOR="#8250DF"><FONT COLOR="#FFFFFF"><B>Base</B>`,
		"event color":                       `<TD BGCOLOR="#1A7F37"><FONT COLOR="#FFFFFF"><B>EventTick</B>`,
		"app color":                         `<TD BGCOLOR="#57606A"><FONT COLOR="#FFFFFF"><B>app</B>`,
		"undeclared event node":             `"event:EventGhost" [id="n_event_EventGhost", label=<`,
		"undeclared event note":             `>undeclared</FONT>`,
	} {
		t.Run(name, func(t *testing.T) {
			require.Contains(t, dot, want)
		})
	}

	// The drawing marks the exception, not the default: a public event gets
	// the note in the model for the panel, but no line in the node.
	require.NotContains(t, dot, "public, no session required")

	// An inherited row belongs to the abstract page, not to the page.
	require.NotContains(t, dot, `"page:PageIndex":"a1"`)

	// The drawing carries no legend; the page names the kinds in its tree.
	require.NotContains(t, dot, "legend")

	require.True(t, bytes.HasPrefix(b.Bytes(), []byte("digraph \"app\" {\n")))
	require.True(t, bytes.HasSuffix(b.Bytes(), []byte("}\n")))
}

// TestWriteDOTEscapes makes sure a route with the characters an HTML-like
// label reserves cannot break out of the label.
func TestWriteDOTEscapes(t *testing.T) {
	m := &model.App{
		PkgName: "app",
		Pages: []*model.Page{{
			TypeName: "PageQuery",
			Route:    `/q/{id}&<>"`,
			GET: &model.HandlerGET{
				Handler: &model.Handler{Name: "GET", HTTPMethod: "GET"},
			},
		}},
	}
	var b bytes.Buffer
	require.NoError(t, graph.WriteDOT(&b, m, "app", graph.Options{}))
	require.Contains(t, b.String(), `>/q/{id}&amp;&lt;&gt;&quot;</FONT>`)
}

func TestWriteDOTRankDir(t *testing.T) {
	var b bytes.Buffer
	require.NoError(t, graph.WriteDOT(&b, testModel(), "app",
		graph.Options{RankDir: "LR"}))
	require.Contains(t, b.String(), "rankdir=LR;")
}

func TestWriteDOTEmpty(t *testing.T) {
	var b bytes.Buffer
	require.NoError(t, graph.WriteDOT(&b, &model.App{PkgName: "app"}, "app", graph.Options{}))
	require.Contains(t, b.String(), "digraph \"app\" {")
	require.Contains(t, b.String(), "}\n")
}

func TestWriteDOTElementIDs(t *testing.T) {
	var b bytes.Buffer
	require.NoError(t, graph.WriteDOT(&b, testModel(), "app", graph.Options{}))
	dot := b.String()

	// The HTML page addresses the SVG by these ids.
	require.Contains(t, dot, `"page:PageIndex" [id="n_page_PageIndex", label=<`)
	require.Contains(t, dot, `"event:EventTick" [id="n_event_EventTick", label=<`)
	require.Contains(t, dot, `ID="r_n_page_PageIndex_a0" HREF="#"`)
	require.Contains(t, dot, `[id="e_0", color=`)
}

func TestWriteDOTHideLabel(t *testing.T) {
	var b bytes.Buffer
	require.NoError(t, graph.WriteDOT(&b, testModel(), "app",
		graph.Options{HideLabel: true}))
	require.NotContains(t, b.String(), `label="example.com/m/app"`)
}

func TestBuild(t *testing.T) {
	g := graph.Build(testModel(), "app")

	require.Equal(t, "example.com/m/app", g.Label)

	kinds := map[string][]string{}
	for _, n := range g.Nodes {
		kinds[n.Kind] = append(kinds[n.Kind], n.Name)
	}
	require.Equal(t, []string{"PageIndex", "PageOther", "PageStatic"}, kinds["page"])
	require.Equal(t, []string{"Base"}, kinds["abstract"])
	require.Equal(t, []string{"app"}, kinds["app"])
	require.Equal(t, []string{
		"EventPostArchived", "EventMessagingSent", "EventTick", "EventGhost",
	}, kinds["event"])

	// A note names its kind, which the page turns into an icon and a label.
	// Every event says whether it is private, the panel has no default to
	// read an absent note as.
	byName := map[string]graph.Node{}
	for _, n := range g.Nodes {
		byName[n.Name] = n
	}
	require.Equal(t, []graph.Note{
		{Kind: "private", Text: "private, requires a session"},
	}, byName["EventMessagingSent"].Notes)
	require.Equal(t, []graph.Note{
		{Kind: "public", Text: "public, no session required"},
	}, byName["EventPostArchived"].Notes)
	require.Equal(t, []graph.Note{
		{Kind: "public", Text: "public, no session required"},
		{Kind: "signal-scoped", Text: "signal-scoped"},
	}, byName["EventTick"].Notes)

	// The rows carry what the details panel reads.
	index := g.Nodes[1]
	require.Equal(t, "PageIndex", index.Name)
	require.Equal(t, "POST /send", index.Rows[1].Text)
	require.Equal(t, "POST", index.Rows[1].Method)
	require.Equal(t, "/send", index.Rows[1].Route)

	var got []string
	for _, e := range g.Edges {
		got = append(got, e.Kind+" "+e.From+":"+e.FromPort+" -> "+e.To+":"+e.ToPort)
	}
	require.Equal(t, []string{
		"dispatch page:PageIndex:a0 -> event:EventMessagingSent:",
		"dispatch page:PageIndex:a0 -> event:EventGhost:",
		"subscribe event:EventPostArchived: -> abstract:Base:e0",
		"embed abstract:Base: -> page:PageIndex:",
	}, got)
}

// TestBuildSource checks that every node and every row carries where the app
// package declares it. The page links to Path and prints File.
func TestBuildSource(t *testing.T) {
	fset := token.NewFileSet()
	// Without added lines every position sits on line 1, column offset+1.
	at := func(name string, offset int) ast.Expr {
		f := fset.AddFile(name, -1, 100)
		return &ast.Ident{NamePos: f.Pos(offset)}
	}

	get := &model.Handler{
		Name: "GET", HTTPMethod: "GET", Route: "/",
		Expr: at("/m/app/page_index.go", 40),
	}
	m := &model.App{
		Fset: fset, PkgName: "app", Expr: at("/m/app/app.go", 0),
		Pages: []*model.Page{{
			TypeName: "PageIndex", Route: "/",
			Expr: at("/m/app/page_index.go", 10),
			GET:  &model.HandlerGET{Handler: get},
		}},
		Actions: []*model.Handler{{
			Name: "POSTSignOut", HTTPMethod: "POST", Route: "/sign-out",
			Expr: at("/m/app/app.go", 60),
		}},
		Events: []*model.Event{{
			TypeName: "EventTick", Subject: "tick",
			Expr: at("/m/app/events.go", 4),
		}},
	}

	byName := map[string]graph.Node{}
	for _, n := range graph.Build(m, "app").Nodes {
		byName[n.Name] = n
	}

	require.Equal(t, &graph.Source{
		Path: "/m/app/page_index.go", File: "page_index.go", Line: 1, Col: 11,
	}, byName["PageIndex"].Source)
	require.Equal(t, &graph.Source{
		Path: "/m/app/page_index.go", File: "page_index.go", Line: 1, Col: 41,
	}, byName["PageIndex"].Rows[0].Source)
	require.Equal(t, &graph.Source{
		Path: "/m/app/app.go", File: "app.go", Line: 1, Col: 1,
	}, byName["app"].Source)
	require.Equal(t, &graph.Source{
		Path: "/m/app/app.go", File: "app.go", Line: 1, Col: 61,
	}, byName["app"].Rows[0].Source)
	require.Equal(t, &graph.Source{
		Path: "/m/app/events.go", File: "events.go", Line: 1, Col: 5,
	}, byName["EventTick"].Source)

	// A model the parser left without expressions carries no source, and no
	// Fset means no position to read one from.
	for _, n := range graph.Build(testModel(), "app").Nodes {
		require.Nil(t, n.Source, n.Name)
		for _, r := range n.Rows {
			require.Nil(t, r.Source, n.Name+"."+r.Text)
		}
	}
}

func TestWriteHTML(t *testing.T) {
	svg := []byte(`<?xml version="1.0"?>` + "\n" +
		`<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/svg11.dtd">` +
		"\n" + `<svg width="10pt"><g id="n_page_PageIndex"></g></svg>`)

	var b bytes.Buffer
	require.NoError(t, graph.WriteHTML(&b, testModel(), "app", svg))
	page := b.String()

	require.True(t, strings.HasPrefix(page, "<!doctype html>"))
	require.Contains(t, page, "<title>example.com/m/app</title>")

	// The prolog and the doctype of the standalone SVG must not survive into
	// the page, the root element must.
	require.NotContains(t, page, "<?xml")
	require.NotContains(t, page, "<!DOCTYPE svg")
	require.Contains(t, page, `<svg width="10pt"><g id="n_page_PageIndex"></g></svg>`)

	// Nothing may be loaded from the network: the kit is inlined, not linked.
	require.NotContains(t, page, "src=")
	require.NotContains(t, page, "<link")
	require.Contains(t, page, "Morpheus v0.1.0")
	require.Contains(t, page, "<neo-tree id=\"tree\"")
	require.Contains(t, page, `<neo-button id="fit"`)
	require.Contains(t, page, "lucide-info") // The icons are inlined too.

	data := between(t, page, `<script type="application/json" id="graph-data">`, "</script>")
	var g graph.Graph
	require.NoError(t, json.Unmarshal([]byte(data), &g))
	require.Equal(t, "n_page_PageIndex", g.Nodes[1].DOM)
	require.Equal(t, "r_n_page_PageIndex_a0", g.Nodes[1].Rows[1].DOM)
	require.Equal(t, "e_0", g.Edges[0].DOM)
}

func between(t *testing.T, s, from, to string) string {
	t.Helper()
	_, rest, ok := strings.Cut(s, from)
	require.True(t, ok, "missing %q", from)
	got, _, ok := strings.Cut(rest, to)
	require.True(t, ok, "missing %q", to)
	return got
}

// TestDiagramCSS checks that the stylesheet dressing the drawing keys on the
// colors Graphviz actually bakes into the SVG.
func TestDiagramCSS(t *testing.T) {
	var b bytes.Buffer
	require.NoError(t, graph.WriteDOT(&b, testModel(), "app", graph.Options{}))
	dot := b.String()

	var page bytes.Buffer
	require.NoError(t, graph.WriteHTML(&page, testModel(), "app",
		[]byte(`<svg></svg>`)))
	css := page.String()

	for name, tc := range map[string]struct {
		baked string // Written into the DOT, so Graphviz writes it to the SVG.
		rule  string // Recolors it in the page.
	}{
		"page header": {
			baked: `BGCOLOR="#0969DA"`,
			rule:  `#canvas g.node polygon[fill="#0969DA" i] { fill: var(--dp-page); }`,
		},
		"event header": {
			baked: `BGCOLOR="#1A7F37"`,
			rule:  `#canvas g.node polygon[fill="#1A7F37" i] { fill: var(--dp-event); }`,
		},
		"cell background": {
			baked: `BGCOLOR="#FFFFFF"`,
			rule:  `#canvas g.node polygon[fill="#FFFFFF" i] { fill: var(--dp-surface); }`,
		},
		"dispatch edge": {
			baked: `color="#BC4C00"`,
			rule:  `#canvas g.edge path[stroke="#BC4C00" i] { stroke: var(--dp-dispatch); }`,
		},
		"embed edge": {
			baked: `color="#8C959F"`,
			rule:  `#canvas g.edge polygon[fill="#8C959F" i] { fill: var(--dp-embed); }`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Contains(t, dot, tc.baked)
			require.Contains(t, css, tc.rule)
		})
	}

	// Every color has a value in both themes.
	require.Contains(t, css, "--dp-page: #4493f8;")
	require.Contains(t, css, ":root.dark {")
	require.Contains(t, css, "@media (prefers-color-scheme: dark) {")
}
