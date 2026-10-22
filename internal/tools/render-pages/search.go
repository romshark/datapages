package main

import (
	"encoding/json"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
)

// searchEntry is a section of a page in search.json, which docs.js searches.
// The short names keep the file small.
type searchEntry struct {
	Page    string `json:"p"` // Title of the page.
	Section string `json:"s"` // Heading of the section, empty above the first.
	URL     string `json:"u"` // Relative to the site root.
	Text    string `json:"t"`
}

// searchTextLimit bounds the text of a [searchEntry] in bytes. Sections of the
// specification hold over 1000 words, which a reader finds by their heading
// and first lines.
const searchTextLimit = 1500

// searchEntries returns the entries of d, whose document is node. The first
// entry holds the text above the first heading below level 1. Each heading of
// a level that the contents of the page list starts another. Code blocks are
// left out.
func searchEntries(d doc, node ast.Node, src []byte) []searchEntry {
	entries := []searchEntry{{Page: d.page.title, URL: d.page.path}}
	var text strings.Builder
	flush := func() {
		e := &entries[len(entries)-1]
		words := strings.Fields(text.String())
		e.Text = truncate(strings.Join(words, " "), searchTextLimit)
		text.Reset()
	}
	for n := node.FirstChild(); n != nil; n = n.NextSibling() {
		switch n := n.(type) {
		case *ast.Heading:
			if n.Level == 1 || n.Level > d.page.depth() {
				writePlainText(&text, n, src)
				text.WriteString(" ")
				continue
			}
			flush()
			id, _ := n.Attribute("id")
			entries = append(entries, searchEntry{
				Page:    d.page.title,
				Section: plainText(n, src),
				URL:     d.page.path + "#" + id.Value(src),
			})
		case *ast.CodeBlock, *ast.HTMLBlock, *ast.ThematicBreak:
		default:
			writePlainText(&text, n, src)
			text.WriteString(" ")
		}
	}
	flush()
	return entries
}

// truncate returns s cut at the last space before n bytes, or s if it is not
// longer.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if i := strings.LastIndexByte(s[:n], ' '); i > 0 {
		return s[:i]
	}
	return s[:n]
}

// searchJSON returns search.json, the entries of all docs.
func searchJSON(docs []doc) ([]byte, error) {
	entries := []searchEntry{}
	for _, d := range docs {
		entries = append(entries, d.search...)
	}
	return json.Marshal(entries)
}
