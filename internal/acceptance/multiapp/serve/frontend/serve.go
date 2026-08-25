// Package frontend builds the frontend server.
//
// The datapages.NewServer call sits here rather than in the command,
// and it is the only call of the module naming the frontend app package.
// A scan that read commands alone would find neither this one nor the admin one.
package frontend

import (
	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/multiapp/app/frontend"
	"github.com/romshark/datapages/internal/acceptance/multiapp/app/frontend/datapagesgen"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/sessions"
)

// New builds the frontend server. It carries sessions and no metrics.
func New(
	a *frontend.App, broker messaging.Broker,
	manager sessions.Manager[frontend.SessionData],
	opts ...datapages.ServerOption,
) (datapages.Server, error) {
	return datapages.NewServer[
		frontend.App,
		frontend.SessionData,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](a, broker, append([]datapages.ServerOption{
		datapages.WithSessionManager(manager),
	}, opts...)...)
}
