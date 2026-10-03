// Package app embeds its assets with a single file, not a directory.
package app

import (
	"embed"
	"net/http"

	"github.com/romshark/datapages"
)

// AssetsFS is /static/
//
//go:embed static/style.css
var AssetsFS embed.FS

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}
