// Package boot builds the same server as serve, two directories below the
// module root and under internal.
//
// The scan walks every package of the module. Neither the depth of a package
// nor internal keeps a call out of what it reads.
package boot

import (
	"log/slog"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/multicall/app"
	"github.com/romshark/datapages/internal/acceptance/multicall/app/datapagesgen"
	"github.com/romshark/datapages/modules/messaging"
)

// New builds the server for a. The logger is applied before opts,
// which lets a caller replace it.
func New(
	a *app.App, broker messaging.Broker, logger *slog.Logger,
	opts ...datapages.ServerOption,
) (datapages.Server, error) {
	opts = append([]datapages.ServerOption{datapages.WithLogger(logger)}, opts...)
	return datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](a, broker, opts...)
}
