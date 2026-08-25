// Package app is the one application of a module whose datapages.NewServer
// calls are spread over several packages: the command, a library package
// beside it and one nested below the module root.
//
// Every call names this app and the same four type arguments. The case exists
// to check that the scan reads all of them and generates this package once.
package app

import (
	"fmt"
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

// EventTick is "tick"
type EventTick struct {
	N int `json:"n"`
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<pre id="echo">index</pre>`), nil
}

func (PageIndex) OnTick(
	event EventTick,
	sse datapages.SSE,
) error {
	return sse.PatchElement(templ.Raw(
		fmt.Sprintf(`<div id="out">tick %d</div>`, event.N),
	))
}

// POSTTick is /tick
func (PageIndex) POSTTick(
	_ *http.Request,
	signals datapages.Signals[struct {
		N int `json:"n"`
	}],
	tick datapages.Dispatcher[EventTick],
) error {
	return tick.Dispatch(EventTick{N: signals.Values.N})
}
