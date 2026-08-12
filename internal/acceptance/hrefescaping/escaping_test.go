// Asserts the round trip href has to provide.
// A value handed to a URL builder is the value the handler parses back.

package acceptance_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/internal/acceptance/hrefescaping/app"
	"github.com/romshark/datapages/internal/acceptance/hrefescaping/datapagesgen"
	"github.com/romshark/datapages/internal/acceptance/hrefescaping/datapagesgen/action"
	"github.com/romshark/datapages/internal/acceptance/hrefescaping/datapagesgen/href"
	"github.com/romshark/datapages/modules/msgbroker/inmem"
)

// TestHrefEscaping hands a value with a URL separator in it to a generated
// builder and asks the server what it received.
func TestHrefEscaping(t *testing.T) {
	c := client.New(t, datapagesgen.NewServer(&app.App{}, inmem.New(8)))

	tests := map[string]struct {
		url  string
		want string
	}{
		"slash in a path value": {
			url:  href.PageItem("a/b"),
			want: `name="a/b"`,
		},
		"question mark in a path value": {
			url:  href.PageItem("a?b"),
			want: `name="a?b"`,
		},
		"hash in a path value": {
			url:  href.PageItem("a#b"),
			want: `name="a#b"`,
		},
		"ampersand in a query value": {
			url:  href.PageSearch(href.QueryPageSearch{Term: "a&page=99", Page: 1}),
			want: `term="a&page=99" page=1`,
		},
		"hash in a query value": {
			url:  href.PageSearch(href.QueryPageSearch{Term: "a#b", Page: 1}),
			want: `term="a#b" page=1`,
		},
		"space and plus in a query value": {
			url:  href.PageSearch(href.QueryPageSearch{Term: "a b+c", Page: 2}),
			want: `term="a b+c" page=2`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			resp := c.Get(t, tt.url)
			require.Equal(t, http.StatusOK, resp.Status, "GET %s", tt.url)
			require.Equal(t, tt.want, resp.Element(t, "echo"), "GET %s", tt.url)
		})
	}
}

// TestActionURLEscaping covers the same round trip through an action expression,
// which carries its URL the same way a link does.
func TestActionURLEscaping(t *testing.T) {
	c := client.New(t, datapagesgen.NewServer(&app.App{}, inmem.New(8)))

	expr := action.POSTPageItemRename("a/b",
		action.QueryPOSTPageItemRename{To: "c&d=e"})

	// The expression a template carries: @post('<url>').
	const prefix, suffix = "@post('", "')"
	require.True(t, strings.HasPrefix(expr, prefix), expr)
	require.True(t, strings.HasSuffix(expr, suffix), expr)
	url := strings.TrimSuffix(strings.TrimPrefix(expr, prefix), suffix)

	resp := c.Action(t, http.MethodPost, url, "")
	require.Equal(t, http.StatusOK, resp.Status, url)
	require.Contains(t, resp.Body, `renamed "a/b" to "c&d=e"`,
		"the values did not survive the action URL")
}
