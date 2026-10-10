// Package app exercises PageError404:
// the page and the status a URL that no page claims is answered with.
package app

import (
	"errors"
	"html"
	"net/http"
	"strings"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw("index"), nil
}

// PageError404 is /not-found
//
// The redirect is conditional. A 404 page may answer the request itself
// instead of rendering, which needs its own status and its Location header.
//
// For an unmatched URL, the page receives that URL's query.
type PageError404 struct{ App *App }

func (PageError404) GET(
	r *http.Request,
	query datapages.Query[struct {
		Term string `query:"q"`
	}],
) (
	body datapages.Component,
	redirect datapages.Redirect,
	err error,
) {
	if strings.HasPrefix(r.URL.Path, "/go-home") {
		return nil, datapages.Redirect{URL: "/"}, nil
	}
	return templ.Raw(`<p id="msg">no such page</p>` +
		`<p id="term">` + html.EscapeString(query.Values.Term) + `</p>`), redirect, nil
}

// POSTStreamFail is /stream-fail
//
// The response is committed as an event stream before the action runs.
// With neither PageError500 nor RecoverError there is nothing left to answer with,
// which must stay silence rather than a status written into the stream.
func (PageIndex) POSTStreamFail(_ *http.Request, _ datapages.SSE) error {
	return errors.New("the action failed with the stream open")
}
