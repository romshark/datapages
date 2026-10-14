// Tests the 2 servers this module builds.
//
// The module names one app package with sessions and no metrics, and another
// with metrics and no sessions. Each app package is generated for what its own
// calls name: both features are generated, 1 into each package,
// and neither package carries the other's.

package acceptance_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/datapagestest"
	adminapp "github.com/romshark/datapages/internal/acceptance/multiapp/app/admin"
	admingen "github.com/romshark/datapages/internal/acceptance/multiapp/app/admin/datapagesgen"
	adminaction "github.com/romshark/datapages/internal/acceptance/multiapp/app/admin/datapagesgen/action"
	frontendapp "github.com/romshark/datapages/internal/acceptance/multiapp/app/frontend"
	frontendgen "github.com/romshark/datapages/internal/acceptance/multiapp/app/frontend/datapagesgen"
	frontendaction "github.com/romshark/datapages/internal/acceptance/multiapp/app/frontend/datapagesgen/action"
	serveadmin "github.com/romshark/datapages/internal/acceptance/multiapp/serve/admin"
	servefrontend "github.com/romshark/datapages/internal/acceptance/multiapp/serve/frontend"
	"github.com/romshark/datapages/internal/testlog"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/sessions"
	sessinmem "github.com/romshark/datapages/modules/sessions/inmem"
)

func broker() messaging.Broker {
	return inmem.New(messaging.DefaultBrokerChanBuffer)
}

// echo returns the text of the element the index page of either app renders
// its state into.
func echo(t *testing.T, page *datapagestest.Response) string {
	t.Helper()
	return datapagestest.Text(page.ElementByID(t, "echo"))
}

func text(s string) map[string]string { return map[string]string{"text": s} }

func report(n int) map[string]int { return map[string]int{"n": n} }

func signIn(user, nickname string) map[string]string {
	return map[string]string{"user": user, "nickname": nickname}
}

