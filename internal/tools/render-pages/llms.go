package main

import (
	"bytes"
	"fmt"
)

// llmsTxt returns llms.txt, which lists the Markdown copies of the pages for
// language models in the format of https://llmstxt.org. docs[i] is the doc of
// pages[i], and the first page introduces the project.
func llmsTxt(pages []docPage, docs []doc) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# Datapages\n\n> %s\n\n", docs[0].description)
	b.WriteString("Each link leads to the Markdown copy of a page of the " +
		"documentation. llms-full.txt holds all of them in 1 file.\n")
	group := ""
	for i, p := range pages {
		if p.group != group {
			group = p.group
			fmt.Fprintf(&b, "\n## %s\n\n", group)
		}
		if p.external() {
			fmt.Fprintf(&b, "- [%s](%s)\n", p.title, p.url)
			continue
		}
		fmt.Fprintf(&b, "- [%s](%s): %s\n",
			p.title, p.markdownFile(), docs[i].description)
	}
	return b.Bytes()
}

// llmsFullTxt returns llms-full.txt, the Markdown copies of all pages in the
// order of the navigation with their links as seen from the site root.
func llmsFullTxt(pages []docPage, docs []doc) []byte {
	var b bytes.Buffer
	for i, p := range pages {
		if p.external() {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.Write(bytes.TrimSpace(docs[i].markdownFromRoot))
		b.WriteString("\n")
	}
	return b.Bytes()
}
