// Tests the two servers this module builds.
//
// The module names one app package with sessions and no metrics, and another
// with metrics and no sessions. Each app package is generated for what its own
// calls name: both features are generated, one into each package,
// and neither package carries the other's.

package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/client"
	adminapp "github.com/romshark/datapages/internal/acceptance/multiapp/app/admin"
	admingen "github.com/romshark/datapages/internal/acceptance/multiapp/app/admin/datapagesgen"
	frontendapp "github.com/romshark/datapages/internal/acceptance/multiapp/app/frontend"
	frontendgen "github.com/romshark/datapages/internal/acceptance/multiapp/app/frontend/datapagesgen"
	serveadmin "github.com/romshark/datapages/internal/acceptance/multiapp/serve/admin"
	servefrontend "github.com/romshark/datapages/internal/acceptance/multiapp/serve/frontend"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/sessions"
	sessinmem "github.com/romshark/datapages/modules/sessions/inmem"
)

func broker() messaging.Broker {
	return inmem.New(messaging.DefaultBrokerChanBuffer)
}

// TestFrontendCarriesSessions tests the session handling generated for the
// app package that declares a session type. The other app package declares
// none and is generated in the same run.
func TestFrontendCarriesSessions(t *testing.T) {
	t.Parallel()
	c := client.New(t, mustNewFrontend(t, &frontendapp.App{}, broker())).WithJar(t)

	require.Equal(t, "anonymous", c.Get(t, "/").Element(t, "echo"))

	resp := c.Action(t, http.MethodPost, "/sign-in/",
		`{"user":"alice","nickname":"al"}`)
	require.Equal(t, http.StatusOK, resp.Status)

	require.Equal(t, "user=alice nickname=al", c.Get(t, "/").Element(t, "echo"))
}

// TestFrontendCarriesNoMetrics tests the frontend server against the option
// its type arguments do not ask for. The generated code refuses it,
// which is how a caller finds out that this app package has no instrumentation.
func TestFrontendCarriesNoMetrics(t *testing.T) {
	t.Parallel()
	s, err := datapages.NewServer[
		frontendapp.App,
		frontendapp.SessionData,
		datapages.DisablePrometheus,
		frontendgen.Server,
	](&frontendapp.App{}, broker(),
		datapages.WithSessionManager(newSessionManager()),
		datapages.WithPrometheus(prometheusConfig()))
	require.Nil(t, s)
	require.ErrorContains(t, err, "unexpected option WithPrometheus")
}

// TestAdminCarriesMetrics tests the instrumentation generated for the app
// package whose calls name datapages.EnablePrometheus. The counters are read
// from the registry the server was given, the same registry a scrape reads.
func TestAdminCarriesMetrics(t *testing.T) {
	t.Parallel()
	c := client.New(t, mustNewAdmin(t, &adminapp.App{}, broker()))

	require.Equal(t, "admin", c.Get(t, "/").Element(t, "echo"))
	require.Equal(t, http.StatusOK,
		c.Action(t, http.MethodPost, "/report/", `{"n":1}`).Status)

	total := requestsTotal(t)
	require.GreaterOrEqual(t, total, float64(2),
		"the request counter stands at %v after two requests", total)
}

// requestsTotal sums every sample of the generated request counter in the
// registry the servers were given, the same registry a scrape reads.
func requestsTotal(t *testing.T) float64 {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err, "gathering metrics")

	var total float64
	found := false
	for _, f := range families {
		if f.GetName() != "datapages_http_requests_total" {
			continue
		}
		found = true
		for _, m := range f.GetMetric() {
			total += m.GetCounter().GetValue()
		}
	}
	require.True(t, found, "the request counter was not registered")
	return total
}

// TestAdminCarriesNoSessions tests the admin server against the option its
// type arguments do not ask for. The generated Init rejects a session manager
// by name, which only an app package declaring no session type is given.
func TestAdminCarriesNoSessions(t *testing.T) {
	t.Parallel()
	s, err := datapages.NewServer[
		adminapp.App,
		datapages.DisableSessions,
		datapages.EnablePrometheus,
		admingen.Server,
	](&adminapp.App{}, broker(),
		datapages.WithPrometheus(prometheusConfig()),
		datapages.WithSessionManager(
			sessinmem.New[datapages.DisableSessions](
				sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
			),
		))
	require.Nil(t, s)
	require.ErrorContains(t, err, "unexpected option WithSessionManager")
	require.ErrorContains(t, err, "declares no session type")
}

// TestAdminRequiresItsPrometheusConfig tests the admin server without the
// option its metrics mode requires.
//
// The frontend counterpart cannot be written: the scan refuses a call naming a
// session type without datapages.WithSessionManager, and every call of this
// module is scanned, test files included.
func TestAdminRequiresItsPrometheusConfig(t *testing.T) {
	t.Parallel()
	s, err := datapages.NewServer[
		adminapp.App,
		datapages.DisableSessions,
		datapages.EnablePrometheus,
		admingen.Server,
	](&adminapp.App{}, broker())
	require.Nil(t, s)
	require.ErrorContains(t, err, "missing option WithPrometheus")
}

// TestBothServersRunTogether tests the two applications running side by side.
// Each serves its own routes and neither serves the other's.
func TestBothServersRunTogether(t *testing.T) {
	t.Parallel()
	front, err := servefrontend.New(
		&frontendapp.App{}, broker(), newSessionManager(),
	)
	require.NoError(t, err)
	adm, err := serveadmin.New(&adminapp.App{}, broker(), prometheusConfig())
	require.NoError(t, err)

	f := client.New(t, front)
	a := client.New(t, adm)

	require.Equal(t, "anonymous", f.Get(t, "/").Element(t, "echo"))
	require.Equal(t, "admin", a.Get(t, "/").Element(t, "echo"))

	// The action each app declares is served by that app alone. The status a
	// router answers a URL no page claims with is not fixed,
	// hence the assertion is only that the request did not reach a handler.
	require.Equal(t, http.StatusOK,
		f.Action(t, http.MethodPost, "/notice/", `{"text":"hi"}`).Status)
	require.GreaterOrEqual(t,
		a.Action(t, http.MethodPost, "/notice/", `{"text":"hi"}`).Status,
		http.StatusBadRequest, "the admin server serves the frontend action")
	require.Equal(t, http.StatusOK,
		a.Action(t, http.MethodPost, "/report/", `{"n":1}`).Status)
	require.GreaterOrEqual(t,
		f.Action(t, http.MethodPost, "/report/", `{"n":1}`).Status,
		http.StatusBadRequest, "the frontend server serves the admin action")
}
