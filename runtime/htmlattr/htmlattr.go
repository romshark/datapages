// Package htmlattr escapes values written into Datastar attributes.
// A value is escaped for its reader first and for the attribute second,
// because the browser decodes the attribute before Datastar evaluates it.
//
// Application code must not import this package.
package htmlattr

import (
	"html"
	"io"
	"net/url"
	"strings"
)

// WritePathValue writes v into the @get URL of a data-init attribute.
// Percent encoding removes the quotes that would end the JavaScript string,
// HTML escaping removes the ampersand that url.PathEscape keeps.
func WritePathValue(w io.Writer, v string) {
	_, _ = io.WriteString(w, html.EscapeString(url.PathEscape(v)))
}

var signalStringEscaper = strings.NewReplacer(
	"\\", `\\`,
	"'", `\'`,
	"\n", `\n`,
	"\r", `\r`,
)

// WriteSignalString writes s as a quoted string inside a data-signals attribute.
// It escapes s for the JavaScript string first and for the attribute second.
func WriteSignalString(w io.Writer, s string) {
	_, _ = io.WriteString(w, html.EscapeString(signalStringEscaper.Replace(s)))
}

// WriteSignalValue writes a number or boolean inside a data-signals attribute.
func WriteSignalValue(w io.Writer, s string) {
	_, _ = io.WriteString(w, html.EscapeString(s))
}
