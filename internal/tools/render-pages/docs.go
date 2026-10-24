package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/text"
)

// repoURL is the repository on GitHub.
// The site links the files of the repository that it does not show to their page there.
const repoURL = "https://github.com/romshark/datapages"

// docPage is an entry of the navigation of the site: a page that shows a
// Markdown file of the repository or, with url set, a link that leaves the site.
// The pages of a version are under docs/<version>/ of the site, which is the
// root that their paths and links are relative to.
type docPage struct {
	file  string // Path from the repository root.
	path  string // URL path from the root of the version. Empty for its first page.
	url   string // Destination of a link that leaves the site.
	title string // Title in the navigation and in <title>.
	group string // Group of the navigation that lists the entry.
	// tocDepth is the deepest heading level that the contents of the page list.
	// 0 means 3.
	tocDepth int
}

// docPages are the entries of the navigation in its order,
// which the previous and next links of the pages follow.
var docPages = []docPage{
	{
		file:  "guide/README.md",
		path:  "",
		title: "Introduction",
		group: "Get started",
	},
	{
		file:  "guide/installation.md",
		path:  "installation",
		title: "Installation",
		group: "Get started",
	},
	{
		file:  "guide/upgrading.md",
		path:  "upgrading",
		title: "Upgrading",
		group: "Get started",
	},
	{
		file:  "guide/tutorial/README.md",
		path:  "tutorial/",
		title: "Overview",
		group: "Tutorial",
	},
	{
		file:  "guide/tutorial/create-project.md",
		path:  "tutorial/create-project",
		title: "Create the project",
		group: "Tutorial",
	},
	{
		file:  "guide/tutorial/show-the-list.md",
		path:  "tutorial/show-the-list",
		title: "Show the list",
		group: "Tutorial",
	},
	{
		file:  "guide/tutorial/add-items.md",
		path:  "tutorial/add-items",
		title: "Add items",
		group: "Tutorial",
	},
	{
		file:  "guide/tutorial/update-every-tab.md",
		path:  "tutorial/update-every-tab",
		title: "Update every open tab",
		group: "Tutorial",
	},
	{
		file:  "guide/tutorial/check-off-and-remove.md",
		path:  "tutorial/check-off-and-remove",
		title: "Check off and remove items",
		group: "Tutorial",
	},
	{
		file:  "guide/tutorial/rename-items.md",
		path:  "tutorial/rename-items",
		title: "Rename an item",
		group: "Tutorial",
	},
	{
		file:  "guide/tutorial/sign-in.md",
		path:  "tutorial/sign-in",
		title: "Sign in",
		group: "Tutorial",
	},
	{
		file:  "guide/tutorial/test-the-app.md",
		path:  "tutorial/test-the-app",
		title: "Test the app",
		group: "Tutorial",
	},
	{
		file:  "guide/application-model.md",
		path:  "application-model",
		title: "Application model",
		group: "Concepts",
	},
	{
		file:  "guide/request-lifecycle.md",
		path:  "request-lifecycle",
		title: "Request lifecycle",
		group: "Concepts",
	},
	{
		file:  "guide/pages.md",
		path:  "pages",
		title: "Pages and routes",
		group: "Guides",
	},
	{
		file:  "guide/actions.md",
		path:  "actions",
		title: "Actions",
		group: "Guides",
	},
	{
		file:  "guide/templates.md",
		path:  "templates",
		title: "Templates and forms",
		group: "Guides",
	},
	{
		file:  "guide/events.md",
		path:  "events",
		title: "Events",
		group: "Guides",
	},
	{
		file:  "guide/state.md",
		path:  "state",
		title: "Per-tab state",
		group: "Guides",
	},
	{
		file:  "guide/sessions.md",
		path:  "sessions",
		title: "Sessions and auth",
		group: "Guides",
	},
	{
		file:  "guide/assets.md",
		path:  "assets",
		title: "Static assets",
		group: "Guides",
	},
	{
		file:  "guide/offline.md",
		path:  "offline",
		title: "Offline and shims",
		group: "Guides",
	},
	{
		file:  "guide/testing.md",
		path:  "testing",
		title: "Testing",
		group: "Guides",
	},
	{
		file:  "guide/deployment.md",
		path:  "deployment",
		title: "Deployment",
		group: "Guides",
	},
	{
		file:  "guide/performance.md",
		path:  "performance",
		title: "Performance",
		group: "Guides",
	},
	{
		file:  "guide/security.md",
		path:  "security",
		title: "Security",
		group: "Guides",
	},
	{
		file:  "guide/multiple-applications.md",
		path:  "multiple-applications",
		title: "Multiple applications",
		group: "Guides",
	},
	{
		file:  "guide/ai-agents.md",
		path:  "ai-agents",
		title: "AI agents",
		group: "Guides",
	},
	{
		file:  "guide/troubleshooting.md",
		path:  "troubleshooting",
		title: "Troubleshooting",
		group: "Guides",
	},
	{
		file:  "SPECIFICATION.md",
		path:  "specification",
		title: "Specification",
		group: "Reference", tocDepth: 4,
	},
	// Every command has the sections Flags, Exit status and Examples, which
	// would repeat 5 times in the contents.
	{
		file:  "guide/cli.md",
		path:  "cli",
		title: "CLI",
		group: "Reference", tocDepth: 2,
	},
	{
		file:  "guide/configuration.md",
		path:  "configuration",
		title: "datapages.yaml",
		group: "Reference",
	},
	{
		url:   "https://pkg.go.dev/github.com/romshark/datapages",
		title: "Go API",
		group: "Reference",
	},
	{
		file:  "CHANGELOG.md",
		path:  "changelog",
		title: "Changelog",
		group: "Reference", tocDepth: 2,
	},
	{
		file:  "guide/examples.md",
		path:  "examples",
		title: "Examples",
		group: "More",
	},
	{
		file:  "FAQ.md",
		path:  "faq",
		title: "FAQ",
		group: "More",
	},
}

