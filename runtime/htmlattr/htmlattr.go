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

// WritePathValue writes v as a URL path segment inside a JavaScript string
// in an HTML attribute. Percent encoding keeps v inside the string and the
// segment; HTML escaping keeps the result inside the attribute.
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
	_, _ = io.WriteString(w, SignalString(s))
}

// SignalString returns s escaped the way [WriteSignalString] writes it.
// The generator calls it on what it knows at generation time,
// a query parameter name for one, and writes the result as a literal.
func SignalString(s string) string {
	return html.EscapeString(signalStringEscaper.Replace(s))
}

// WriteSignalValue writes a number or boolean inside a data-signals attribute.
func WriteSignalValue(w io.Writer, s string) {
	_, _ = io.WriteString(w, html.EscapeString(s))
}
