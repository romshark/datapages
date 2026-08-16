// Drives the generated error recovery of ./app.

package acceptance_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/romshark/datapages/internal/acceptance/recovererror/app"
	"github.com/romshark/datapages/internal/acceptance/recovererror/datapagesgen"
	"github.com/romshark/datapages/modules/msgbroker/inmem"
)

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(datapagesgen.NewServer(&app.App{}, inmem.New(8)))
	t.Cleanup(srv.Close)
	return srv
}

func post(t *testing.T, srv *httptest.Server, path string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, srv.URL+path, nil,
	)
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

// TestRecoveredActionErrors covers a failed action of a Datastar request.
// The visitor is shown what RecoverError rendered and the request itself succeeds.
// Nothing navigates to a status page here, and nothing would read a status.
func TestRecoveredActionErrors(t *testing.T) {
	tests := map[string]struct{ path, want string }{
		"sentinel the hook knows":  {"/bad/", "bad request"},
		"another sentinel":         {"/missing/", "not found"},
		"error without a sentinel": {"/plain/", "unknown"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			srv := newServer(t)
			status, body := post(t, srv, tt.path)
			if status != http.StatusOK {
				t.Errorf("status = %d, want 200\n%s", status, body)
			}
			want := `<div id="toast">` + tt.want + `</div>`
			if !strings.Contains(body, want) {
				t.Errorf("the response does not carry %q:\n%s", want, body)
			}
		})
	}
}

// TestUnrecoveredActionErrorStillAnswers covers the case RecoverError itself
// cannot handle. The request must still be answered rather than left hanging,
// and what it is answered with must say nothing about the error.
//
// The answer is the end of the event stream. Its status line went out with the
// first byte, so the server has no status left to send and does not pretend otherwise;
// the failure goes to the operator's log. See the recoverfallback case.
func TestUnrecoveredActionErrorStillAnswers(t *testing.T) {
	srv := newServer(t)

	status, body := post(t, srv, "/unrecoverable/")
	if status != http.StatusOK {
		// The stream was committed before the failure, so 200 is what the
		// client already received.
		t.Errorf("status = %d, want the committed 200", status)
	}
	if strings.Contains(body, "cannot render a toast for this") ||
		strings.Contains(body, "unrecoverable") ||
		strings.Contains(body, "Internal Server Error") {
		t.Errorf("the failure reached the client:\n%s", body)
	}
}

// TestFailedPageLoadRendersTheErrorPage covers an ordinary page load that
// fails. It is not a Datastar request. There is a browser to navigate,
// and it navigates to the app's own 500 page.
func TestFailedPageLoadRendersTheErrorPage(t *testing.T) {
	srv := newServer(t)

	resp, err := srv.Client().Get(srv.URL + "/boom/")
	if err != nil {
		t.Fatalf("GET /boom/: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	if !strings.Contains(string(b), "server error") {
		t.Errorf("the 500 page was not rendered:\n%s", b)
	}
	if strings.Contains(string(b), "the page could not be built") {
		t.Errorf("the error message reached the visitor:\n%s", b)
	}
}