// external reports whether p is a link that leaves the site.
func (p docPage) external() bool { return p.url != "" }

// outFile returns the path of the file that holds p, relative to the root of
// the version. GitHub Pages serves a path without "/" at the end from the file
// with ".html" appended.
func (p docPage) outFile() string {
	if p.path == "" || strings.HasSuffix(p.path, "/") {
		return p.path + "index.html"
	}
	return p.path + ".html"
}

// markdownFile returns the path of the Markdown copy of p, relative to the root
// of the version.
func (p docPage) markdownFile() string {
	return strings.TrimSuffix(p.outFile(), ".html") + ".md"
}

// base returns the URL of the root of the version relative to p.
func (p docPage) base() string {
	if n := strings.Count(p.path, "/"); n > 0 {
		return strings.Repeat("../", n)
	}
	return "./"
}

// sourceURL returns the URL of the file of p at the Git ref on GitHub.
func (p docPage) sourceURL(ref string) string {
	return repoURL + "/blob/" + ref + "/" + p.file
}

// depth returns the deepest heading level that the contents of p list.
func (p docPage) depth() int {
	if p.tocDepth == 0 {
		return 3
	}
	return p.tocDepth
}

// doc is the Markdown file of a [docPage] converted for [docsPage].
type doc struct {
	page        docPage
	title       string    // Text of the level 1 heading.
	description string    // Text of the first paragraph.
	sections    []section // Every heading, in the order of the page.
	body        string
	// markdown is the Markdown copy of the page, whose links lead to the
	// Markdown copies of the other pages. markdownFromRoot links them as seen
	// from the root of the version, which is where llms-full.txt lives.
	markdown         []byte
	markdownFromRoot []byte
	search           []searchEntry
}

// section is a heading of a [doc].
type section struct {
	ID    string
	Title string
	Level int
}

// tocEntry is a link of the contents of a page.
type tocEntry struct {
	Href  string
	Title string
	Level int
}

// contents returns the contents of d:
// its headings from level 2 down to the depth of its page.
func contents(d doc) []tocEntry {
	var toc []tocEntry
	for _, s := range d.sections {
		if s.Level >= 2 && s.Level <= d.page.depth() {
			toc = append(toc, tocEntry{Href: "#" + s.ID, Title: s.Title, Level: s.Level})
		}
	}
	return toc
}

// navGroup is a group of the navigation.
type navGroup struct {
	Title   string
	Entries []navEntry
}

// navEntry is a link of the navigation or a previous or next link.
// The zero navEntry is no link.
type navEntry struct {
	Title    string
	Href     string
	Current  bool
	External bool
}

