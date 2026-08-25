// Package admin is the metered application of a module that builds two.
//
// Its server is generated with datapages.DisableSessions and datapages.EnablePrometheus,
// the opposite of frontend in both type arguments.
package admin

import (
	"fmt"
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

// EventReport is "report"
type EventReport struct {
	N int `json:"n"`
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<pre id="echo">admin</pre>`), nil
}

func (PageIndex) OnReport(
	event EventReport,
	sse datapages.SSE,
) error {
	return sse.PatchElement(templ.Raw(
		fmt.Sprintf(`<div id="out">report %d</div>`, event.N),
	))
}

// POSTReport is /report
func (PageIndex) POSTReport(
	_ *http.Request,
	signals datapages.Signals[struct {
		N int `json:"n"`
	}],
	report datapages.Dispatcher[EventReport],
) error {
	return report.Dispatch(EventReport{N: signals.Values.N})
}
