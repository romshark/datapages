// Package app declares on a stream embed the methods it doesn't take.
// A stream embed shares event handlers and stream hooks,
// which are the same on every page that embeds it.
// A GET renders 1 page, and an action has 1 route, which 1 page can serve.
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// EventPing is "ping"
type EventPing struct{}

type Base struct{ App *App }

func (Base) GET(r *http.Request) (body datapages.Component, err error) { /* ErrStreamEmbedUnsupportedMethod */
	return nil, nil
}

// POSTSave is /save
func (Base) POSTSave(r *http.Request) error { return nil } /* ErrStreamEmbedUnsupportedMethod */

// GETManual is /manual.pdf
func (Base) GETManual(r *http.Request) (file datapages.File, err error) { /* ErrStreamEmbedUnsupportedMethod */
	return file, nil
}

func (Base) Format() string { return "" } /* ErrStreamEmbedUnsupportedMethod */

func (Base) format() string { return "" }

func (Base) OnPing(event EventPing, sse datapages.SSE) error { return nil }

func (Base) StreamOpen(r *http.Request, id datapages.StreamID) error { return nil }

func (Base) StreamClose(r *http.Request, id datapages.StreamID) error { return nil }

// PageIndex is /
type PageIndex struct {
	App *App
	Base
}

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// PageNoGET is /no-get
//
// The page does not adopt the GET of Base.
type PageNoGET struct { /* ErrPageMissingGET */
	App *App
	Base
}
