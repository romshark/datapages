package main

import (
	"bytes"
	"context"
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/stretchr/testify/require"
)

// testPages are pages at the paths of [docPages]: 1 for a directory, 1 in it,
// 1 in the site root, and a link that leaves the site.
var testPages = []docPage{
	{
		file:  "guide/README.md",
		path:  "docs/",
		title: "Introduction",
		group: "Get started",
	},
	{
		file:  "guide/events.md",
		path:  "docs/events",
		title: "Events",
		group: "Guides",
	},
	{
		file:     "SPECIFICATION.md",
		path:     "specification",
		title:    "Specification",
		group:    "Reference",
		tocDepth: 4,
	},
	{
		url:   "https://pkg.go.dev/github.com/romshark/datapages",
		title: "Go API",
		group: "Reference",
	},
}

var testSite = site{
	pages: testPages,
	ids: []map[string]bool{
		{"introduction": true},
		{"events": true, "dispatch": true},
		{"spec": true, "pages": true},
		nil,
	},
}

// blob is the URL of the files of the main branch on GitHub.
const blob = "https://github.com/romshark/datapages/blob/main/"

// TestResolveLink tests that a link relative to the file of a page leads to the
// page of the file it names, that a link to another file of the repository
// resolves to the file on GitHub and that every other link stays as written.
func TestResolveLink(t *testing.T) {
	for name, tt := range map[string]struct {
		from       int
		dest, want string
	}{
		"heading": {
			from: 1, dest: "#dispatch",
			want: "#dispatch",
		},
		"own file": {
			from: 1, dest: "events.md",
			want: "#",
		},
		"page in the same dir": {
			from: 0, dest: "events.md#dispatch",
			want: "../docs/events#dispatch",
		},
		"page in the parent": {
			from: 1, dest: "../SPECIFICATION.md",
			want: "../specification",
		},
		"heading of other page": {
			from: 1, dest: "../SPECIFICATION.md#pages",
			want: "../specification#pages",
		},
		"from the site root": {
			from: 2, dest: "guide/events.md#dispatch",
			want: "./docs/events#dispatch",
		},
		"directory of a README": {
			from: 2, dest: "guide/",
			want: "./docs/",
		},
		"README of the dir": {
			from: 1, dest: "README.md",
			want: "../docs/",
		},
		"file": {
			from: 1, dest: "../FAQ.md",
			want: blob + "FAQ.md",
		},
		"nested file": {
			from: 1, dest: "../internal/acceptance/README.md",
			want: blob + "internal/acceptance/README.md",
		},
		"line of a file": {
			from: 2, dest: "datapages.go#L10",
			want: blob + "datapages.go#L10",
		},
		"directory": {
			from: 1, dest: "../example/counter/",
			want: "https://github.com/romshark/datapages/tree/main/example/counter/",
		},
		"absolute": {
			from: 1, dest: "https://templ.guide",
			want: "https://templ.guide",
		},
		"mailto": {
			from: 1, dest: "mailto:dev@example.com",
			want: "mailto:dev@example.com",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := testSite.resolveLink(tt.from, tt.dest)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestResolveLinkErr tests that a fragment naming no heading of the linked page
// and an invalid URL are errors.
func TestResolveLinkErr(t *testing.T) {
	for name, dest := range map[string]string{
		"unknown heading":               "#missing",
		"heading of another page":       "../SPECIFICATION.md#dispatch",
		"unknown heading of other page": "../SPECIFICATION.md#missing",
		"invalid":                       "%zz",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := testSite.resolveLink(1, dest)
			require.Error(t, err)
		})
	}
}

// TestResolveMarkdownLink tests that a link of a Markdown copy leads to the
// Markdown copy of the page it names, as seen from the copy and as seen from
// the site root.
func TestResolveMarkdownLink(t *testing.T) {
	for name, tt := range map[string]struct {
		base, dest, want string
	}{
		"other page": {
			base: "../", dest: "../SPECIFICATION.md#pages",
			want: "../specification.md#pages",
		},
		"directory page": {
			base: "../", dest: "README.md",
			want: "../docs/index.md",
		},
		"from the root": {
			base: "", dest: "../SPECIFICATION.md#pages",
			want: "specification.md#pages",
		},
		"own heading": {
			base: "../", dest: "#dispatch",
			want: "#dispatch",
		},
		"file": {
			base: "../", dest: "../FAQ.md",
			want: blob + "FAQ.md",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := testSite.resolve(1, tt.dest, tt.base, docPage.markdownFile)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestConvertDocs tests that each file of a page yields its title, its
// description, its headings and its body, and that its links lead to the other
// pages in the HTML and to their Markdown copies in the Markdown copy.
func TestConvertDocs(t *testing.T) {
	docs, err := convertDocs(testPages, [][]byte{
		[]byte("# Introduction\n\n" +
			"Datapages is a *web* framework\nfor `Go`.\n\n" +
			"Read [the events](events.md#dispatch) and [the FAQ](../FAQ.md).\n\n" +
			"| a | b |\n| - | - |\n| 1 | 2 |\n"),
		[]byte("# Events\n\nEvents update tabs.\n\n## Dispatch\n\n" +
			"See [pages][spec] and [again][spec].\n\n" +
			"[spec]: ../SPECIFICATION.md#pages\n"),
		[]byte("# Spec\n\n## Pages\n\n### Actions\n\n#### Parameters\n\n" +
			"See [the guide](guide/) and [pages](#pages).\n"),
		nil,
	}, nil)
	require.NoError(t, err)
	require.Len(t, docs, 4)

	intro := docs[0]
	require.Equal(t, "Introduction", intro.title)
	require.Equal(t, "Datapages is a web framework for Go.", intro.description)
	require.Contains(t, intro.body, `<a href="../docs/events#dispatch">`)
	require.Contains(t, intro.body, `<a href="`+blob+`FAQ.md">`)
	require.Contains(t, intro.body, "<table>")
	require.Contains(t, string(intro.markdown),
		"[the events](../docs/events.md#dispatch)")
	require.Contains(t, string(intro.markdownFromRoot),
		"[the events](docs/events.md#dispatch)")

	events := docs[1]
	require.Equal(t, []section{
		{ID: "events", Title: "Events", Level: 1},
		{ID: "dispatch", Title: "Dispatch", Level: 2},
	}, events.sections)
	require.Equal(t, 2,
		strings.Count(events.body, `<a href="../specification#pages">`))
	require.Contains(t, string(events.markdown),
		"[spec]: ../specification.md#pages\n")

	spec := docs[2]
	require.Equal(t, []section{
		{ID: "spec", Title: "Spec", Level: 1},
		{ID: "pages", Title: "Pages", Level: 2},
		{ID: "actions", Title: "Actions", Level: 3},
		{ID: "parameters", Title: "Parameters", Level: 4},
	}, spec.sections)
	require.Contains(t, spec.body, `<a href="./docs/">`)
	require.Contains(t, spec.body, `<a href="#pages">`)
	require.Contains(t, spec.body, `<h4 id="parameters">`)

	require.Zero(t, docs[3])
}

// TestConvertDocsDeadFragment tests that a link to a fragment naming no heading
// fails the conversion. GitHub derives the ID use_it from the heading,
// goldmark derives use-it.
func TestConvertDocsDeadFragment(t *testing.T) {
	_, err := convertDocs(testPages[:1], [][]byte{
		[]byte("# Title\n\nSee [usage](#use_it).\n\n## Use_it\n"),
	}, nil)
	require.ErrorContains(t, err,
		`guide/README.md: link "#use_it" names no heading`)
}

// TestConvertDocsPlaceholder tests that a link to a heading of a placeholder
// page is not checked, since the page has no headings yet.
func TestConvertDocsPlaceholder(t *testing.T) {
	docs, err := convertDocs(testPages[:2], [][]byte{
		[]byte("# Introduction\n\nSee [dispatch](events.md#dispatch).\n"),
		[]byte("# Events\n\n`guide/events.md` does not exist yet.\n"),
	}, []bool{false, true})
	require.NoError(t, err)
	require.Contains(t, docs[0].body, `<a href="../docs/events#dispatch">`)
}

// TestReadDocs tests that a missing file is an error, and with drafts a
// placeholder page that names the file.
func TestReadDocs(t *testing.T) {
	root := t.TempDir()
	_, err := readDocs(root, false)
	require.ErrorIs(t, err, fs.ErrNotExist)

	docs, err := readDocs(root, true)
	require.NoError(t, err)
	require.Equal(t, "Introduction", docs[0].title)
	require.Contains(t, docs[0].body,
		"<code>guide/README.md</code> does not exist yet.")
}

// TestContents tests that the contents of a page list its headings from level
// 2 down to the depth of the page.
func TestContents(t *testing.T) {
	sections := []section{
		{ID: "spec", Title: "Spec", Level: 1},
		{ID: "pages", Title: "Pages", Level: 2},
		{ID: "actions", Title: "Actions", Level: 3},
		{ID: "parameters", Title: "Parameters", Level: 4},
	}
	for name, tt := range map[string]struct {
		page docPage
		want []tocEntry
	}{
		"default depth": {
			page: testPages[1],
			want: []tocEntry{
				{Href: "#pages", Title: "Pages", Level: 2},
				{Href: "#actions", Title: "Actions", Level: 3},
			},
		},
		"depth 4": {
			page: testPages[2],
			want: []tocEntry{
				{Href: "#pages", Title: "Pages", Level: 2},
				{Href: "#actions", Title: "Actions", Level: 3},
				{Href: "#parameters", Title: "Parameters", Level: 4},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			d := doc{page: tt.page, sections: sections}
			require.Equal(t, tt.want, contents(d))
		})
	}
}

// TestNavigation tests that the navigation groups the pages in their order,
// links them relative to the current page and marks it.
func TestNavigation(t *testing.T) {
	require.Equal(t, []navGroup{
		{Title: "Get started", Entries: []navEntry{
			{Title: "Introduction", Href: "../docs/"},
		}},
		{Title: "Guides", Entries: []navEntry{
			{Title: "Events", Href: "../docs/events", Current: true},
		}},
		{Title: "Reference", Entries: []navEntry{
			{Title: "Specification", Href: "../specification"},
			{
				Title:    "Go API",
				Href:     "https://pkg.go.dev/github.com/romshark/datapages",
				External: true,
			},
		}},
	}, navigation(testPages, 1))
}

// TestNeighbors tests that the previous and next links of a page lead to the
// pages next to it and skip links that leave the site.
func TestNeighbors(t *testing.T) {
	for name, tt := range map[string]struct {
		page       int
		prev, next navEntry
	}{
		"first": {
			page: 0,
			next: navEntry{Title: "Events", Href: "../docs/events"},
		},
		"middle": {
			page: 1,
			prev: navEntry{Title: "Introduction", Href: "../docs/"},
			next: navEntry{Title: "Specification", Href: "../specification"},
		},
		"last before a link": {
			page: 2,
			prev: navEntry{Title: "Events", Href: "./docs/events"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			prev, next := neighbors(testPages, tt.page)
			require.Equal(t, tt.prev, prev)
			require.Equal(t, tt.next, next)
		})
	}
}

// convert converts md as the file of the first test page.
func convert(t *testing.T, md string) doc {
	t.Helper()
	docs, err := convertDocs(testPages[:1], [][]byte{[]byte(md)}, nil)
	require.NoError(t, err)
	return docs[0]
}

// TestCodeBlocks tests that a code block in a language that Chroma has a lexer
// for gets token classes, that other code blocks stay plain text, and that
// every block gets its title and a copy button for its own ID, which a block
// with a title shows in the title.
func TestCodeBlocks(t *testing.T) {
	const goFunc = `<pre><code class="language-go"><span class="kd">func</span>`
	for name, tt := range map[string]struct {
		md   string
		want []string
	}{
		"go": {
			md:   "```go\nfunc f() {}\n```\n",
			want: []string{goFunc},
		},
		"no language": {
			md:   "```\nfunc f() {}\n```\n",
			want: []string{"<pre><code>func f() {}\n</code></pre>"},
		},
		"unknown language": {
			md:   "```nolexer\n<b>\n```\n",
			want: []string{`<pre><code class="language-nolexer">&lt;b&gt;`},
		},
		"title": {
			md: "```go title=\"app/app.go\"\nfunc f() {}\n```\n",
			want: []string{
				`<div class="code-title">app/app.go<neo-clipcopy for="code_1">`,
				`</neo-clipcopy></div><div class="code-body" id="code_1">` + goFunc,
				"</code></pre></div></div>\n",
			},
		},
		"copy buttons": {
			md: "```\na\n```\n\n```\nb\n```\n",
			want: []string{
				`<div class="code-body" id="code_1">`,
				`<neo-clipcopy for="code_1">`,
				`<div class="code-body" id="code_2">`,
				`<neo-clipcopy for="code_2">`,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			body := convert(t, "# T\n\n"+tt.md).body
			for _, want := range tt.want {
				require.Contains(t, body, want)
			}
		})
	}
}

// TestCallouts tests that a blockquote that starts with a GitHub alert marker
// becomes a callout without the marker and that other blockquotes stay.
func TestCallouts(t *testing.T) {
	for name, tt := range map[string]struct {
		md, want, unwanted string
	}{
		"note": {
			md: "> [!NOTE]\n> Read *this*.\n",
			want: `<div class="callout callout-note" role="note">` +
				`<p class="callout-title">Note</p>` +
				"\n<p>Read <em>this</em>.</p>\n</div>",
			unwanted: "[!NOTE]",
		},
		"marker only": {
			md: "> [!WARNING]\n",
			want: `<div class="callout callout-warning" role="note">` +
				`<p class="callout-title">Warning</p>` + "\n</div>",
			unwanted: "<p></p>",
		},
		"unknown kind": {
			md:       "> [!FOO]\n> text\n",
			want:     "<blockquote>",
			unwanted: "callout",
		},
		"blockquote": {
			md:       "> text\n",
			want:     "<blockquote>\n<p>text</p>\n</blockquote>",
			unwanted: "callout",
		},
	} {
		t.Run(name, func(t *testing.T) {
			body := convert(t, "# T\n\n"+tt.md).body
			require.Contains(t, body, tt.want)
			require.NotContains(t, body, tt.unwanted)
		})
	}
}

// TestHeadingLinks tests that every heading below level 1 ends with a link to
// itself.
func TestHeadingLinks(t *testing.T) {
	body := convert(t, "# Title\n\n## The `App` type\n").body
	require.Contains(t, body, `<h1 id="title">Title</h1>`)
	require.Contains(t, body, `<h2 id="the-app-type">The <code>App</code> type`+
		`<a class="heading-link" href="#the-app-type"`+
		` aria-label="Link to this section"></a></h2>`)
}

// TestSearchEntries tests that a page yields 1 search entry for the text above
// its first section and 1 for each heading down to its depth, without code.
func TestSearchEntries(t *testing.T) {
	docs, err := convertDocs(testPages[1:2], [][]byte{[]byte("# Events\n\n" +
		"Events update\nother tabs.\n\n```go\nfunc hidden() {}\n```\n\n" +
		"## Dispatch\n\nUse a `Dispatcher`.\n\n#### Details\n\n- a list\n")}, nil)
	require.NoError(t, err)
	require.Equal(t, []searchEntry{
		{
			Page: "Events",
			URL:  "docs/events",
			Text: "Events Events update other tabs.",
		},
		{
			Page:    "Events",
			Section: "Dispatch",
			URL:     "docs/events#dispatch",
			Text:    "Use a Dispatcher. Details a list",
		},
	}, docs[0].search)
}

// TestTruncate tests that a long text is cut at a space and a short one stays.
func TestTruncate(t *testing.T) {
	for name, tt := range map[string]struct {
		s    string
		n    int
		want string
	}{
		"short":    {s: "a b", n: 5, want: "a b"},
		"at space": {s: "aaa bbb ccc", n: 9, want: "aaa bbb"},
		"no space": {s: "aaaaaa", n: 3, want: "aaa"},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, truncate(tt.s, tt.n))
		})
	}
}

// TestLLMSTxt tests that llms.txt names the project with the description of
// the first page and lists every entry under its group, the pages by their
// Markdown copy.
func TestLLMSTxt(t *testing.T) {
	docs := []doc{
		{description: "Datapages is a web framework for Go."},
		{description: "Events update tabs."},
		{description: "The rules."},
		{},
	}
	require.Equal(t, "# Datapages\n\n"+
		"> Datapages is a web framework for Go.\n\n"+
		"Each link leads to the Markdown copy of a page of the documentation. "+
		"llms-full.txt holds all of them in 1 file.\n"+
		"\n## Get started\n\n"+
		"- [Introduction](docs/index.md): Datapages is a web framework for Go.\n"+
		"\n## Guides\n\n"+
		"- [Events](docs/events.md): Events update tabs.\n"+
		"\n## Reference\n\n"+
		"- [Specification](specification.md): The rules.\n"+
		"- [Go API](https://pkg.go.dev/github.com/romshark/datapages)\n",
		string(llmsTxt(testPages, docs)))
}

// render returns the HTML of c.
func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, c.Render(context.Background(), &b))
	return b.String()
}

