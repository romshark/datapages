// Asserts the fallback the generated code takes when RecoverError cannot
// answer.

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

// TestRecoverFallbackStatus is the recorded failure.
func TestRecoverFallbackStatus(t *testing.T) {
	srv := httptest.NewServer(datapagesgen.NewServer(&app.App{}, inmem.New(8)))
	t.Cleanup(srv.Close)

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, srv.URL+"/bad/", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST /bad/: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil && !strings.Contains(err.Error(), "unexpected EOF") {
		t.Fatalf("reading body: %v", err)
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	if strings.Contains(string(b), "Bad Request") &&
		resp.StatusCode == http.StatusOK {
		t.Errorf("the status text was written into a 200 response body:\n%s", b)
	}
}
