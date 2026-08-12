// Package app exercises the values a GET handler may return besides its body.
//
// Each one changes the response, not the page. A redirect leaves the page unrendered.
// The two streaming flags decide whether the document carries the
// attribute that reloads it when the tab becomes visible again.
package app

import (
	"net/http"

	"github.com/a-h/templ"
)

type App struct{}

func echo(s string) templ.Component {
	return templ.Raw("<pre id=\"echo\">" + s + "</pre>")
}

// PageIndex is /
//
// The plain shape. Its body carries the reload attribute the other pages switch off.
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body templ.Component, err error) {
	return echo("index"), nil
}

// PageGone is /gone
//
// A page load that sends the visitor elsewhere instead of rendering.
type PageGone struct{ App *App }

func (PageGone) GET(_ *http.Request) (
	body templ.Component, redirect string, err error,
) {
	return echo("never rendered"), "/", nil
}

// PageMaybe is /maybe
//
// The same handler renders or redirects depending on the request.
type PageMaybe struct{ App *App }

func (PageMaybe) GET(
	_ *http.Request,
	query struct {
		Go bool `query:"go"`
	},
) (body templ.Component, redirect string, redirectStatus int, err error) {
	if query.Go {
		return nil, "/", http.StatusMovedPermanently, nil
	}
	return echo("stayed"), "", 0, nil
}

// PageBackground is /background
//
// A page whose stream keeps running while the tab is hidden.
type PageBackground struct{ App *App }

func (PageBackground) GET(_ *http.Request) (
	body templ.Component, enableBackgroundStreaming bool, err error,
) {
	return echo("background"), true, nil
}

// PageNoRefresh is /no-refresh
//
// A page that must not reload itself when the tab becomes visible again.
type PageNoRefresh struct{ App *App }

func (PageNoRefresh) GET(_ *http.Request) (
	body templ.Component, disableRefreshAfterHidden bool, err error,
) {
	return echo("no refresh"), true, nil
}