// navigation returns the navigation of pages as seen from pages[current].
func navigation(pages []docPage, current int) []navGroup {
	var groups []navGroup
	for i, p := range pages {
		if len(groups) == 0 || groups[len(groups)-1].Title != p.group {
			groups = append(groups, navGroup{Title: p.group})
		}
		e := navEntry{Title: p.title, Current: i == current}
		if p.external() {
			e.Href, e.External = p.url, true
		} else {
			e.Href = pages[current].base() + p.path
		}
		g := &groups[len(groups)-1]
		g.Entries = append(g.Entries, e)
	}
	return groups
}

// neighbors returns the pages before and after pages[current] in the navigation.
// Links that leave the site are skipped.
func neighbors(pages []docPage, current int) (prev, next navEntry) {
	link := func(p docPage) navEntry {
		return navEntry{Title: p.title, Href: pages[current].base() + p.path}
	}
	for i := current - 1; i >= 0; i-- {
		if !pages[i].external() {
			prev = link(pages[i])
			break
		}
	}
	for i := current + 1; i < len(pages); i++ {
		if !pages[i].external() {
			next = link(pages[i])
			break
		}
	}
	return prev, next
}

// docView is what [docsPage] shows of 1 page.
type docView struct {
	doc        doc
	nav        []navGroup
	toc        []tocEntry
	prev, next navEntry
	edition    edition
	versions   []versionLink // The versions that the header offers.
}

// newView returns the view of docs[i], the doc of pages[i], in edition e.
func newView(pages []docPage, docs []doc, i int, e edition) docView {
	prev, next := neighbors(pages, i)
	return docView{
		doc:      docs[i],
		nav:      navigation(pages, i),
		toc:      contents(docs[i]),
		prev:     prev,
		next:     next,
		edition:  e,
		versions: e.versions(pages[i].base(), pages[i].path),
	}
}

// base returns the URL of the root of the version relative to the page of v.
func (v docView) base() string { return v.doc.page.base() }

// home returns the URL of the site root, the landing page, relative to the
// page of v.
func (v docView) home() string { return v.base() + "../../" }

// sourceURL returns the URL of the file of the page of v on GitHub, at the Git
// ref of its edition.
func (v docView) sourceURL() string { return v.doc.page.sourceURL(v.edition.ref()) }

// section returns the part of the site that the page of v belongs to, as
// [siteHeader] takes it.
func (v docView) section() string {
	switch v.doc.page.path {
	case "specification", "examples":
		return v.doc.page.path
	}
	return "docs"
}

// readDocs reads the files of [docPages] from the repository root and converts
// them, with links to the files of the Git ref on GitHub.
// With drafts, a page whose file does not exist yet is a placeholder,
// which the watcher shows while the page is being written. Without,
// the missing file is an error.
func readDocs(root, ref string, drafts bool) ([]doc, error) {
	srcs := make([][]byte, len(docPages))
	missing := make([]bool, len(docPages))
	for i, p := range docPages {
		if p.external() {
			continue
		}
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p.file)))
		if drafts && errors.Is(err, fs.ErrNotExist) {
			src = fmt.Appendf(nil, "# %s\n\n`%s` does not exist yet.\n", p.title, p.file)
			missing[i] = true
		} else if err != nil {
			return nil, err
		}
		srcs[i] = src
	}
	return convertDocs(docPages, srcs, missing, ref)
}

