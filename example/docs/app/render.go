package app

import (
	"context"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/a-h/templ"

	"github.com/romshark/datapages/example/docs/delta"
	"github.com/romshark/datapages/example/docs/richtext"
	"github.com/romshark/datapages/example/docs/store"
)

// richtextHTML renders a document as editor markup. [richtext.WriteHTML]
// escapes the text and writes only formats [richtext.CheckDocument] accepted.
func richtextHTML(doc delta.Delta) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return richtext.WriteHTML(w, doc)
	})
}

// initial returns the first letter of the last word of a name:
// "C" for "Anonymous Capybara".
func initial(name string) string {
	last := name[strings.LastIndexByte(name, ' ')+1:]
	r, _ := utf8.DecodeRuneInString(last)
	return string(r)
}

func collaboratorTitle(c store.Collaborator) string {
	if c.You {
		return c.Name + " (you)"
	}
	return c.Name
}
