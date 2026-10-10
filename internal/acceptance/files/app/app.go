// Package app serves files: from GET actions on App and on a page,
// and from a POST action.
package app

import (
	"errors"
	"html"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

// ModTime is the modification time of every file GETFile serves.
var ModTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// RecoverError patches the error into the page. A handler that answers with
// a file must never reach it, a Datastar request included.
func (*App) RecoverError(err error, sse datapages.SSE) error {
	return sse.PatchElement(templ.Raw(
		`<p id="recovered">` + html.EscapeString(err.Error()) + `</p>`))
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw("index"), nil
}

// PageError404 is /not-found
type PageError404 struct{ App *App }

func (PageError404) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<p id="page">not found</p>`), nil
}

// PageError500 is /error
type PageError500 struct{ App *App }

func (PageError500) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<p id="page">internal error</p>`), nil
}

// GETFile is /files/{name}
//
// Serves the name back as text. The query selects a failure.
func (*App) GETFile(
	_ *http.Request,
	path datapages.Path[struct {
		Name string `path:"name"`
	}],
	query datapages.Query[struct {
		Fail     string `query:"fail"`
		Download bool   `query:"download"`
	}],
) (datapages.File, error) {
	switch query.Values.Fail {
	case "missing":
		return datapages.File{}, datapages.ErrNotFound
	case "error":
		return datapages.File{}, errors.New("the handler failed")
	case "panic":
		panic("the handler panicked")
	case "type":
		return datapages.File{Body: strings.NewReader("no type")}, nil
	}
	f := datapages.File{
		Type:         "text/plain; charset=utf-8",
		Body:         strings.NewReader("content of " + path.Values.Name),
		ModTime:      ModTime,
		CacheControl: "public, max-age=60",
	}
	if query.Values.Download {
		f.Filename = path.Values.Name
	}
	return f, nil
}

// POSTUpper is /upper
//
// Answers with the request body in upper case. An empty body is a 400.
func (*App) POSTUpper(r *http.Request) (datapages.File, error) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return datapages.File{}, err
	}
	if len(b) == 0 {
		return datapages.File{}, datapages.ErrBadRequest
	}
	return datapages.File{
		Type: "text/plain; charset=utf-8",
		Body: strings.NewReader(strings.ToUpper(string(b))),
	}, nil
}

// PageDoc is /doc/{id}
type PageDoc struct{ App *App }

func (PageDoc) GET(
	_ *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
) (body datapages.Component, err error) {
	return templ.Raw(`<p id="doc">` + html.EscapeString(path.Values.ID) + `</p>`), nil
}

// GETExport is /doc/{id}/export
func (PageDoc) GETExport(
	_ *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
) (datapages.File, error) {
	return datapages.File{
		Type: "text/plain; charset=utf-8",
		Body: strings.NewReader("export of " + path.Values.ID),
	}, nil
}