// TestFrontendCarriesSessions tests the session handling generated for the
// app package that declares a session type. The other app package declares
// none and is generated in the same run.
func TestFrontendCarriesSessions(t *testing.T) {
	t.Parallel()
	v := datapagestest.New(t, mustNewFrontend(t, &frontendapp.App{}, broker())).Visitor(t)

	tab := v.Load(t, "/")
	require.Equal(t, "anonymous", echo(t, tab.Page()))

	resp := tab.Do(t, http.MethodPost, "/sign-in/", signIn("alice", "al"))
	require.Equal(t, http.StatusOK, resp.Status)

	require.Equal(t, "user=alice nickname=al", echo(t, v.Get(t, "/")))
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
	v := datapagestest.New(t, mustNewAdmin(t, &adminapp.App{}, broker())).Visitor(t)

	tab := v.Load(t, "/")
	require.Equal(t, "admin", echo(t, tab.Page()))
	require.Equal(t, http.StatusOK,
		tab.Do(t, http.MethodPost, "/report/", report(1)).Status)

	total := requestsTotal(t)
	require.GreaterOrEqual(t, total, float64(2),
		"the request counter stands at %v after 2 requests", total)
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
// The error uses the name the app package declares.
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
	require.ErrorContains(t, err, "unexpected option WithSessionManager: "+
		"package admin declares no session type")
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

// TestBothServersRunTogether tests the 2 applications running side by side.
// Each serves its own routes and neither serves the other's.
func TestBothServersRunTogether(t *testing.T) {
	t.Parallel()
	front, err := servefrontend.New(
		&frontendapp.App{}, broker(), newSessionManager(),
	)
	require.NoError(t, err)
	adm, err := serveadmin.New(&adminapp.App{}, broker(), prometheusConfig())
	require.NoError(t, err)

	f := datapagestest.New(t, front).Visitor(t).Load(t, "/")
	a := datapagestest.New(t, adm).Visitor(t).Load(t, "/")

	require.Equal(t, "anonymous", echo(t, f.Page()))
	require.Equal(t, "admin", echo(t, a.Page()))

	// The action each app declares is served by that app alone. The status a
	// router answers a URL no page claims with is not fixed,
	// hence the assertion is only that the request did not reach a handler.
	require.Equal(t, http.StatusOK,
		f.Do(t, http.MethodPost, "/notice/", text("hi")).Status)
	require.GreaterOrEqual(t,
		a.Do(t, http.MethodPost, "/notice/", text("hi")).Status,
		http.StatusBadRequest, "the admin server serves the frontend action")
	require.Equal(t, http.StatusOK,
		a.Do(t, http.MethodPost, "/report/", report(1)).Status)
	require.GreaterOrEqual(t,
		f.Do(t, http.MethodPost, "/report/", report(1)).Status,
		http.StatusBadRequest, "the frontend server serves the admin action")
}

// TestSharedEventReachesBothApps tests the event neither app package declares.
//
// Both name the type of the events package,
// which is what makes the 2 applications take part in 1 event.
// Given 1 broker, what either dispatches reaches the streams of both.
func TestSharedEventReachesBothApps(t *testing.T) {
	t.Parallel()
	shared := broker()

	f := datapagestest.New(t,
		mustNewFrontend(t, &frontendapp.App{}, shared)).Visitor(t).Load(t, "/")
	a := datapagestest.New(t,
		mustNewAdmin(t, &adminapp.App{}, shared)).Visitor(t).Load(t, "/")

	fs := f.Connect(t, nil)
	as := a.Connect(t, nil)

	// Dispatched by admin, handled by both.
	a.Do(t, http.MethodPost, "/announce/", text("from admin")).
		WantStatus(t, http.StatusOK)
	as.WaitFor(t, datapagestest.TextContains("announcement", "from admin"),
		"the dispatching application did not receive its own event")
	fs.WaitFor(t, datapagestest.TextContains("announcement", "from admin"),
		"the other application did not receive the shared event")

	// And the other way round.
	f.Do(t, http.MethodPost, "/announce/", text("from frontend")).
		WantStatus(t, http.StatusOK)
	fs.WaitFor(t, datapagestest.TextContains("announcement", "from frontend"),
		"the dispatching application did not receive its own event")
	as.WaitFor(t, datapagestest.TextContains("announcement", "from frontend"),
		"the other application did not receive the shared event")

	// An event only 1 application declares stays with it. The announcement
	// the frontend dispatches next is the anchor: a notice that reached the admin
	// stream would arrive before it, since the broker delivers in publication order.
	f.Do(t, http.MethodPost, "/notice/", text("private")).WantStatus(t, http.StatusOK)
	fs.WaitFor(t, datapagestest.TextContains("out", "private"))
	f.Do(t, http.MethodPost, "/announce/", text("after the notice")).
		WantStatus(t, http.StatusOK)
	private := datapagestest.Func(`an event carrying "private"`,
		func(ev datapagestest.Event) bool {
			for _, v := range ev.Data {
				if strings.Contains(v, "private") {
					return true
				}
			}
			return false
		})
	as.Absent(t, private, datapagestest.TextContains("announcement", "after the notice"),
		"the admin stream received an event only frontend declares")
}

// TestActionLoggerIsPerApplication tests the logger a dropped action option is
// reported to. Each application has its own, which the Init of the other
// server must not replace.
//
// It must not use t.Parallel(): both Init calls write the logger the action
// package of their own application holds.
func TestActionLoggerIsPerApplication(t *testing.T) {
	var frontendLog, adminLog testlog.Buffer
	mustNewFrontend(t, &frontendapp.App{}, broker(),
		datapages.WithLogger(frontendLog.Logger()))
	mustNewAdmin(t, &adminapp.App{}, broker(),
		datapages.WithLogger(adminLog.Logger()))

	frontendaction.WithRetryInterval(-1)
	adminaction.WithRetryMaxCount(-1)

	const dropped = `level=WARN msg="dropping an action option with an invalid value" `
	frontendLog.RequireLines(t, dropped+"option=WithRetryInterval value=-1")
	adminLog.RequireLines(t, dropped+"option=WithRetryMaxCount value=-1")
}