// TestHeadingKeepsTemplateIDs tests that a heading that would take the ID of
// an element of the template gets another one.
func TestHeadingKeepsTemplateIDs(t *testing.T) {
	body := convert(t, "# T\n\n## Main content\n").body
	require.Contains(t, body, `<h2 id="main-content-1">`)
}

// TestSkipLink tests that the first element of every page is the link to its
// <main> element, which every page has exactly 1 of.
func TestSkipLink(t *testing.T) {
	docs, err := convertDocs(testPages, [][]byte{
		[]byte("# Introduction\n"),
		[]byte("# Events\n\n## Dispatch\n"),
		[]byte("# Spec\n"),
		nil,
	}, nil)
	require.NoError(t, err)
	for name, c := range map[string]templ.Component{
		"landing page": indexPage("v1"),
		"docs page":    docsPage(newView(testPages, docs, 1, "v1")),
	} {
		t.Run(name, func(t *testing.T) {
			html := render(t, c)
			body := html[strings.Index(html, "<body"):]
			body = body[strings.Index(body, ">")+1:]
			require.True(t, strings.HasPrefix(body, `<a class="skip-link"`+
				` href="#main-content">Skip to content</a>`), body[:200])
			require.Equal(t, 1, strings.Count(html, "<main"))
			require.Contains(t, html, `id="main-content" tabindex="-1">`)
		})
	}
}

