// Tests two instances of each application running in one process.
//
// A generated package is imported once and built any number of times. State
// kept in a package variable would be shared by every server built from it:
// the first would pass and the second would read what the first left behind.
// Four servers here, two per app, each with its own broker, its own session
// manager and its own app value.

package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/client"
	adminapp "github.com/romshark/datapages/internal/acceptance/multiapp/app/admin"
	frontendapp "github.com/romshark/datapages/internal/acceptance/multiapp/app/frontend"
)

// instances builds a server per entry and returns the clients driving them,
// keyed the way the assertions name them.
func instances(t *testing.T) map[string]*client.Client {
	t.Helper()
	return map[string]*client.Client{
		"frontend A": client.New(t,
			mustNewFrontend(t, &frontendapp.App{}, broker())).WithJar(t),
		"frontend B": client.New(t,
			mustNewFrontend(t, &frontendapp.App{}, broker())).WithJar(t),
		"admin A": client.New(t, mustNewAdmin(t, &adminapp.App{}, broker())),
		"admin B": client.New(t, mustNewAdmin(t, &adminapp.App{}, broker())),
	}
}

// TestFourInstancesServeTheirOwnApp tests all four answering at once. Two
// servers of one app must not collide over a route, a subject or a collector.
func TestFourInstancesServeTheirOwnApp(t *testing.T) {
	t.Parallel()
	all := instances(t)

	for name, want := range map[string]string{
		"frontend A": "anonymous",
		"frontend B": "anonymous",
		"admin A":    "admin",
		"admin B":    "admin",
	} {
		require.Equal(t, want, all[name].Get(t, "/").Element(t, "echo"), name)
	}

	// Each app serves the action it declares, on both of its instances.
	for _, name := range []string{"frontend A", "frontend B"} {
		require.Equal(t, http.StatusOK,
			all[name].Action(t, http.MethodPost, "/notice/", `{"text":"hi"}`).Status,
			name)
	}
	for _, name := range []string{"admin A", "admin B"} {
		require.Equal(t, http.StatusOK,
			all[name].Action(t, http.MethodPost, "/report/", `{"n":1}`).Status,
			name)
	}
}

// TestOneInstanceDoesNotReachAnother tests an event and a session against the
// second instance of the same app. Neither may cross: the brokers and the
// session managers are per instance, and nothing generated may hold either.
func TestOneInstanceDoesNotReachAnother(t *testing.T) {
	t.Parallel()
	all := instances(t)
	a, b := all["frontend A"], all["frontend B"]

	streamA := a.OpenStream(t, "/_$/", nil)
	defer streamA.Close()
	streamB := b.OpenStream(t, "/_$/", nil)
	defer streamB.Close()

	require.Equal(t, http.StatusOK,
		a.Action(t, http.MethodPost, "/notice/", `{"text":"first"}`).Status)
	require.True(t, streamA.Saw(`<div id="out">first</div>`),
		"the instance that dispatched the event did not receive it")
	require.True(t, streamB.Never(`<div id="out">first</div>`),
		"the event reached the other instance of the same app")

	require.Equal(t, http.StatusOK,
		a.Action(t, http.MethodPost, "/sign-in/",
			`{"user":"alice","nickname":"al"}`).Status)
	require.Equal(t, "user=alice nickname=al", a.Get(t, "/").Element(t, "echo"))
	require.Equal(t, "anonymous", b.Get(t, "/").Element(t, "echo"),
		"a session issued by one instance showed up in the other")
}

// TestBothMeteredInstancesCount tests the collectors the two admin servers
// register on one registry. A second registration that failed or that replaced
// the first would leave the counter standing still.
func TestBothMeteredInstancesCount(t *testing.T) {
	t.Parallel()
	all := instances(t)

	for _, name := range []string{"admin A", "admin B"} {
		before := requestsTotal(t)
		require.Equal(t, http.StatusOK,
			all[name].Action(t, http.MethodPost, "/report/", `{"n":1}`).Status)
		require.Greater(t, requestsTotal(t), before,
			"%s served a request the counter did not see", name)
	}
}
