// Package app exercises the values a GET handler may return besides its body.
//
// Each one changes the response, not the page. A redirect leaves the page unrendered.
// The two streaming flags decide whether a page with a stream carries the
// attribute that reloads it when the tab becomes visible again.
package app

import (
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

func echo(s string) datapages.Component {
	return templ.Raw("<pre id=\"echo\">" + s + "</pre>")
}

// EventPing is "ping"
//
// Its only purpose is to give a page a stream.
type EventPing struct {
	N int `json:"n"`
}

// PageIndex is /
//
// The plain shape. Without a stream it misses no event and does not reload.
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return echo("index"), nil
}

// PageLive is /live
//
// A page whose stream closes with the tab. It reloads to render the events
// missed while it was hidden.
type PageLive struct{ App *App }

func (PageLive) GET(_ *http.Request) (body datapages.Component, err error) {
	return echo("live"), nil
}

func (PageLive) OnPing(event EventPing, sse datapages.SSE) error {
	return sse.PatchElement(echo("ping"))
}

// PageGone is /gone
//
// A page load that sends the visitor elsewhere instead of rendering.
type PageGone struct{ App *App }

func (PageGone) GET(_ *http.Request) (
	body datapages.Component, redirect datapages.Redirect, err error,
) {
	return echo("never rendered"), datapages.Redirect{URL: "/"}, nil
}

// PageMaybe is /maybe
//
// The same handler renders or redirects depending on the request.
type PageMaybe struct{ App *App }

func (PageMaybe) GET(
	_ *http.Request,
	query datapages.Query[struct {
		Go bool `query:"go"`
	}],
) (body datapages.Component, redirect datapages.Redirect, err error) {
	if query.Values.Go {
		return nil, datapages.Redirect{
			URL:    "/",
			Status: http.StatusMovedPermanently,
		}, nil
	}
	return echo("stayed"), redirect, nil
}

// PageBackground is /background
//
// A page whose stream keeps running while the tab is hidden.
type PageBackground struct{ App *App }

func (PageBackground) GET(_ *http.Request) (
	body datapages.Component,
	bgStreaming datapages.EnableBackgroundStreaming,
	err error,
) {
	return echo("background"), true, nil
}

func (PageBackground) OnPing(event EventPing, sse datapages.SSE) error {
	return sse.PatchElement(echo("ping"))
}

// PageNoRefresh is /no-refresh
//
// A page that must not reload itself when the tab becomes visible again.
type PageNoRefresh struct{ App *App }

func (PageNoRefresh) GET(_ *http.Request) (
	body datapages.Component,
	noRefresh datapages.DisableRefreshAfterHidden,
	err error,
) {
	return echo("no refresh"), true, nil
}

func (PageNoRefresh) OnPing(event EventPing, sse datapages.SSE) error {
	return sse.PatchElement(echo("ping"))
}
