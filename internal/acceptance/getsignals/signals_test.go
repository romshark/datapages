// Drives a page GET that reads signals and one that issues a session.

package acceptance_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/internal/acceptance/getsignals/app"
	"github.com/romshark/datapages/modules/csrf"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/sessions"
	sessinmem "github.com/romshark/datapages/modules/sessions/inmem"
)

// newClient starts the server and returns a client that keeps its cookies.
func newClient(t *testing.T) *client.Client {
	t.Helper()
	inMemSessions := sessinmem.New[struct{}](
		sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
	)
	h := mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer),
		inMemSessions)
	return client.New(t, h).WithJar(t)
}

// csrfToken is the token the visitor sends back on an action.
// Datapages derives it from the session token, which is what the response set
// the session cookie to.
func csrfToken(t *testing.T, resp client.Response) string {
	t.Helper()
	for _, raw := range resp.Header.Values("Set-Cookie") {
		ck, err := http.ParseSetCookie(raw)
		require.NoError(t, err, "parsing Set-Cookie")
		if ck.Name != "sessiontoken" {
			continue
		}
		var b strings.Builder
		_, err = csrf.Tokens{}.WriteToken(&b, ck.Value)
		require.NoError(t, err, "writing the CSRF token")
		return b.String()
	}
	t.Fatal("the response set no session cookie")
	return ""
}

// signalsQuery is how the Datastar client sends signals on a GET.
func signalsQuery(json string) string {
	return "/?datastar=" + url.QueryEscape(json)
}

// TestGETReadsSignals covers a page load carrying signals.
func TestGETReadsSignals(t *testing.T) {
	c := newClient(t)

	resp := c.Get(t, signalsQuery(`{"term":"shoes","page":3}`))

	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "term=shoes page=3 user=", resp.Element(t, "echo"))
}

// TestGETWithoutSignals covers the ordinary page load:
// a visitor who typed the URL sends no signals and the handler is given the zero value.
func TestGETWithoutSignals(t *testing.T) {
	c := newClient(t)

	resp := c.Get(t, "/")

	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "term= page=0 user=", resp.Element(t, "echo"))
}

// TestGETWithMalformedSignals covers a datastar parameter that is not signals.
// The request is refused rather than served a page built from nothing.
func TestGETWithMalformedSignals(t *testing.T) {
	c := newClient(t)

	resp := c.Get(t, signalsQuery(`{"term":`))

	require.Equal(t, http.StatusBadRequest, resp.Status)
}

// TestGETIssuesASession covers a page load that returns newSession:
// the cookie is set on that same response, and the next page load reads the session.
func TestGETIssuesASession(t *testing.T) {
	c := newClient(t)

	enter := c.Get(t, "/enter/")
	require.Equal(t, http.StatusOK, enter.Status)
	require.Equal(t, "entered", enter.Element(t, "echo"))

	require.NotEmpty(t, enter.Header.Values("Set-Cookie"),
		"a page load that issues a session set no cookie")

	resp := c.Get(t, "/")
	require.Equal(t, "term= page=0 user=alice", resp.Element(t, "echo"),
		"the session issued by a page load does not reach the next one")
}

// TestActionClosesTheSession covers the other end: an action that reads only
// the session token ends the session, and the page stops seeing it.
func TestActionClosesTheSession(t *testing.T) {
	c := newClient(t)

	enter := c.Get(t, "/enter/")
	require.Equal(t, "term= page=0 user=alice", c.Get(t, "/").Element(t, "echo"))

	token := csrfToken(t, enter)
	req := c.Request(t, http.MethodPost, "/leave/", "")
	req.Header.Set("X-CSRF-Token", token)
	require.Equal(t, http.StatusOK, c.Do(t, req).Status)

	require.Equal(t, "term= page=0 user=", c.Get(t, "/").Element(t, "echo"),
		"the session outlived the action that closed it")
}
