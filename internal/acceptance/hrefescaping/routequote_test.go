// Asserts that a route carrying a quote reaches the browser as the route.
// The generator writes route literals into single-quoted JavaScript strings,
// which a raw quote ends.

package acceptance_test

import (
	"html"
	"mime"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/hrefescaping/app/datapagesgen/action"
	"github.com/romshark/datapages/internal/acceptance/hrefescaping/app/datapagesgen/href"
)

// TestRouteQuoteStreamURL tests the data-init attribute of a page whose route
// carries a quote. The browser decodes the attribute and Datastar reads the URL
// from the JavaScript string, which has to hold the whole stream path.
func TestRouteQuoteStreamURL(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	tests := map[string]struct {
		page   string
		stream string
	}{
		"static route": {
			href.PageQuoted(href.QueryPageQuoted{}), "/o'reilly/_$/",
		},
		"path variable": {
			href.PageQuotedItem("x", href.QueryPageQuotedItem{}), "/o'reilly/x/_$/",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			tab := v.Load(t, tt.page)

			expr := html.UnescapeString(attrValue(t, tab.Page().Body, "data-init"))
			url, rest := jsString(t, strings.TrimPrefix(expr, "@get("))
			require.Equal(t, tt.stream, url, expr)
			require.True(t, strings.HasPrefix(rest, ",{"),
				"the stream URL ends before its string does: %s", expr)

			// Tab.Connect would open a URL built from the page path instead of this one.
			resp, err := v.Client().Do(tab.NewRequest(t, http.MethodGet, url, nil))
			require.NoError(t, err, "opening stream %s", url)
			defer func() { _ = resp.Body.Close() }()
			require.Equal(t, http.StatusOK, resp.StatusCode, "opening stream %s", url)
			mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
			require.Equal(t, "text/event-stream", mt, "opening stream %s", url)
		})
	}
}

// TestRouteQuoteURLSync tests the data-effect attribute that writes a reflected
// signal into the address bar of a page whose route carries a quote.
// Both strings it builds the URL from have to hold the route.
func TestRouteQuoteURLSync(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	tests := map[string]struct {
		page string
		path string
	}{
		"static route":  {href.PageQuoted(href.QueryPageQuoted{}), "/o'reilly"},
		"path variable": {href.PageQuotedItem("x", href.QueryPageQuotedItem{}), "/o'reilly/x"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			resp := v.Get(t, tt.page)
			require.Equal(t, http.StatusOK, resp.Status, resp.Body)

			script := html.UnescapeString(attrValue(t, resp.Body, "data-effect"))
			// (query ? '<path>?' + query : '<path>')
			const call = "replaceState(null, '', (query ? "
			i := strings.Index(script, call)
			require.GreaterOrEqual(t, i, 0, "no replaceState call: %s", script)
			withQuery, rest := jsString(t, script[i+len(call):])
			require.Equal(t, tt.path+"?", withQuery, script)

			const sep = " + query : "
			require.True(t, strings.HasPrefix(rest, sep),
				"the URL ends before its string does: %s", script)
			withoutQuery, rest := jsString(t, rest[len(sep):])
			require.Equal(t, tt.path, withoutQuery, script)
			require.True(t, strings.HasPrefix(rest, ")"),
				"the URL ends before its string does: %s", script)
		})
	}
}

// TestRouteQuoteActionURL tests the action expressions of a page whose route
// carries a quote, in each shape the action package writes: with or without
// path variables and a query.
func TestRouteQuoteActionURL(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)
	quoted := href.PageQuoted(href.QueryPageQuoted{})
	quotedItem := href.PageQuotedItem("x", href.QueryPageQuotedItem{})

	tests := map[string]struct {
		page string // the page whose tab sends the action
		expr string
		echo string
	}{
		"route only": {quoted, action.PageQuoted.Ping.POST(), "pinged"},
		"query": {
			quoted,
			action.PageQuoted.Find.POST(action.PageQuoted.Find.POSTQuery("t")),
			`found "t"`,
		},
		"path variable": {
			quotedItem, action.PageQuotedItem.Rename.POST("x"), `renamed "x"`,
		},
		"path variable and query": {
			quotedItem,
			action.PageQuotedItem.Move.POST("x",
				action.PageQuotedItem.Move.POSTQuery("y")),
			`moved "x" to "y"`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			url, rest := jsString(t, strings.TrimPrefix(tt.expr, "@post("))
			require.Equal(t, ")", rest,
				"the action URL ends before its string does: %s", tt.expr)

			resp := v.Load(t, tt.page).Do(t, tt.expr, nil)
			require.Equal(t, http.StatusOK, resp.Status, "POST %s", url)
			require.Contains(t, resp.Body, tt.echo, "POST %s", url)
		})
	}
}

// attrValue returns the raw value of the attribute name in body,
// still escaped for HTML.
func attrValue(t *testing.T, body, name string) string {
	t.Helper()
	open := " " + name + `="`
	i := strings.Index(body, open)
	require.GreaterOrEqual(t, i, 0, "no %s attribute:\n%s", name, body)
	v := body[i+len(open):]
	j := strings.IndexByte(v, '"')
	require.GreaterOrEqual(t, j, 0, "unterminated %s attribute:\n%s", name, body)
	return v[:j]
}

// jsString reads the single-quoted JavaScript string s starts with and returns
// its value and what follows it. It reads the escapes the generator writes,
// a backslash before the character it escapes.
func jsString(t *testing.T, s string) (value, rest string) {
	t.Helper()
	require.True(t, strings.HasPrefix(s, "'"), "no string at the start of %q", s)
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
			require.Less(t, i, len(s), "unterminated escape in %q", s)
			b.WriteByte(s[i])
		case '\'':
			return b.String(), s[i+1:]
		default:
			b.WriteByte(s[i])
		}
	}
	t.Fatalf("unterminated string %q", s)
	return "", ""
}
