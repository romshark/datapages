package acceptance_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/sessions/app"
	"github.com/romshark/datapages/internal/acceptance/sessions/app/datapagesgen"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/sessions"
)

// mustNewServer builds the server the way datapages.NewServer does and fails the
// test on a configuration error, which no test can carry on from.
func mustNewServer(
	t *testing.T, a *app.App, broker messaging.Broker,
	sessions sessions.Manager[app.SessionData],
	opts ...datapages.ServerOption,
) datapages.Server {
	t.Helper()
	s, err := datapages.NewServer[
		app.App,
		app.SessionData,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](
		a, broker,
		append([]datapages.ServerOption{
			datapages.WithSessionManager[app.SessionData](sessions),
		}, opts...)...,
	)
	require.NoError(t, err)
	return s
}
