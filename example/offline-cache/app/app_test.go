package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/example/offline-cache/app"
	"github.com/romshark/datapages/example/offline-cache/app/datapagesgen"
	"github.com/romshark/datapages/example/offline-cache/app/datapagesgen/action"
	"github.com/romshark/datapages/example/offline-cache/app/datapagesgen/href"
	"github.com/romshark/datapages/example/offline-cache/app/domain"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/sessions"
	sessinmem "github.com/romshark/datapages/modules/sessions/inmem"
)

const password = "demopass"

// newServer serves repo the way cmd/server does, without the service worker.
func newServer(t *testing.T, repo *domain.Repository) *datapagestest.Server {
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
	return datapagestest.New(t, s)
}

func newUser(t *testing.T, repo *domain.Repository, name string) {
	t.Helper()
	_, err := repo.NewUser(t.Context(), name, name+"@demo.test", "", password)
	require.NoError(t, err, "adding user %q", name)
}

// signIn returns a visitor that signed in as user name on the login page.
func signIn(t *testing.T, srv *datapagestest.Server, name string) *datapagestest.Visitor {
	t.Helper()
	v := srv.Visitor(t)
	resp := v.Load(t, href.PageLogin(href.QueryPageLogin{})).Do(t, action.PageLogin.Submit.POST(), map[string]string{
		"emailorusername": name,
		"password":        password,
	})
	require.Equal(t, http.StatusOK, resp.Status, "signing in: %s", resp.Body)
	c := v.Cookie(datapages.DefaultSessionCookieName)
	require.True(t, c != nil && c.Value != "", "signing in set no session cookie")
	return v
}
