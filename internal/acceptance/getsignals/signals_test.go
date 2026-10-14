// Tests a page GET that reads signals and one that issues a session.

package acceptance_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/internal/acceptance/getsignals/app"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/sessions"
	sessinmem "github.com/romshark/datapages/modules/sessions/inmem"
)

func serve(t *testing.T) *datapagestest.Server {
	t.Helper()
	inMemSessions := sessinmem.New[struct{}](
		sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
	)
	return datapagestest.New(t, mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer),
		inMemSessions))
}

// echo returns the text of the element into which a page echoes what its handler read.
func echo(t *testing.T, resp *datapagestest.Response) string {
	t.Helper()
	return datapagestest.Text(resp.ElementByID(t, "echo"))
}

// signalsQuery is how the Datastar client sends signals on a GET.
func signalsQuery(json string) string {
	return "/?datastar=" + url.QueryEscape(json)
}

// TestGETReadsSignals tests a page load carrying signals.
func TestGETReadsSignals(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	resp := v.Get(t, signalsQuery(`{"term":"shoes","page":3}`))

	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "term=shoes page=3 user=", echo(t, resp))
}

// TestGETReadsNestedSignals tests a signals struct that nests.
//
// A nested struct is a nested signal: the client sends 1 object and the
// handler reads foo.bar.bazz and foo.fuzz out of it. The page reflects foo.fuzz into
// a query parameter, which is how a nested signal is referenced by its path.
func TestGETReadsNestedSignals(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	resp := v.Get(t, "/nested/?fuzz=q&datastar="+url.QueryEscape(
		`{"foo":{"bar":{"bazz":"x"},"fuzz":"y"}}`,
	))

	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "bazz=x fuzz=y query=q", echo(t, resp))
	require.Contains(t, resp.Body, "params.set('fuzz', $foo.fuzz)",
		"the page does not reflect the nested signal")
}

// TestGETWithoutSignals tests the ordinary page load:
// a visitor who typed the URL sends no signals and the handler is given the zero value.
func TestGETWithoutSignals(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	resp := v.Get(t, "/")

	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "term= page=0 user=", echo(t, resp))
}

// TestGETWithMalformedSignals tests a datastar parameter that is not signals.
// The request is refused rather than served a page built from nothing.
func TestGETWithMalformedSignals(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	resp := v.Get(t, signalsQuery(`{"term":`))

	require.Equal(t, http.StatusBadRequest, resp.Status)
}

// TestGETIssuesASession tests a page load that returns newSession:
// the cookie is set on that same response, and the next page load reads the session.
func TestGETIssuesASession(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	enter := v.Get(t, "/enter/")
	require.Equal(t, http.StatusOK, enter.Status)
	require.Equal(t, "entered", echo(t, enter))

	require.NotEmpty(t, enter.Header.Values("Set-Cookie"),
		"a page load that issues a session set no cookie")

	resp := v.Get(t, "/")
	require.Equal(t, "term= page=0 user=alice", echo(t, resp),
		"the session issued by a page load does not reach the next one")
}

// TestActionClosesTheSession tests the other end: an action that reads only
// the session token ends the session, and the page stops seeing it.
func TestActionClosesTheSession(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	v.Get(t, "/enter/")
	index := v.Load(t, "/")
	require.Equal(t, "term= page=0 user=alice", echo(t, index.Page()))

	// The page loaded after /enter/ carries the CSRF token of the session it set.
	require.Equal(t, http.StatusOK, index.Do(t, http.MethodPost, "/leave/", nil).Status)

	require.Equal(t, "term= page=0 user=", echo(t, v.Get(t, "/")),
		"the session outlived the action that closed it")
}
