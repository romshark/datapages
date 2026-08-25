// Package app serves a page under the URL prefix its assets are served under.
package app

import (
	"embed"
	"net/http"

	"github.com/romshark/datapages"
)

// AssetsFS is /static/
//
//go:embed static/*
var AssetsFS embed.FS

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// PageStaticFile is /static/{rest...}
type PageStaticFile struct{ App *App }

func (PageStaticFile) GET(r *http.Request, path datapages.Path[struct {
	Rest string `path:"rest"`
}]) (body datapages.Component, err error) {
	return nil, nil
}
