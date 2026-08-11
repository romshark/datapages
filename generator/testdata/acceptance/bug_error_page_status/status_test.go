// Asserts that a rendered error page carries the status it stands for.

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

// TestErrorPageStatus is the recorded failure.
func TestErrorPageStatus(t *testing.T) {
	srv := httptest.NewServer(datapagesgen.NewServer(&app.App{}, inmem.New(8)))
	t.Cleanup(srv.Close)

	resp, err := srv.Client().Get(srv.URL + "/no-such-page/")
	if err != nil {
		t.Fatalf("GET /no-such-page/: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}

	// The page is the app's own. The status is therefore wrong rather than
	// merely absent.
	if !strings.Contains(string(b), `<p id="msg">no such page</p>`) {
		t.Fatalf("the custom 404 page was not rendered:\n%s", b)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}
