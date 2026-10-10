//nolint:all

package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// EventPing is "ping"
type EventPing struct {
	Data string `json:"data"`
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return body, err
}

/* ErrGETActionInput: sse */

// GETStream is /stream
func (PageIndex) GETStream(
	r *http.Request, sse datapages.SSE,
) (file datapages.File, err error) {
	return file, nil
}

/* ErrGETActionInput: signals */

// GETSignals is /signals
func (PageIndex) GETSignals(
	r *http.Request,
	signals datapages.Signals[struct {
		Name string `json:"name"`
	}],
) (file datapages.File, err error) {
	return file, nil
}

/* ErrGETActionInput: page cache */

// GETCache is /cache
func (*App) GETCache(
	r *http.Request, pageCache datapages.PageCacheWriter,
) (file datapages.File, err error) {
	return file, nil
}

/* ErrGETActionInput: dispatcher */

// GETDispatch is /dispatch
func (*App) GETDispatch(
	r *http.Request, dispatch datapages.Dispatcher[EventPing],
) (file datapages.File, err error) {
	return file, nil
}

/* ErrGETActionMissingFile: no results */

// GETNothing is /nothing
func (*App) GETNothing(r *http.Request) {}

/* ErrGETActionMissingFile: a document */

// GETDocument is /document
func (*App) GETDocument(r *http.Request) (body datapages.Component, err error) {
	return body, nil
}

/* ErrFileWithOutput: a redirect next to the file */

// GETRedirect is /redirect
func (*App) GETRedirect(r *http.Request) (
	file datapages.File, redirect datapages.Redirect, err error,
) {
	return file, redirect, nil
}

/* ErrGETActionNameConflict: href.App.ImageQuery builds the query of GETImage */

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

// GETImageQuery is /image-query
func (*App) GETImageQuery(r *http.Request) (file datapages.File, err error) {
	return file, nil
}
