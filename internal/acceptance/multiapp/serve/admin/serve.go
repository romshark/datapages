// Package admin builds the admin server.
//
// It names datapages.EnablePrometheus where the frontend names
// datapages.DisablePrometheus, and datapages.DisableSessions where the frontend
// names a session type. One run generates the metrics instrumentation into
// app/admin/datapagesgen and the session handling into the frontend's.
package admin

import (
	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/multiapp/app/admin"
	"github.com/romshark/datapages/internal/acceptance/multiapp/app/admin/datapagesgen"
	"github.com/romshark/datapages/modules/messaging"
)

// New builds the admin server. It carries metrics and no sessions.
func New(
	a *admin.App, broker messaging.Broker, metrics datapages.PrometheusConfig,
	opts ...datapages.ServerOption,
) (datapages.Server, error) {
	return datapages.NewServer[
		admin.App,
		datapages.DisableSessions,
		datapages.EnablePrometheus,
		datapagesgen.Server,
	](a, broker, append([]datapages.ServerOption{
		datapages.WithPrometheus(metrics),
	}, opts...)...)
}
