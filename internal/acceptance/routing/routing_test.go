// Drives the generated router of ./app over HTTP.

package acceptance_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/internal/acceptance/routing/app"
	"github.com/romshark/datapages/internal/acceptance/routing/datapagesgen"
	"github.com/romshark/datapages/internal/acceptance/routing/datapagesgen/href"
	"github.com/romshark/datapages/modules/msgbroker/inmem"
)

func newClient(t *testing.T) *client.Client {
	t.Helper()
	return client.New(t, datapagesgen.NewServer(&app.App{}, inmem.New(8)))
}

// TestRoundTrip covers the pair the generator writes for every page:
// the function that builds the URL and the handler that takes it apart.
// They are generated from one model and are asserted here against each other.
// A change of order, name or number format in one of them cannot pass unnoticed.
func TestRoundTrip(t *testing.T) {
	c := newClient(t)

	tests := map[string]struct {
		url  string
		want string
	}{
		"index": {
			url:  href.PageIndex(),
			want: "index",
		},
		"path of every kind": {
			url:  href.PagePath("hello", -42, 18446744073709551615, 2.5, true),
			want: `s="hello" i=-42 u=18446744073709551615 f=2.5 b=true`,
		},
		"sized integers": {
			url: href.PageInts(
				-128, -32768, -2147483648, -9223372036854775808, 255, 65535, 4294967295,
			),
			want: "i8=-128 i16=-32768 i32=-2147483648 i64=-9223372036854775808 " +
				"u8=255 u16=65535 u32=4294967295",
		},
		"query of every kind": {
			url: href.PageQuery(href.QueryPageQuery{
				Term:  "golang",
				Limit: 25,
				Ratio: 0.5,
				Score: -1.25,
				Big:   4294967295,
				Deep:  -9223372036854775808,
				Flag:  true,
			}),
			want: `term="golang" limit=25 ratio=0.5 score=-1.25 ` +
				"big=4294967295 deep=-9223372036854775808 flag=true",
		},
		"path and query together": {
			url: href.PageMixed("acme", 7, href.QueryPageMixed{
				Tab:  "open",
				Page: 3,
			}),
			want: `org="acme" id=7 tab="open" page=3`,
		},
		"path variables that collide by name": {
			url:  href.PageConflict(1, 2, "three"),
			want: `value=1 s_value=2 s_s_value="three"`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			resp := c.Get(t, tt.url)
			require.Equal(t, http.StatusOK, resp.Status, resp.Body)
			require.Equal(t, tt.want, resp.Element(t, "echo"))
		})
	}
}

// TestQueryDefaults covers a query string that leaves everything out.
// Absent parameters are the common case on a first page load and must not be an error.
func TestQueryDefaults(t *testing.T) {
	c := newClient(t)

	resp := c.Get(t, "/q/")
	require.Equal(t, http.StatusOK, resp.Status, resp.Body)
	const want = `term="" limit=0 ratio=0 score=0 big=0 deep=0 flag=false`
	require.Equal(t, want, resp.Element(t, "echo"))
}

// TestUnparsableValues covers URLs whose values do not fit the declared type.
// The handler must not run, and the client must learn that it sent the bad
// request rather than that the server broke.
func TestUnparsableValues(t *testing.T) {
	c := newClient(t)

	tests := map[string]string{
		"path int is a word":       "/p/x/notanint/1/1.0/true/",
		"path uint is negative":    "/p/x/1/-1/1.0/true/",
		"path float is a word":     "/p/x/1/1/nope/true/",
		"path bool is a number":    "/p/x/1/1/1.0/7/",
		"path int8 overflows":      "/ints/128/0/0/0/0/0/0/",
		"path int16 overflows":     "/ints/0/32768/0/0/0/0/0/",
		"path int32 overflows":     "/ints/0/0/2147483648/0/0/0/0/",
		"path int64 overflows":     "/ints/0/0/0/9223372036854775808/0/0/0/",
		"path uint8 overflows":     "/ints/0/0/0/0/256/0/0/",
		"path uint16 overflows":    "/ints/0/0/0/0/0/65536/0/",
		"path uint32 overflows":    "/ints/0/0/0/0/0/0/4294967296/",
		"path uint16 is negative":  "/ints/0/0/0/0/0/-1/0/",
		"path int32 is a word":     "/ints/0/0/nope/0/0/0/0/",
		"query int is a word":      "/q/?limit=many",
		"query float is a word":    "/q/?score=high",
		"query float32 is a word":  "/q/?ratio=high",
		"query int64 is a word":    "/q/?deep=deep",
		"query bool is a word":     "/q/?flag=yes-please",
		"query uint32 is negative": "/q/?big=-1",
		"reflected int is a word":  "/reflect/?p=many",
	}

	for name, url := range tests {
		t.Run(name, func(t *testing.T) {
			resp := c.Get(t, url)
			require.Equal(t, http.StatusBadRequest, resp.Status,
				"GET %s\n%s", url, resp.Body)
		})
	}
}

