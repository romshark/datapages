// Drives the return values of a GET handler other than its body.

package acceptance_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/romshark/datapages/internal/acceptance/getoptions/app"
	"github.com/romshark/datapages/internal/acceptance/getoptions/datapagesgen"
	"github.com/romshark/datapages/modules/msgbroker/inmem"
)

// reloadAttr is what the server writes on the body so that a tab reloads the
// page when it becomes visible again. The two streaming flags exist to suppress it.
const reloadAttr = "data-on:visibilitychange"

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(datapagesgen.NewServer(&app.App{}, inmem.New(8)))
	t.Cleanup(srv.Close)
	return srv
}

// get performs a page load without following redirects.
func get(t *testing.T, srv *httptest.Server, path string) (*http.Response, string) {
	t.Helper()
	client := *srv.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, srv.URL+path, nil,
	)
	if err != nil {
		t.Fatalf("building GET %s: %v", path, err)
	}
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	b, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return resp, string(b)
}

// TestRedirectFromPageLoad covers a GET that returns a redirect.
// The visitor is sent elsewhere and the body the handler also returned is not rendered.
func TestRedirectFromPageLoad(t *testing.T) {
	srv := newServer(t)

	resp, body := get(t, srv, "/gone/")
	if resp.StatusCode < 300 || resp.StatusCode > 399 {
		t.Fatalf("status = %d, want a redirect\n%s", resp.StatusCode, body)
	}
	if got, want := resp.Header.Get("Location"), "/"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
	if strings.Contains(body, "never rendered") {
		t.Errorf("the body was rendered alongside the redirect:\n%s", body)
	}
}

// TestRedirectStatusFromPageLoad covers a handler that chooses the status,
// and the same handler when it decides not to redirect at all.
func TestRedirectStatusFromPageLoad(t *testing.T) {
	srv := newServer(t)

	t.Run("redirecting", func(t *testing.T) {
		resp, _ := get(t, srv, "/maybe/?go=true")
		if resp.StatusCode != http.StatusMovedPermanently {
			t.Errorf("status = %d, want %d",
				resp.StatusCode, http.StatusMovedPermanently)
		}
		if got, want := resp.Header.Get("Location"), "/"; got != want {
			t.Errorf("Location = %q, want %q", got, want)
		}
	})

	t.Run("not redirecting", func(t *testing.T) {
		resp, body := get(t, srv, "/maybe/")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		if !strings.Contains(body, "stayed") {
			t.Errorf("the page did not render:\n%s", body)
		}
		if resp.Header.Get("Location") != "" {
			t.Error("a page that did not redirect sent a Location header")
		}
	})
}

// TestVisibilityReload covers the two flags that decide whether a hidden tab
// reloads the page when it comes back.
//
// Both suppress the same body attribute. A page that keeps its stream running
// in the background must not reload, and a page that asks not to reload must not either.
func TestVisibilityReload(t *testing.T) {
	srv := newServer(t)

	tests := map[string]struct {
		path       string
		wantReload bool
	}{
		"the plain page reloads":             {"/", true},
		"background streaming suppresses it": {"/background/", false},
		"disabled refresh suppresses it":     {"/no-refresh/", false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			resp, body := get(t, srv, tt.path)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			if got := strings.Contains(body, reloadAttr); got != tt.wantReload {
				t.Errorf("the page carries %s: %v, want %v\n%s",
					reloadAttr, got, tt.wantReload, body)
			}
		})
	}
}
