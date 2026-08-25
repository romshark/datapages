package acceptance_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/multiapp/app/admin"
	admingen "github.com/romshark/datapages/internal/acceptance/multiapp/app/admin/datapagesgen"
	"github.com/romshark/datapages/internal/acceptance/multiapp/app/frontend"
	frontendgen "github.com/romshark/datapages/internal/acceptance/multiapp/app/frontend/datapagesgen"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/sessions"
	sessinmem "github.com/romshark/datapages/modules/sessions/inmem"
)

// registry is shared. The generated code registers its collectors once per
// process and a second registry would come up empty.
var registry = prometheus.NewRegistry()

// mustNewFrontend builds the frontend server, which carries sessions and no metrics,
// and fails the test on a configuration error.
//
// The other call naming the frontend app package is in serve/frontend.
func mustNewFrontend(
	t *testing.T, a *frontend.App, broker messaging.Broker,
	opts ...datapages.ServerOption,
) datapages.Server {
	t.Helper()
	s, err := datapages.NewServer[
		frontend.App,
		frontend.SessionData,
		datapages.DisablePrometheus,
		frontendgen.Server,
	](a, broker, append([]datapages.ServerOption{
		datapages.WithSessionManager(newSessionManager()),
	}, opts...)...)
	require.NoError(t, err)
	return s
}

// mustNewAdmin builds the admin server, which carries metrics and no sessions,
// and fails the test on a configuration error.
func mustNewAdmin(
	t *testing.T, a *admin.App, broker messaging.Broker,
	opts ...datapages.ServerOption,
) datapages.Server {
	t.Helper()
	s, err := datapages.NewServer[
		admin.App,
		datapages.DisableSessions,
		datapages.EnablePrometheus,
		admingen.Server,
	](a, broker, append([]datapages.ServerOption{
		datapages.WithPrometheus(prometheusConfig()),
	}, opts...)...)
	require.NoError(t, err)
	return s
}

func newSessionManager() sessions.Manager[frontend.SessionData] {
	return sessinmem.New[frontend.SessionData](
		sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
	)
}

func prometheusConfig() datapages.PrometheusConfig {
	return datapages.PrometheusConfig{
		Host:       "127.0.0.1:0",
		Registerer: registry,
		Gatherer:   registry,
	}
}
