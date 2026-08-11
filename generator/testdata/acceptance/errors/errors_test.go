// Drives the generated error handling of ./app.

package acceptance_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dpacceptance/app"
	"dpacceptance/datapagesgen"

	"github.com/romshark/datapages/modules/msgbroker/inmem"
)

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(datapagesgen.NewServer(&app.App{}, inmem.New(8)))
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, srv *httptest.Server, path string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatalf("building GET %s: %v", path, err)
	}
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return resp.StatusCode, string(b)
}

func post(t *testing.T, srv *httptest.Server, path string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, srv.URL+path, nil)
	if err != nil {
		t.Fatalf("building POST %s: %v", path, err)
	}
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
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

// TestNotFoundPage covers the page the app supplies for a URL no page claims.
// The page is rendered rather than a bare status line, and it is given the
// path that was asked for.
//
// The status it arrives with is a separate matter; see the
// bug_error_page_status case.
func TestNotFoundPage(t *testing.T) {
	tests := map[string]string{
		"unknown url":                "/no-such-page/",
		"the error page's own route": "/not-found/",
	}

	for name, path := range tests {
		t.Run(name, func(t *testing.T) {
			srv := newServer(t)
			_, body := get(t, srv, path)
			if got, want := echoed(t, body), "not found: "+path; got != want {
				t.Errorf(" got: %s\nwant: %s", got, want)
			}
		})
	}
}

// TestServerErrorPageRoute covers the 500 page reached by its own route. What
// happens to it when a handler fails is a separate matter; see the
// bug_error500_without_recover case.
func TestServerErrorPageRoute(t *testing.T) {
	srv := newServer(t)

	status, body := get(t, srv, "/server-error/")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if got, want := echoed(t, body), "server error"; got != want {
		t.Errorf(" got: %s\nwant: %s", got, want)
	}
}

// TestFailedPageLoad covers a page load whose handler fails. Whatever the
// visitor is given, it cannot be the error the handler produced: that text is
// written for the operator's log.
func TestFailedPageLoad(t *testing.T) {
	srv := newServer(t)

	status, body := get(t, srv, "/boom/")
	if status != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", status, http.StatusInternalServerError)
	}
	if strings.Contains(body, "the page could not be built") {
		t.Errorf("the error message reached the visitor:\n%s", body)
	}
}

// TestActionErrorStatus covers the status an action's error becomes. The
// sentinels are the only way an application chooses it, and each one must
// arrive as itself.
func TestActionErrorStatus(t *testing.T) {
	tests := map[string]struct {
		path string
		want int
	}{
		"plain error":      {"/boom/plain/", http.StatusInternalServerError},
		"bad request":      {"/boom/bad/", http.StatusBadRequest},
		"forbidden":        {"/boom/forbidden/", http.StatusForbidden},
		"not found":        {"/boom/not-found/", http.StatusNotFound},
		"conflict":         {"/boom/conflict/", http.StatusConflict},
		"wrapped sentinel": {"/boom/wrapped/", http.StatusNotFound},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			srv := newServer(t)
			status, body := post(t, srv, tt.path)
			if status != tt.want {
				t.Errorf("status = %d, want %d\n%s", status, tt.want, body)
			}
			if strings.Contains(body, "no such item") ||
				strings.Contains(body, "something went wrong") {
				t.Errorf("the error message reached the client:\n%s", body)
			}
		})
	}
}

// TestFailedPageLoadIsNotCached covers the response of a failed page load. A
// cached 500 outlives the failure that caused it.
func TestFailedPageLoadIsNotCached(t *testing.T) {
	srv := newServer(t)

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, srv.URL+"/boom/", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /boom/: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if cc := resp.Header.Get("Cache-Control"); strings.Contains(cc, "max-age") &&
		!strings.Contains(cc, "max-age=0") {
		t.Errorf("Cache-Control = %q on a failed page load", cc)
	}
}
