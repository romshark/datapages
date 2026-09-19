package app

import (
	"github.com/romshark/datapages/example/offline-cache/app/datapagesgen/assets"
	"github.com/romshark/datapages/modules/offline"
)

// OfflineWorkerVersion is the service worker's own version. Bump it when the
// worker script or the precached shell/offline set changes.
const OfflineWorkerVersion = 3

// OfflineConfig builds the offline module configuration. The PageOffline route is
// supplied by the generated datapagesgen.WithOffline option, not configured here.
// Per-page offline snapshots are written by handlers through the pageCache
// parameter (see PageTicket.GET).
func OfflineConfig() offline.Config {
	return offline.Config{
		WorkerVersion: OfflineWorkerVersion,
		Assets: []string{
			assets.Path("style.css"),
			assets.Path("basecoat.css"),
			assets.Path("basecoat.js"),
			assets.Path("datastar.js"),
			assets.Path("favicon.svg"),
		},
	}
}
