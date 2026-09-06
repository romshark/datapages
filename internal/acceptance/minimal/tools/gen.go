//go:build ignore

// Command gen is what no build configuration compiles.
// Its NewServer call contradicts the one in cmd/server on the metrics mode:
// the scan has to skip it, or generating this module fails.
package main

import (
	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/minimal/app"
	"github.com/romshark/datapages/internal/acceptance/minimal/app/datapagesgen"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func main() {
	_, _ = datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.EnablePrometheus,
		datapagesgen.Server,
	](&app.App{}, inmem.New(8))
}
