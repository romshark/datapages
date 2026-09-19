package acceptance_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/pagesentinels/app"
	"github.com/romshark/datapages/internal/acceptance/pagesentinels/app/datapagesgen"
	"github.com/romshark/datapages/modules/messaging"
)

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
