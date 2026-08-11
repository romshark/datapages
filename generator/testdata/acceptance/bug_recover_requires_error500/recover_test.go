// Asserts that RecoverError is called for an app that defines it.

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

// TestRecoverErrorIsCalled is the recorded failure.
func TestRecoverErrorIsCalled(t *testing.T) {
	srv := httptest.NewServer(datapagesgen.NewServer(&app.App{}, inmem.New(8)))
	t.Cleanup(srv.Close)

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, srv.URL+"/fail/", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST /fail/: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil && !strings.Contains(err.Error(), "unexpected EOF") {
		t.Fatalf("reading body: %v", err)
	}

	if !strings.Contains(string(b), `<div id="toast">something went wrong</div>`) {
		t.Errorf("RecoverError did not answer the failed request:\n%s", b)
	}
}
