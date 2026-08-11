// Asserts the round trip href has to provide. A value handed to a URL builder
// is the value the handler parses back.

package acceptance_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dpacceptance/app"
	"dpacceptance/datapagesgen"
	"dpacceptance/datapagesgen/href"

	"github.com/romshark/datapages/modules/msgbroker/inmem"
)

// TestHrefEscaping is the recorded failure. Every subtest hands a value with
// a URL separator in it to a generated builder and asks the server what it
// received.
func TestHrefEscaping(t *testing.T) {
	srv := httptest.NewServer(datapagesgen.NewServer(&app.App{}, inmem.New(8)))
	t.Cleanup(srv.Close)

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
		"ampersand in a query value": {
			url:  href.PageSearch(href.QueryPageSearch{Term: "a&page=99", Page: 1}),
			want: `term="a&page=99" page=1`,
		},
		"hash in a query value": {
			url:  href.PageSearch(href.QueryPageSearch{Term: "a#b", Page: 1}),
			want: `term="a#b" page=1`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			status, body := get(t, srv, tt.url)
			if status != http.StatusOK {
				t.Fatalf("GET %s: status = %d, want 200", tt.url, status)
			}
			if got := echoed(t, body); got != tt.want {
				t.Errorf("GET %s\n got: %s\nwant: %s", tt.url, got, tt.want)
			}
		})
	}
}

func get(t *testing.T, srv *httptest.Server, url string) (int, string) {
	t.Helper()
	resp, err := srv.Client().Get(srv.URL + url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body of %s: %v", url, err)
	}
	return resp.StatusCode, string(b)
}

func echoed(t *testing.T, body string) string {
	t.Helper()
	const open = `<pre id="echo">`
	i := strings.Index(body, open)
	if i < 0 {
		t.Fatalf("no echo element in response:\n%s", body)
	}
	rest := body[i+len(open):]
	j := strings.Index(rest, "</pre>")
	if j < 0 {
		t.Fatalf("unterminated echo element in response:\n%s", body)
	}
	return rest[:j]
}
