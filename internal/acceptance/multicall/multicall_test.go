// Covers the servers the three constructors of this module build.
//
// A module may build one application from any number of packages.
// The app package is parsed once and generated once,
// whichever call asked for the server, hence all three servers serve the same routes.

package acceptance_test

import (
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/internal/acceptance/multicall/app"
	"github.com/romshark/datapages/internal/acceptance/multicall/app/datapagesgen/action"
	"github.com/romshark/datapages/internal/acceptance/multicall/app/datapagesgen/href"
	"github.com/romshark/datapages/internal/acceptance/multicall/internal/boot"
	"github.com/romshark/datapages/internal/acceptance/multicall/serve"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

// constructors are the ways this module builds its one server:
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

// TestEveryConstructorServesTheSameApp covers a page, an action and the event
// it dispatches against a server from each constructor.
func TestEveryConstructorServesTheSameApp(t *testing.T) {
	t.Parallel()
	for name, newServer := range constructors {
		t.Run(name, func(t *testing.T) {
			c := client.New(t, newServer(t))

			resp := c.Get(t, href.PageIndex())
			require.Equal(t, http.StatusOK, resp.Status)
			require.Equal(t, "index", resp.Element(t, "echo"))

			s := c.OpenStream(t, "/_$/", nil)
			defer s.Close()

			c.Action(t, http.MethodPost, "/tick/", `{"n":7}`)
			require.True(t, s.Saw(`<div id="out">tick 7</div>`))
		})
	}
}

// TestActionsAreOneSetOfRoutes covers the generated action expressions against
// each server. One app package is generated once, hence the routes a caller
// reaches do not depend on which call built the server.
func TestActionsAreOneSetOfRoutes(t *testing.T) {
	t.Parallel()
	require.Equal(t, "@post('/tick/')", action.POSTPageIndexTick())

	for name, newServer := range constructors {
		t.Run(name, func(t *testing.T) {
			c := client.New(t, newServer(t))
			resp := c.Action(t, http.MethodPost, "/tick/", `{"n":1}`)
			require.Equal(t, http.StatusOK, resp.Status)
		})
	}
}
