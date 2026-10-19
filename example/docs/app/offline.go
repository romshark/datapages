package app

import (
	"github.com/romshark/datapages/example/docs/app/datapagesgen/assets"
	"github.com/romshark/datapages/modules/offline"
)

// OfflineWorkerVersion identifies the installed service worker and its cache.
// Increment it after a Datapages upgrade, a change to [PageOffline] or to the
// list of files in [OfflineConfig]: the worker fetches them when it installs.
const OfflineWorkerVersion = 2

// OfflineConfig stores what [PageOffline] needs to open a document offline.
func OfflineConfig() offline.Config {
	return offline.Config{
		WorkerVersion: OfflineWorkerVersion,
		Assets: []string{
			assets.Path("datastar-1.0.4.js"),
			assets.Path("editor.css"),
			assets.Path("editor.js"),
			assets.Path("favicon.svg"),
			assets.Path("style.css"),
		},
	}
}
