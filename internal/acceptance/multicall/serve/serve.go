// Package serve builds the server of this module.
//
// The datapages.NewServer call lives here rather than in the command,
// which is what a module does when more than one entry point needs the same server.
package serve

import (
	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/multicall/app"
	"github.com/romshark/datapages/internal/acceptance/multicall/app/datapagesgen"
	"github.com/romshark/datapages/modules/messaging"
)

// New builds the server for a.
func New(
	a *app.App, broker messaging.Broker, opts ...datapages.ServerOption,
) (datapages.Server, error) {
	return datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](a, broker, opts...)
}
