// Package app is a small application used to exercise the API of the server
// the generator writes: its options, its lifecycle and what it exports.
package app

import (
	"fmt"
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

type App struct{}

// EventPing is "ping"
type EventPing struct {
	N int `json:"n"`
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body templ.Component, err error) {
	return templ.Raw(`<pre id="echo">index</pre>`), nil
}

func (PageIndex) OnPing(
	event EventPing,
	sse *datastar.ServerSentEventGenerator,
) error {
	return sse.PatchElementTempl(templ.Raw(
		fmt.Sprintf(`<div id="out">ping %d</div>`, event.N)))
}

// POSTPing is /ping
//
// The smallest thing that puts an event on a stream: one action, one
// dispatch, one handler.
func (PageIndex) POSTPing(
	_ *http.Request,
	signals struct {
		N int `json:"n"`
	},
	dispatch func(EventPing) error,
) error {
	return dispatch(EventPing{N: signals.N})
}
