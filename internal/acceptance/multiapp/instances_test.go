// Tests 2 instances of each application running in 1 process.
//
// A generated package is imported once and built any number of times. State
// kept in a package variable would be shared by every server built from it:
// the first would pass and the second would read what the first left behind.
// 4 servers here, 2 per app, each with its own broker, its own session
// manager and its own app value.

package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/datapagestest"
	adminapp "github.com/romshark/datapages/internal/acceptance/multiapp/app/admin"
	frontendapp "github.com/romshark/datapages/internal/acceptance/multiapp/app/frontend"
)

// instances builds a server per entry and returns a visitor of each,
// keyed the way the assertions name them.
func instances(t *testing.T) map[string]*datapagestest.Visitor {
	t.Helper()
	return map[string]*datapagestest.Visitor{
		"frontend A": datapagestest.New(t,
			mustNewFrontend(t, &frontendapp.App{}, broker())).Visitor(t),
		"frontend B": datapagestest.New(t,
			mustNewFrontend(t, &frontendapp.App{}, broker())).Visitor(t),
		"admin A": datapagestest.New(t,
			mustNewAdmin(t, &adminapp.App{}, broker())).Visitor(t),
		"admin B": datapagestest.New(t,
			mustNewAdmin(t, &adminapp.App{}, broker())).Visitor(t),
	}
}

// Test4InstancesServeTheirOwnApp tests all 4 answering at once. 2
// servers of 1 app must not collide over a route, a subject or a collector.
func Test4InstancesServeTheirOwnApp(t *testing.T) {
	t.Parallel()
	all := instances(t)

	for name, want := range map[string]string{
		"frontend A": "anonymous",
		"frontend B": "anonymous",
		"admin A":    "admin",
		"admin B":    "admin",
	} {
		require.Equal(t, want, echo(t, all[name].Get(t, "/")), name)
	}

	// Each app serves the action it declares, on both of its instances.
	for _, name := range []string{"frontend A", "frontend B"} {
		tab := all[name].Load(t, "/")
		require.Equal(t, http.StatusOK,
			tab.Do(t, http.MethodPost, "/notice/", text("hi")).Status, name)
	}
	for _, name := range []string{"admin A", "admin B"} {
		tab := all[name].Load(t, "/")
		require.Equal(t, http.StatusOK,
			tab.Do(t, http.MethodPost, "/report/", report(1)).Status, name)
	}
}

// TestOneInstanceDoesNotReachAnother tests an event and a session against the
// second instance of the same app. Neither may cross: the brokers and the
// session managers are per instance, and nothing generated may hold either.
func TestOneInstanceDoesNotReachAnother(t *testing.T) {
	t.Parallel()
	all := instances(t)
	a, b := all["frontend A"], all["frontend B"]

	tabA := a.Load(t, "/")
	streamA := tabA.Connect(t, nil)
	tabB := b.Load(t, "/")
	streamB := tabB.Connect(t, nil)

	tabA.Do(t, http.MethodPost, "/notice/", text("first")).WantStatus(t, http.StatusOK)
	streamA.WaitFor(t, datapagestest.TextContains("out", "first"),
		"the instance that dispatched the event did not receive it")

	// The notice B dispatches next is the anchor: had the event of A reached the
	// other instance, it would have arrived before it.
	tabB.Do(t, http.MethodPost, "/notice/", text("second")).WantStatus(t, http.StatusOK)
	streamB.Absent(t,
		datapagestest.TextContains("out", "first"),
		datapagestest.TextContains("out", "second"),
		"the event reached the other instance of the same app")

	tabA.Do(t, http.MethodPost, "/sign-in/", signIn("alice", "al")).
		WantStatus(t, http.StatusOK)
	require.Equal(t, "user=alice nickname=al", echo(t, a.Get(t, "/")))
	require.Equal(t, "anonymous", echo(t, b.Get(t, "/")),
		"a session issued by one instance showed up in the other")
}

// TestBothMeteredInstancesCount tests the collectors the 2 admin servers
// register on 1 registry. A second registration that failed or that replaced
// the first would leave the counter standing still.
func TestBothMeteredInstancesCount(t *testing.T) {
	t.Parallel()
	all := instances(t)

	for _, name := range []string{"admin A", "admin B"} {
		tab := all[name].Load(t, "/")
		before := requestsTotal(t)
		require.Equal(t, http.StatusOK,
			tab.Do(t, http.MethodPost, "/report/", report(1)).Status)
		require.Greater(t, requestsTotal(t), before,
			"%s served a request the counter did not see", name)
	}
}