// TestTemplateIDs tests that templateIDs lists every element ID that the
// template of a docs page writes, which the code blocks and headings of the
// page do not take.
func TestTemplateIDs(t *testing.T) {
	docs, err := convertDocs(testPages, [][]byte{
		[]byte("# Introduction\n"),
		[]byte("# Events\n\n## Dispatch\n\n```go\nfunc f() {}\n```\n"),
		[]byte("# Spec\n"),
		nil,
	}, nil)
	require.NoError(t, err)
	html := render(t, docsPage(newView(testPages, docs, 1, "v1")))
	written := make(map[string]bool)
	ids := regexp.MustCompile(` id="([^"]+)"`)
	for _, m := range ids.FindAllStringSubmatch(html, -1) {
		written[m[1]] = true
	}
	for _, s := range docs[1].sections {
		delete(written, s.ID)
	}
	delete(written, "code_1")
	want := make(map[string]bool)
	for _, id := range templateIDs {
		want[id] = true
	}
	require.Equal(t, want, written)
}

// TestPageLinks tests that a docs page links its Markdown copy on the site and
// its file on GitHub, and that these links and the GitHub link of the header
// show their icon as icon links.
func TestPageLinks(t *testing.T) {
	docs, err := convertDocs(testPages, [][]byte{
		[]byte("# Introduction\n"),
		[]byte("# Events\n"),
		[]byte("# Spec\n"),
		nil,
	}, nil)
	require.NoError(t, err)
	for name, tt := range map[string]struct {
		page int
		want []string
	}{
		"page in a directory": {
			page: 1,
			want: []string{
				`<a class="icon-link" href="../docs/events.md">` +
					`<svg class="icon-markdown"`,
				`<a class="icon-link" href="` + blob + `guide/events.md">` +
					`<svg class="icon-github"`,
				`<a class="header-github icon-link"` +
					` href="https://github.com/romshark/datapages">` +
					`<svg class="icon-github"`,
			},
		},
		"page in the site root": {
			page: 2,
			want: []string{
				`<a class="icon-link" href="./specification.md">`,
				`<a class="icon-link" href="` + blob + `SPECIFICATION.md">`,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			v := newView(testPages, docs, tt.page, "v1")
			html := render(t, docsPage(v))
			for _, want := range tt.want {
				require.Contains(t, html, want)
			}
		})
	}
}

// TestMermaid tests that a mermaid code block becomes the element that docs.js
// draws the diagram of, with the source escaped and without a copy button.
func TestMermaid(t *testing.T) {
	body := convert(t, "# T\n\n```mermaid\nflowchart LR\n    a --> b\n```\n").body
	require.Contains(t, body,
		"<pre class=\"mermaid\">flowchart LR\n    a --&gt; b\n</pre>")
	require.NotContains(t, body, "code-block")
}
