package richtext

import (
	"io"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/romshark/datapages/example/docs/delta"
)

// previewLines is the number of lines in a document thumbnail, more than fit.
const previewLines = 30

// WriteHTML writes doc as the markup Quill 2 renders in its editor element.
// The page shows it until the editor loads and replaces it with the same
// markup, which keeps the text in place.
func WriteHTML(w io.Writer, doc delta.Delta) error {
	var b strings.Builder
	writeLines(&b, doc, math.MaxInt, true)
	_, err := io.WriteString(w, b.String())
	return err
}

// Preview returns the markup of [WriteHTML] for the first lines of doc, the
// thumbnail in the document list. Links become plain text: the thumbnail is
// inside a link, and links must not nest.
func Preview(doc delta.Delta) string {
	var b strings.Builder
	writeLines(&b, doc, previewLines, false)
	return b.String()
}

func writeLines(b *strings.Builder, doc delta.Delta, maxLines int, links bool) {
	container := ""
	n := 0
	for l := range lines(doc) {
		if n == maxLines {
			break
		}
		n++
		if c := containerOf(l); c != container {
			b.WriteString(containerClose[container])
			b.WriteString(containerOpen[c])
			container = c
		}
		writeLine(b, l, links)
	}
	b.WriteString(containerClose[container])
}

// Quill wraps each run of list items or code lines in one container element.
var (
	containerOpen = map[string]string{
		"list":       "<ol>",
		"code-block": `<div class="ql-code-block-container" spellcheck="false">`,
	}
	containerClose = map[string]string{
		"list":       "</ol>",
		"code-block": "</div>",
	}
)

func containerOf(l line) string {
	switch {
	case l.format["list"] != nil:
		return "list"
	case l.format["code-block"] != nil:
		return "code-block"
	}
	return ""
}

// headerTags maps the header values [CheckDocument] accepts to elements.
var headerTags = map[any]string{1.0: "h1", 2.0: "h2", 3.0: "h3"}

func writeLine(b *strings.Builder, l line, links bool) {
	tag := "p"
	switch {
	case l.format["header"] != nil:
		tag = headerTags[l.format["header"]]
	case l.format["list"] != nil:
		tag = "li"
	case l.format["blockquote"] != nil:
		tag = "blockquote"
	case l.format["code-block"] != nil:
		tag = "div"
	}
	b.WriteString("<")
	b.WriteString(tag)
	if list, ok := l.format["list"].(string); ok {
		b.WriteString(` data-list="`)
		escape(b, list)
		b.WriteString(`"`)
	}
	writeClasses(b, tag == "div", l.format)
	b.WriteString(">")
	if tag == "li" {
		// Quill CSS draws the list marker here.
		b.WriteString(`<span class="ql-ui" contenteditable="false"></span>`)
	}
	if len(l.runs) == 0 {
		b.WriteString("<br>")
	}
	for _, r := range l.runs {
		if tag == "div" {
			// Code lines have no text formats in Quill.
			writeContent(b, r)
			continue
		}
		writeRun(b, r, links)
	}
	b.WriteString("</")
	b.WriteString(tag)
	b.WriteString(">")
}

// writeClasses writes the class attribute of a line, if any.
func writeClasses(b *strings.Builder, code bool, format delta.Attributes) {
	align, hasAlign := format["align"].(string)
	indent, hasIndent := format["indent"].(float64)
	if !code && !hasAlign && !hasIndent {
		return
	}
	b.WriteString(` class="`)
	sep := ""
	if code {
		b.WriteString("ql-code-block")
		sep = " "
	}
	if hasAlign {
		b.WriteString(sep)
		b.WriteString("ql-align-")
		b.WriteString(align)
		sep = " "
	}
	if hasIndent {
		b.WriteString(sep)
		b.WriteString("ql-indent-")
		b.WriteString(strconv.Itoa(int(indent)))
	}
	b.WriteString(`"`)
}

// inlineTags lists the elements of the text formats from outermost to
// innermost, as Inline.order of Quill nests them.
var inlineTags = [...]struct{ format, open, close string }{
	{"code", "<code>", "</code>"},
	{"bold", "<strong>", "</strong>"},
	{"italic", "<em>", "</em>"},
	{"strike", "<s>", "</s>"},
	{"underline", "<u>", "</u>"},
}

func writeRun(b *strings.Builder, r run, links bool) {
	// closing holds the end tags of the opened elements: at most every inline
	// tag, a link and a span.
	var closing [len(inlineTags) + 2]string
	n := 0
	for _, t := range inlineTags {
		if r.format[t.format] != nil {
			b.WriteString(t.open)
			closing[n] = t.close
			n++
		}
	}
	if link, ok := r.format["link"].(string); ok && links {
		b.WriteString(`<a href="`)
		escape(b, link)
		b.WriteString(`" rel="noopener noreferrer" target="_blank">`)
		closing[n] = "</a>"
		n++
	}
	color, hasColor := r.format["color"].(string)
	background, hasBackground := r.format["background"].(string)
	if hasColor || hasBackground {
		b.WriteString(`<span style="`)
		if hasColor {
			writeColor(b, "color", "--color", color)
		}
		if hasColor && hasBackground {
			b.WriteString(" ")
		}
		if hasBackground {
			writeColor(b, "background-color", "--background", background)
		}
		b.WriteString(`">`)
		closing[n] = "</span>"
		n++
	}
	writeContent(b, r)
	for _, c := range slices.Backward(closing[:n]) {
		b.WriteString(c)
	}
}

// writeContent writes the text of r, or the img element of its image.
func writeContent(b *strings.Builder, r run) {
	if r.embed.Type == "image" {
		b.WriteString(`<img src="`)
		escape(b, r.embed.Value)
		if width, ok := r.format["width"].(string); ok {
			b.WriteString(`" width="`)
			escape(b, width)
		}
		b.WriteString(`">`)
		return
	}
	escape(b, r.text)
}

// writeColor writes color to property and to custom, the custom property the
// editor writes. The stylesheet computes dark mode colors from custom.
func writeColor(b *strings.Builder, property, custom, color string) {
	b.WriteString(property)
	b.WriteString(": ")
	escape(b, color)
	b.WriteString("; ")
	b.WriteString(custom)
	b.WriteString(": ")
	escape(b, color)
	b.WriteString(";")
}

// htmlEscaper escapes the same characters as [html.EscapeString], the same way.
var htmlEscaper = strings.NewReplacer(
	`&`, "&amp;", `'`, "&#39;", `<`, "&lt;", `>`, "&gt;", `"`, "&#34;",
)

// escape writes s escaped for HTML text and attributes, without the copy
// [html.EscapeString] makes. Writing to a [strings.Builder] does not fail.
func escape(b *strings.Builder, s string) { _, _ = htmlEscaper.WriteString(b, s) }
