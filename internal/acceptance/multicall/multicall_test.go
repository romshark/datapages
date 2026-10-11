// Tests the servers the 3 constructors of this module build.
//
// A module may build 1 application from any number of packages.
// The app package is parsed once and generated once,
// whichever call asked for the server, hence all 3 servers serve the same routes.

package acceptance_test

import (
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/internal/acceptance/multicall/app"
	"github.com/romshark/datapages/internal/acceptance/multicall/app/datapagesgen/action"
	"github.com/romshark/datapages/internal/acceptance/multicall/app/datapagesgen/href"
	"github.com/romshark/datapages/internal/acceptance/multicall/internal/boot"
	"github.com/romshark/datapages/internal/acceptance/multicall/serve"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

// constructors are the ways this module builds its only server:
// the call in the test package, the one in serve and the one in internal/boot.
var constructors = map[string]func(*testing.T) datapages.Server{
	"test package": func(t *testing.T) datapages.Server {
		t.Helper()
		return mustNewServer(t, &app.App{},
			inmem.New(messaging.DefaultBrokerChanBuffer))
	},
	"serve": func(t *testing.T) datapages.Server {
		t.Helper()
		s, err := serve.New(&app.App{},
			inmem.New(messaging.DefaultBrokerChanBuffer))
		require.NoError(t, err)
		return s
	},
	"internal/boot": func(t *testing.T) datapages.Server {
		t.Helper()
		s, err := boot.New(&app.App{},
			inmem.New(messaging.DefaultBrokerChanBuffer),
			slog.New(slog.NewTextHandler(io.Discard, nil)))
		require.NoError(t, err)
		return s
	},
}

// TestEveryConstructorServesTheSameApp tests a page, an action and the event
// it dispatches against a server from each constructor.
func TestEveryConstructorServesTheSameApp(t *testing.T) {
	t.Parallel()
	for name, newServer := range constructors {
		t.Run(name, func(t *testing.T) {
			v := datapagestest.New(t, newServer(t)).Visitor(t)
			tab := v.Load(t, href.PageIndex())
			require.Equal(t, "index",
				datapagestest.Text(tab.Page().ElementByID(t, "echo")))

			st := tab.Connect(t, nil)

			resp := tab.Do(t, http.MethodPost, "/tick/", map[string]int{"n": 7})
			require.Equal(t, http.StatusOK, resp.Status)
			st.WaitFor(t, datapagestest.TextContains("out", "tick 7"))
		})
	}
}

// TestActionsAre1SetOfRoutes tests the generated action expressions against
// each server. 1 app package is generated once, hence the routes a caller
// reaches do not depend on which call built the server.
func TestActionsAre1SetOfRoutes(t *testing.T) {
	t.Parallel()
	require.Equal(t, "@post('/tick/')", action.PageIndex.Tick.POST())

	for name, newServer := range constructors {
		t.Run(name, func(t *testing.T) {
			v := datapagestest.New(t, newServer(t)).Visitor(t)
			tab := v.Load(t, href.PageIndex())
			resp := tab.Do(t, http.MethodPost, "/tick/", map[string]int{"n": 1})
			require.Equal(t, http.StatusOK, resp.Status)
		})
	}
}
