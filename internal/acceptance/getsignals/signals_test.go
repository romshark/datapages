// Drives a page GET that reads signals and one that issues a session.

package acceptance_test

import (
	"crypto/sha256"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/internal/acceptance/getsignals/app"
	"github.com/romshark/datapages/internal/acceptance/getsignals/datapagesgen"
	csrfhmac "github.com/romshark/datapages/modules/csrf/hmac"
	"github.com/romshark/datapages/modules/msgbroker/inmem"
	sessinmem "github.com/romshark/datapages/modules/sessmanager/inmem"
	"github.com/romshark/datapages/modules/sesstokgen"
)

// newClient starts the server and returns a client that keeps its cookies,
// together with the CSRF token manager, which a test needs to send an action
// as the visitor the session names.
func newClient(t *testing.T) (*client.Client, *csrfhmac.TokenManager) {
	t.Helper()
	key := sha256.Sum256([]byte("acceptance-csrf"))
	tm, err := csrfhmac.New(key[:])
	require.NoError(t, err, "building CSRF token manager")
	sessions := sessinmem.New[app.Session](
		sesstokgen.Generator{Length: sesstokgen.DefaultLength},
	)
	h := datapagesgen.NewServer(&app.App{}, inmem.New(8), sessions,
		datapagesgen.WithCSRFProtection(datapagesgen.CSRFConfig{TokenManager: tm}))
	return client.New(t, h).WithJar(t), tm
}

// signalsQuery is how the Datastar client sends signals on a GET.
func signalsQuery(json string) string {
	return "/?datastar=" + url.QueryEscape(json)
}

// TestGETReadsSignals covers a page load carrying signals.
func TestGETReadsSignals(t *testing.T) {
	c, _ := newClient(t)

	resp := c.Get(t, signalsQuery(`{"term":"shoes","page":3}`))

	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "term=shoes page=3 user=", resp.Element(t, "echo"))
}

// TestGETWithoutSignals covers the ordinary page load:
// a visitor who typed the URL sends no signals and the handler is given the zero value.
func TestGETWithoutSignals(t *testing.T) {
	c, _ := newClient(t)

	resp := c.Get(t, "/")

	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "term= page=0 user=", resp.Element(t, "echo"))
}

// TestGETWithMalformedSignals covers a datastar parameter that is not signals.
// The request is refused rather than served a page built from nothing.
func TestGETWithMalformedSignals(t *testing.T) {
	c, _ := newClient(t)

	resp := c.Get(t, signalsQuery(`{"term":`))

	require.Equal(t, http.StatusBadRequest, resp.Status)
}

// TestGETIssuesASession covers a page load that returns newSession:
// the cookie is set on that same response, and the next page load reads the session.
func TestGETIssuesASession(t *testing.T) {
	c, _ := newClient(t)

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
	c, csrf := newClient(t)

	c.Get(t, "/enter/")
	require.Equal(t, "term= page=0 user=alice", c.Get(t, "/").Element(t, "echo"))

	token := csrf.GenerateToken("alice", app.IssuedAt.Unix())
	req := c.Request(t, http.MethodPost, "/leave/", "")
	req.Header.Set("X-CSRF-Token", token)
	require.Equal(t, http.StatusOK, c.Do(t, req).Status)

	require.Equal(t, "term= page=0 user=", c.Get(t, "/").Element(t, "echo"),
		"the session outlived the action that closed it")
}
