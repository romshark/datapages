package acceptance_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/recovererror/app"
	"github.com/romshark/datapages/internal/acceptance/recovererror/app/datapagesgen"
	"github.com/romshark/datapages/modules/messaging"
)

// registry receives the metrics of every server the tests build.
// The counters are global to the process, and 1 registry reads them all.
var registry = prometheus.NewRegistry()

// mustNewServer builds the server the way datapages.NewServer does and fails the
// test on a configuration error, which no test can carry on from.
// It adds datapages.WithPrometheus, which the server requires.
func mustNewServer(
	t *testing.T, a *app.App, broker messaging.Broker,
	opts ...datapages.ServerOption,
) datapages.Server {
	t.Helper()
	opts = append([]datapages.ServerOption{
		datapages.WithPrometheus(datapages.PrometheusConfig{
			Host:       "127.0.0.1:0",
			Registerer: registry,
			Gatherer:   registry,
		}),
	}, opts...)
	s, err := datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.EnablePrometheus,
		datapagesgen.Server,
	](a, broker, opts...)
	require.NoError(t, err)
	return s
}
