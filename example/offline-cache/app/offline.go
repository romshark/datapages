package app

import (
	"github.com/romshark/datapages/example/offline-cache/app/datapagesgen/assets"
	"github.com/romshark/datapages/modules/offline"
)

// OfflineWorkerVersion is the service worker's own version. Increment it when
// the worker script or precached files change.
const OfflineWorkerVersion = 4

// OfflineConfig returns the offline module configuration. The generated
// datapagesgen.WithOffline option supplies the PageOffline route.
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
