//nolint:all

package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

type Session = datapages.Session[struct{}]

// GETImage is /images/{name}
func (*App) GETImage(
	r *http.Request,
	path datapages.Path[struct {
		Name string `path:"name"`
	}],
	query datapages.Query[struct {
		Width int `query:"w"`
	}],
) (file datapages.File, err error) {
	return file, nil
}

// GETFeed is /feed.xml
//
// Takes nothing but the request and returns no error.
func (*App) GETFeed(r *http.Request) datapages.File {
	return datapages.File{}
}

// POSTExport is /export
//
// A non-GET action may return a file too.
func (*App) POSTExport(
	r *http.Request,
	session Session,
	signals datapages.Signals[struct {
		Format string `json:"format"`
	}],
) (datapages.File, error) {
	return datapages.File{}, nil
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

// PageDoc is /doc/{id}
type PageDoc struct {
	App *App
	Downloads
}

func (PageDoc) GET(
	r *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
) (body datapages.Component, err error) {
	return body, err
}

// GETExport is /doc/{id}/export
func (PageDoc) GETExport(
	r *http.Request,
	session Session,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
) (file datapages.File, err error) {
	return file, nil
}

// Downloads is an abstract page. Every page embedding it serves its GET action,
// and its GET action is not the GET of that page.
type Downloads struct{ App *App }

// GETManual is /manual.pdf
func (Downloads) GETManual(r *http.Request) (file datapages.File, err error) {
	return file, nil
}
