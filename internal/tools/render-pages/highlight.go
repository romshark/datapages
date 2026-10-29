package main

import (
	"context"
	"html"
	"io"
	"regexp"
	"strconv"

	"github.com/alecthomas/chroma/v3"
	chromahtml "github.com/alecthomas/chroma/v3/formatters/html"
	"github.com/alecthomas/chroma/v3/lexers"
	"github.com/alecthomas/chroma/v3/styles"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/renderer"
	goldhtml "github.com/yuin/goldmark/v2/renderer/html"
)

// codeFormatter writes tokens as <span> elements with the short class names of
// Chroma, such as "k" for a keyword, which style.css colors.
var codeFormatter = chromahtml.New(
	chromahtml.WithClasses(true),
	chromahtml.PreventSurroundingPre(true),
)

// codeTitle finds the title in the info string of a code block, as in
// ```go title="app/app.go". GitHub reads the first word of the info string as
// the language and ignores the rest.
var codeTitle = regexp.MustCompile(`\btitle="([^"]*)"`)

// codeBlocks returns the renderer of the code blocks of 1 page. It shows the
// title of a block above it and a button that copies it, and highlights it in
// the language of its info string if Chroma has a lexer for that language.
// A block in the language mermaid is a diagram, which docs.js draws, as GitHub does.
//
// It is a decorator because goldmark applies the renderers of its extensions,
// CommonMark among them, after a renderer given as an option. The blocks of a page
// are numbered for the copy buttons, which makes the renderer valid for 1 page only.
func codeBlocks() func(goldhtml.NodeRenderer) goldhtml.NodeRenderer {
	n := 0
	return func(goldhtml.NodeRenderer) goldhtml.NodeRenderer {
		return renderer.NodeRendererFunc(func(
			w io.Writer, source []byte, node ast.Node, entering bool, _ renderer.Context,
		) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			b := node.(*ast.CodeBlock)
			if lang, _ := b.Language(source); lang == "mermaid" {
				_, err := io.WriteString(w, `<pre class="mermaid">`+
					html.EscapeString(b.Value.Str(source))+"</pre>\n")
				return ast.WalkSkipChildren, err
			}
			n++
			// goldmark writes no "_" into the ID of a heading, which keeps this
			// ID apart from the heading IDs of the page.
			id := "code_" + strconv.Itoa(n)
			button := func() error {
				return copyButton(id).Render(context.Background(), w)
			}
			// The button of a block with a title is in the title, which
			// centers it on the title.
			title := codeTitle.FindStringSubmatch(b.Info.Value(source))
			_, _ = io.WriteString(w, `<div class="code-block">`)
			if title != nil {
				_, _ = io.WriteString(w,
					`<div class="code-title">`+html.EscapeString(title[1]))
				if err := button(); err != nil {
					return ast.WalkStop, err
				}
				_, _ = io.WriteString(w, `</div>`)
			}
			_, _ = io.WriteString(w, `<div class="code-body" id="`+id+`">`)
			if err := writeCode(w, b, source); err != nil {
				return ast.WalkStop, err
			}
			_, _ = io.WriteString(w, `</div>`)
			if title == nil {
				if err := button(); err != nil {
					return ast.WalkStop, err
				}
			}
			_, _ = io.WriteString(w, "</div>\n")
			return ast.WalkSkipChildren, nil
		})
	}
}

// writeCode writes b as a <pre> element, highlighted in the language of its
// info string if Chroma has a lexer for that language.
func writeCode(w io.Writer, b *ast.CodeBlock, source []byte) error {
	code := b.Value.Str(source)
	lang, _ := b.Language(source)
	if lang == "" {
		_, err := io.WriteString(w, "<pre><code>"+html.EscapeString(code)+"</code></pre>")
		return err
	}
	_, _ = io.WriteString(w, `<pre><code class="language-`+html.EscapeString(lang)+`">`)
	if lexer := lexers.Get(lang); lexer == nil {
		_, _ = io.WriteString(w, html.EscapeString(code))
	} else {
		tokens, err := chroma.Coalesce(lexer).Tokenise(nil, code)
		if err != nil {
			return err
		}
		// The style only sets colors, which the classes leave to style.css.
		if err := codeFormatter.Format(w, styles.Fallback, tokens); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "</code></pre>")
	return err
}
