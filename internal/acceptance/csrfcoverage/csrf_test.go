// Asserts that CSRF protection covers every state-changing action a
// visitor with a session can reach.

package acceptance_test

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/romshark/datapages/internal/acceptance/csrfcoverage/app"
	"github.com/romshark/datapages/internal/acceptance/csrfcoverage/datapagesgen"
	csrfhmac "github.com/romshark/datapages/modules/csrf/hmac"
	"github.com/romshark/datapages/modules/msgbroker/inmem"
	sessinmem "github.com/romshark/datapages/modules/sessmanager/inmem"
	"github.com/romshark/datapages/modules/sesstokgen"
)

// TestCSRFCoversEveryAction covers a state-changing action that declares
// neither session nor sessionToken.
//
// A visitor with a session sends it without a CSRF token,
// which is the request a cross-site page can make their browser send.
// The server refuses it and the action does not take effect.
func TestCSRFCoversEveryAction(t *testing.T) {
	tm, err := csrfhmac.New([]byte("acceptance-csrf-secret-value-0123"))
	if err != nil {
		t.Fatalf("building CSRF token manager: %v", err)
	}
	sessions := sessinmem.New[struct{}](
		sesstokgen.Generator{Length: sesstokgen.DefaultLength},
	)

	srv := httptest.NewServer(datapagesgen.NewServer(
		&app.App{}, inmem.New(8), sessions,
		datapagesgen.WithCSRFProtection(
			datapagesgen.CSRFConfig{TokenManager: tm},
		),
	))
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("building cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar}

	post := func(path, body, token string) int {
		req, err := http.NewRequestWithContext(context.Background(),
			http.MethodPost, srv.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatalf("building POST %s: %v", path, err)
		}
		req.Header.Set("Datastar-Request", "true")
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("X-CSRF-Token", token)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode
	}

	if st := post("/sign-in/", `{"user":"alice"}`, ""); st != http.StatusOK {
		t.Fatalf("signing in: status = %d", st)
	}

	if st := post("/delete/", `{"confirm":true}`, ""); st != http.StatusForbidden {
		t.Errorf("a state-changing action of a visitor with a session was "+
			"served without a CSRF token: status = %d, want %d",
			st, http.StatusForbidden)
	}

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, srv.URL+"/", nil,
	)
	if err != nil {
		t.Fatalf("building GET /: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading /: %v", err)
	}
	if strings.Contains(string(b), "deleted=1") {
		t.Error("the refused action took effect")
	}
}
