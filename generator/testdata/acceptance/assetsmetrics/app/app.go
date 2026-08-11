// Package app exercises the two generator options a datapages.yaml carries
// that change what is generated: static asset serving and Prometheus metrics.
package app

import (
	"net/http"

	"dpacceptance/datapagesgen/httperr"
	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

type App struct{}

// EventAnnounced is "announced"
type EventAnnounced struct {
	Text string `json:"text"`
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body templ.Component, err error) {
	return templ.Raw(`<pre id="echo">index</pre>`), nil
}

func (PageIndex) OnAnnounced(
	event EventAnnounced,
	sse *datastar.ServerSentEventGenerator,
) error {
	return sse.PatchElementTempl(
		templ.Raw(`<div id="out">` + event.Text + `</div>`))
}

// POSTAnnounce is /announce
//
// Publishing is what the broker metrics count.
func (PageIndex) POSTAnnounce(
	_ *http.Request,
	signals struct {
		Text string `json:"text"`
	},
	dispatch func(EventAnnounced) error,
) error {
	return dispatch(EventAnnounced{Text: signals.Text})
}

// POSTFail is /fail
//
// Errors are counted. One is needed here to count.
func (PageIndex) POSTFail(_ *http.Request) error {
	return httperr.BadRequest
}