// convertDocs converts srcs, the Markdown files of pages, to HTML with GitHub
// Flavored Markdown and an ID on every heading. srcs[i] is the file of
// pages[i] and nil for a link that leaves the site.
//
// A link is relative to the file that contains it, as on GitHub. A link to the
// file of a page leads to that page. A link to another file of the repository
// is rewritten to the file at the Git ref on GitHub. The site contains none of
// these files.
//
// A link to a fragment must name a heading of its page. goldmark derives the ID
// of a heading with "_" or a non-ASCII letter differently from GitHub, which
// would leave a link that works on GitHub dead on the site. A link to a page
// that is a placeholder, as placeholders[i] reports for pages[i], is not
// checked: its headings are not written yet.
func convertDocs(
	pages []docPage, srcs [][]byte, placeholders []bool, ref string,
) ([]doc, error) {
	p := parser.New(
		parser.WithAutoHeadingID(),
		parser.WithExtensions(extension.GFMParser),
	)
	s := site{
		pages:        pages,
		ids:          make([]map[string]bool, len(pages)),
		placeholders: placeholders,
		ref:          ref,
	}
	docs := make([]doc, len(pages))
	nodes := make([]ast.Node, len(pages))
	// Links are resolved after all files are parsed because a link can name a
	// heading of a page that is parsed later.
	links := make([][]*ast.Link, len(pages))
	for i, src := range srcs {
		if pages[i].external() {
			continue
		}
		d := &docs[i]
		d.page = pages[i]
		s.ids[i] = make(map[string]bool)
		ctx := parser.NewContext(parser.WithIDGenerator(goldmarkIDs{}))
		for _, id := range templateIDs {
			ctx.IDs().Put([]byte(id))
		}
		nodes[i] = p.Parse(src, parser.WithContext(ctx))
		markCallouts(nodes[i], src)
		for n := nodes[i].FirstChild(); n != nil; n = n.NextSibling() {
			if para, ok := n.(*ast.Paragraph); ok && d.description == "" {
				d.description = plainText(para, src)
			}
		}
		_ = ast.Walk(nodes[i], func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			switch n := n.(type) {
			case *ast.Heading:
				attr, _ := n.Attribute("id")
				id := attr.Value(src)
				s.ids[i][id] = true
				title := plainText(n, src)
				d.sections = append(d.sections,
					section{ID: id, Title: title, Level: n.Level})
				if n.Level == 1 && d.title == "" {
					d.title = title
				}
			case *ast.Link:
				links[i] = append(links[i], n)
			}
			return ast.WalkContinue, nil
		})
	}

	for i, src := range srcs {
		if pages[i].external() {
			continue
		}
		d := &docs[i]
		var err error
		// The Markdown copies are rewritten from the destinations as written,
		// which the HTML below replaces.
		if d.markdown, err = rewriteLinks(src, links[i],
			func(dest string) (string, error) {
				return s.resolve(i, dest, pages[i].base(), docPage.markdownFile)
			}); err != nil {
			return nil, fmt.Errorf("%s: %w", pages[i].file, err)
		}
		if d.markdownFromRoot, err = rewriteLinks(src, links[i],
			func(dest string) (string, error) {
				return s.resolve(i, dest, "", docPage.markdownFile)
			}); err != nil {
			return nil, fmt.Errorf("%s: %w", pages[i].file, err)
		}
		for _, l := range links[i] {
			dest, err := s.resolveLink(i, l.Destination.Value(src))
			if err != nil {
				return nil, fmt.Errorf("%s: %w", pages[i].file, err)
			}
			l.Destination = text.NewSingleLineValueFromString(dest, text.IdentityDecoder)
		}
		d.search = searchEntries(*d, nodes[i], src)
		r := html.New(
			html.WithExtensions(extension.GFMHTMLRenderer),
			html.WithNodeRendererDecorator(ast.KindCodeBlock, codeBlocks()),
			html.WithNodeRendererDecorator(ast.KindHeading, headingLinks),
			html.WithNodeRendererDecorator(ast.KindBlockquote, callouts),
		)
		var body bytes.Buffer
		if err := r.Render(&body, src, nodes[i]); err != nil {
			return nil, fmt.Errorf("%s: %w", pages[i].file, err)
		}
		d.body = body.String()
	}
	return docs, nil
}

// templateIDs are the element IDs that [docsPage] writes around the converted
// Markdown. goldmark gives a heading that would take one of them a suffix, as
// main-content-1 for "Main content", which keeps the skip link and the scripts
// pointing at the template's elements.
var templateIDs = []string{
	"main-content",
	"docs-nav",
	"docs-nav-toggle",
	"docs-toc-title",
	"search-results",
}

// goldmarkIDs generates the IDs of goldmark's default generator. Parse keeps
// the IDs put into a context only when the context names its own generator,
// and replaces them with an empty set otherwise.
type goldmarkIDs struct{}

func (goldmarkIDs) Generate(value []byte, kind ast.NodeKind) []byte {
	return parser.NewIDs().Generate(value, kind)
}

// site holds the heading IDs that links are resolved against.
type site struct {
	pages        []docPage
	ids          []map[string]bool // The heading IDs of every page.
	placeholders []bool            // Which pages have no file yet. May be nil.
	ref          string            // The Git ref of the links to GitHub.
}

