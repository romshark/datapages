package acceptance_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/actions/app"
	"github.com/romshark/datapages/internal/acceptance/actions/app/datapagesgen"
	"github.com/romshark/datapages/modules/messaging"
)

// mustNewServer builds the server the way datapages.NewServer does and fails the
// test on a configuration error, which no test can carry on from.
func mustNewServer(
	t *testing.T, a *app.App, broker messaging.Broker,
	opts ...datapages.ServerOption,
) datapages.Server {
	t.Helper()
	s, err := datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](a, broker, opts...)
	require.NoError(t, err)
	return s
}
