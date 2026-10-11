// Package app declares routes below the URL prefix its assets are served under,
// which net/http accepts as more specific than the prefix.
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

// GETThumb is /static/thumbs/{name}
func (*App) GETThumb(r *http.Request, path datapages.Path[struct {
	Name string `path:"name"`
}]) (datapages.File, error) {
	return datapages.File{}, nil
}

// POSTUpload is /static/upload
func (*App) POSTUpload(r *http.Request) error { return nil }

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// PageAbout is /static/about
//
// Reported once: its action is below the prefix because the page is.
type PageAbout struct{ App *App }

func (PageAbout) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// POSTLike is /static/about/like
func (PageAbout) POSTLike(r *http.Request) error { return nil }

// PageStatics is /statics
//
// Starts with the letters of the prefix, not with the prefix.
type PageStatics struct{ App *App }

func (PageStatics) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}
