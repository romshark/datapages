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
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/csrfcoverage/app"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/sessions"
	sessinmem "github.com/romshark/datapages/modules/sessions/inmem"
)

// countingSessions counts the store reads a request costs.
type countingSessions struct {
	sessions.Manager[struct{}]
	reads atomic.Int64
}

func (c *countingSessions) ReadSessionFromCookie(cookieValue string) (
	sessions.Record[struct{}], string, bool, error,
) {
	c.reads.Add(1)
	return c.Manager.ReadSessionFromCookie(cookieValue)
}

// TestCSRFOnlyActionReadsNoSession covers the check an action runs when it
// takes no session. The CSRF token is derived from the cookie, hence the store
// stays out of the request.
func TestCSRFOnlyActionReadsNoSession(t *testing.T) {
	t.Parallel()
	store := &countingSessions{Manager: sessinmem.New[struct{}](
		sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
	)}

	srv := httptest.NewServer(mustNewServer(
		t, &app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer), store,
	))
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	require.NoError(t, err, "building cookie jar")
	client := &http.Client{Jar: jar}

	post := func(path, body, token string) int {
		req, err := http.NewRequestWithContext(context.Background(),
			http.MethodPost, srv.URL+path, strings.NewReader(body))
		require.NoError(t, err, "building POST %s", path)
		req.Header.Set("Datastar-Request", "true")
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("X-CSRF-Token", token)
		}
		resp, err := client.Do(req)
		require.NoError(t, err, "POST %s", path)
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode
	}

	require.Equal(t, http.StatusOK,
		post("/sign-in/", `{"user":"alice"}`, ""), "signing in")

	before := store.reads.Load()
	require.Equal(t, http.StatusForbidden,
		post("/delete/", `{"confirm":true}`, ""),
		"an action without a CSRF token was served")
	require.Equal(t, before, store.reads.Load(),
		"the store was read for an action that takes no session")
}

// TestCSRFCoversEveryAction covers a state-changing action that declares
// neither session nor sessionToken.
//
// A visitor with a session sends it without a CSRF token,
// which is the request a cross-site page can make their browser send.
// The server refuses it and the action does not take effect.
func TestCSRFCoversEveryAction(t *testing.T) {
	t.Parallel()
	sessions := sessinmem.New[struct{}](
		sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
	)

	srv := httptest.NewServer(mustNewServer(
		t,
		&app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer), sessions,
	))
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	require.NoError(t, err, "building cookie jar")
	client := &http.Client{Jar: jar}

	post := func(path, body, token string) int {
		req, err := http.NewRequestWithContext(context.Background(),
			http.MethodPost, srv.URL+path, strings.NewReader(body))
		require.NoError(t, err, "building POST %s", path)
		req.Header.Set("Datastar-Request", "true")
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("X-CSRF-Token", token)
		}
		resp, err := client.Do(req)
		require.NoError(t, err, "POST %s", path)
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode
	}

	require.Equal(t, http.StatusOK,
		post("/sign-in/", `{"user":"alice"}`, ""), "signing in")

	require.Equal(t, http.StatusForbidden,
		post("/delete/", `{"confirm":true}`, ""),
		"a state-changing action of a visitor with a session was served "+
			"without a CSRF token")

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, srv.URL+"/", nil,
	)
	require.NoError(t, err, "building GET /")
	resp, err := client.Do(req)
	require.NoError(t, err, "GET /")
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "reading /")
	require.NotContains(t, string(b), "deleted=1", "the refused action took effect")
}
