// Asserts that a failed page load renders the page the application supplies
// for it.

package acceptance_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dpacceptance/app"
	"dpacceptance/datapagesgen"

	"github.com/romshark/datapages/modules/msgbroker/inmem"
)

// TestError500PageIsRendered is the recorded failure.
func TestError500PageIsRendered(t *testing.T) {
	srv := httptest.NewServer(datapagesgen.NewServer(&app.App{}, inmem.New(8)))
	t.Cleanup(srv.Close)

	// The page exists and renders on its own route, which rules out the page
	// itself as the reason the failed load does not show it.
	resp, err := srv.Client().Get(srv.URL + "/server-error/")
	if err != nil {
		t.Fatalf("GET /server-error/: %v", err)
	}
	b, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	if !strings.Contains(string(b), `<p id="msg">`) {
		t.Fatalf("the 500 page does not render on its own route:\n%s", b)
	}

	resp, err = srv.Client().Get(srv.URL + "/boom/")
	if err != nil {
		t.Fatalf("GET /boom/: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d",
			resp.StatusCode, http.StatusInternalServerError)
	}
	if !strings.Contains(string(b), "something went wrong on our side") {
		t.Errorf("a failed page load did not render PageError500:\n%s", b)
	}
}