// TestPageHead covers a page that returns its own head.
//
// The head is where a page carries its title and its meta tags.
// The shell renders it rather than the page.
// The handler hands it back and the server has to put it in the document,
// inside <head> and before the resp.Body.
func TestPageHead(t *testing.T) {
	c := newClient(t)

	resp := c.Get(t, href.PageTitled("welcome"))
	require.Equal(t, http.StatusOK, resp.Status, resp.Body)
	require.Equal(t, "titled welcome", resp.Element(t, "echo"))

	head := strings.Index(resp.Body, "<title>welcome</title>")
	require.GreaterOrEqual(t, head, 0,
		"the head the page returned is not in the document:\n%s", resp.Body)
	require.Less(t, head, strings.Index(resp.Body, "<body"),
		"the page's head was rendered after <body>:\n%s", resp.Body)
}

// TestReflectedSignals covers a query parameter bound to a signal.
//
// The binding is two things the page carries rather than anything the handler returns:
// the signal's initial value, taken from the URL, and the code that writes it back into
// the URL when it changes. A page that renders correctly and carries neither
// leaves the client without state and the URL without the current values.
func TestReflectedSignals(t *testing.T) {
	c := newClient(t)

	resp := c.Get(t, "/reflect/?t=shoes&p=3")
	require.Equal(t, http.StatusOK, resp.Status, resp.Body)
	require.Equal(t, `term="shoes" page=3`, resp.Element(t, "echo"))
	for _, want := range []string{
		`data-signals:term="'shoes'"`,
		`data-signals:page="3"`,
		"window.history.replaceState",
		"params.set('t'",
		"params.set('p'",
	} {
		require.Contains(t, resp.Body, want, "the page does not carry %q")
	}
}

// TestEncodedPathSegments covers values that had to be
// percent-encoded to fit in a URL at all.
//
// The server rewrites a path that does not end in a slash before routing it.
// net/url holds an encoded path twice, once decoded and once raw.
// A rewrite that updates one and not the other routes the request by
// one path and reads its variables out of the other.
func TestEncodedPathSegments(t *testing.T) {
	c := newClient(t)

	tests := map[string]struct{ url, want string }{
		"encoded space": {
			"/p/a%20b/1/2/3.5/true/", `s="a b" i=1 u=2 f=3.5 b=true`,
		},
		"encoded space without the trailing slash": {
			"/p/a%20b/1/2/3.5/true", `s="a b" i=1 u=2 f=3.5 b=true`,
		},
		"encoded slash": {
			"/p/a%2Fb/1/2/3.5/true/", `s="a/b" i=1 u=2 f=3.5 b=true`,
		},
		// The rewrite has to update both forms of the path. Updating the
		// decoded one alone routes the request by a path with one more
		// segment than the URL has.
		"encoded slash without the trailing slash": {
			"/p/a%2Fb/1/2/3.5/true", `s="a/b" i=1 u=2 f=3.5 b=true`,
		},
		"encoded percent": {
			"/p/100%25/1/2/3.5/true/", `s="100%" i=1 u=2 f=3.5 b=true`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			resp := c.Get(t, tt.url)
			require.Equal(t, http.StatusOK, resp.Status, resp.Body)
			require.Equal(t, tt.want, resp.Element(t, "echo"))
		})
	}
}

// TestUnknownRoutes covers paths no page claims.
func TestUnknownRoutes(t *testing.T) {
	c := newClient(t)

	for name, url := range map[string]string{
		"no such page":            "/nothing/",
		"too few path segments":   "/p/x/1/1/",
		"too many path segments":  "/p/x/1/1/1.0/true/extra/",
		"prefix of a real route":  "/p/",
		"page path without slash": "/nothing",
	} {
		t.Run(name, func(t *testing.T) {
			resp := c.Get(t, url)
			require.Equal(t, http.StatusNotFound, resp.Status, "GET %s", url)
		})
	}
}

// TestHrefLiterals covers the URLs themselves. The round trip above proves the
// two generated halves agree; these assertions pin down what they agree on,
// because a self-consistent pair can still address the wrong thing.
func TestHrefLiterals(t *testing.T) {
	tests := map[string]struct{ got, want string }{
		"index":           {href.PageIndex(), "/"},
		"path variables":  {href.PagePath("a", 1, 2, 3.5, false), "/p/a/1/2/3.5/false/"},
		"empty query":     {href.PageQuery(href.QueryPageQuery{}), "/q/"},
		"one query param": {href.PageQuery(href.QueryPageQuery{Limit: 5}), "/q/?limit=5"},
		"two query params": {
			href.PageQuery(href.QueryPageQuery{Term: "x", Limit: 5}),
			"/q/?term=x&limit=5",
		},
		"path and query": {
			href.PageMixed("acme", 7, href.QueryPageMixed{Tab: "open"}),
			"/org/acme/item/7/?tab=open",
		},
		"negative numbers": {
			href.PagePath("a", -1, 2, -3.5, false),
			"/p/a/-1/2/-3.5/false/",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.got)
		})
	}
}