// resolveLink returns the URL that the link dest has on the page with index from.
func (s site) resolveLink(from int, dest string) (string, error) {
	return s.resolve(from, dest, s.pages[from].base(), func(p docPage) string {
		return p.path
	})
}

// resolve returns the URL that the link dest of the file of pages[from] has on
// the site. base is the URL of the root of the version from where the URL is
// read, and target returns the URL of a page relative to that root.
func (s site) resolve(
	from int, dest, base string, target func(docPage) string,
) (string, error) {
	u, err := url.Parse(dest)
	if err != nil {
		return "", fmt.Errorf("link %q: %w", dest, err)
	}
	if u.Scheme != "" || u.Host != "" {
		return dest, nil
	}
	to := from
	if u.Path != "" {
		file := repoPath(s.pages[from].file, u.Path)
		if to = s.pageOf(file); to < 0 {
			return githubURL(s.ref, file, strings.HasSuffix(u.Path, "/"), u), nil
		}
	}
	page := ""
	if to != from {
		page = base + target(s.pages[to])
	}
	switch {
	case u.Fragment == "":
		if page == "" {
			return "#", nil
		}
		return page, nil
	case !s.ids[to][u.Fragment] && !s.placeholder(to):
		return "", fmt.Errorf("link %q names no heading", dest)
	}
	return page + "#" + u.Fragment, nil
}

// placeholder reports whether the page with index i has no file yet.
func (s site) placeholder(i int) bool {
	return i < len(s.placeholders) && s.placeholders[i]
}

// pageOf returns the index of the page that shows file, or -1.
// A directory leads to the page of its README.md, which GitHub shows for it.
func (s site) pageOf(file string) int {
	return slices.IndexFunc(s.pages, func(p docPage) bool {
		return !p.external() &&
			(p.file == file || p.file == path.Join(file, "README.md"))
	})
}

// repoPath returns the path from the repository root of what the link path p
// names in the file from.
func repoPath(from, p string) string {
	if strings.HasPrefix(p, "/") {
		return path.Clean(p[1:])
	}
	return path.Join(path.Dir(from), p)
}

// githubURL returns the URL of file at the Git ref on GitHub,
// with the query and the fragment of the link u. A directory has a tree URL.
func githubURL(ref, file string, dir bool, u *url.URL) string {
	kind := "/blob/" + ref + "/"
	if dir {
		kind, file = "/tree/"+ref+"/", file+"/"
	}
	r := repoURL + kind + file
	if u.RawQuery != "" {
		r += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		r += "#" + u.Fragment
	}
	return r
}

// rewriteLinks returns src with the destination of each of links,
// which were parsed from src, replaced by what resolve returns for it.
func rewriteLinks(
	src []byte, links []*ast.Link, resolve func(dest string) (string, error),
) ([]byte, error) {
	type edit struct {
		text.Index
		dest string
	}
	var edits []edit
	// The links of 1 reference definition share its destination.
	seen := make(map[int]bool)
	for _, l := range links {
		// An owned destination has no position in src, as for a destination
		// with escapes, and keeps its text.
		if l.Destination.IsOwned() {
			continue
		}
		at := l.Destination.Indices()[0]
		if seen[at.Start] {
			continue
		}
		seen[at.Start] = true
		dest, err := resolve(l.Destination.Value(src))
		if err != nil {
			return nil, err
		}
		edits = append(edits, edit{Index: at, dest: dest})
	}
	slices.SortFunc(edits, func(a, b edit) int { return a.Start - b.Start })
	var b bytes.Buffer
	last := 0
	for _, e := range edits {
		b.Write(src[last:e.Start])
		b.WriteString(e.dest)
		last = e.Stop
	}
	b.Write(src[last:])
	return b.Bytes(), nil
}

// plainText returns the text of n without its inline markup.
func plainText(n ast.Node, src []byte) string {
	var b strings.Builder
	writePlainText(&b, n, src)
	return strings.TrimSpace(b.String())
}

// writePlainText writes the text of n without its inline markup to w.
// A line break becomes a space.
func writePlainText(w io.StringWriter, n ast.Node, src []byte) {
	_ = ast.Walk(n, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Text:
			_, _ = w.WriteString(n.Value.Value(src))
			if n.SoftLineBreak() || n.HardLineBreak() {
				_, _ = w.WriteString(" ")
			}
		case *ast.CodeSpan:
			_, _ = w.WriteString(n.Value.Value(src))
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
}
