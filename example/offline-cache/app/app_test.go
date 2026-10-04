package app_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/offline-cache/app"
	"github.com/romshark/datapages/example/offline-cache/app/datapagesgen"
	"github.com/romshark/datapages/example/offline-cache/app/domain"
	"github.com/romshark/datapages/modules/csrf"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/sessions"
	sessinmem "github.com/romshark/datapages/modules/sessions/inmem"
)

const password = "demopass"

// newServer serves repo the way cmd/server does, without the service worker.
func newServer(t *testing.T, repo *domain.Repository) *httptest.Server {
	t.Helper()
	s, err := datapages.NewServer[
		app.App,
		struct{},
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](
		app.NewApp(repo),
		inmem.New(messaging.DefaultBrokerChanBuffer),
		datapages.WithSessionManager(sessinmem.New[struct{}](
			sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
		)),
	)
	require.NoError(t, err, "building server")
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	return srv
}

func newUser(t *testing.T, repo *domain.Repository, name string) {
	t.Helper()
	_, err := repo.NewUser(t.Context(), name, name+"@demo.test", "", password)
	require.NoError(t, err, "adding user %q", name)
}

// visitor is one browser. A visitor without a session is a guest.
type visitor struct {
	srv     *httptest.Server
	session string
}

// signIn returns the visitor holding the session of user name.
func signIn(t *testing.T, srv *httptest.Server, name string) visitor {
	t.Helper()
	resp, body := visitor{srv: srv}.post(t, "/login/submit/", map[string]string{
		"emailorusername": name,
		"password":        password,
	})
	require.Equal(t, http.StatusOK, resp.StatusCode, "signing in: %s", body)
	for _, c := range resp.Cookies() {
		if c.Name == datapages.DefaultSessionCookieName && c.Value != "" {
			return visitor{srv: srv, session: c.Value}
		}
	}
	require.FailNow(t, "signing in set no session cookie")
	return visitor{}
}

// post sends a Datastar action with signals as its body. A visitor with a
// session sends its cookie and the CSRF token the page script would add.
func (v visitor) post(
	t *testing.T, path string, signals any,
) (*http.Response, string) {
	t.Helper()
	b, err := json.Marshal(signals)
	require.NoError(t, err, "encoding signals")
	req, err := http.NewRequestWithContext(t.Context(),
		http.MethodPost, v.srv.URL+path, bytes.NewReader(b))
	require.NoError(t, err, "building POST %s", path)
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Content-Type", "application/json")
	// The transport asks for gzip by default. The SSE writer never ends the
	// gzip stream, and reading an SSE response fails with an unexpected EOF.
	req.Header.Set("Accept-Encoding", "identity")
	if v.session != "" {
		// A cookie jar would not send the Secure session cookie
		// to the plain HTTP test server.
		req.AddCookie(&http.Cookie{
			Name:  datapages.DefaultSessionCookieName,
			Value: v.session,
		})
		var token strings.Builder
		_, err := csrf.Tokens{}.WriteToken(&token, v.session)
		require.NoError(t, err, "writing CSRF token")
		req.Header.Set("X-CSRF-Token", token.String())
	}
	resp, err := v.srv.Client().Do(req)
	require.NoError(t, err, "POST %s", path)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "reading response of POST %s", path)
	return resp, string(body)
}
