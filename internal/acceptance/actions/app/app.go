// Package app exercises action handlers: their methods,
// their parameters and their return values.
//
// Every action records what it received. A test calls the action and then
// reads the record back over a page load. The assertion is that the handler ran with
// the arguments the request carried, not that some string appears in generated source.
package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct {
	mu  sync.Mutex
	log []string
}

func (a *App) record(format string, args ...any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.log = append(a.log, fmt.Sprintf(format, args...))
}

func (a *App) entries() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return strings.Join(a.log, "\n")
}

func echo(s string) datapages.Component {
	return templ.Raw("<pre id=\"echo\">" + s + "</pre>")
}

// Head is the head every page and every rendering action shares.
func (a *App) Head(_ *http.Request) datapages.Head {
	return templ.Raw(`<title>actions</title>`)
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return echo("index"), nil
}

// PageLog is /log
//
// Reads back what the actions recorded.
type PageLog struct{ App *App }

func (p PageLog) GET(_ *http.Request) (body datapages.Component, err error) {
	return echo(p.App.entries()), nil
}

// PageForm is /form
type PageForm struct{ App *App }

func (PageForm) GET(_ *http.Request) (body datapages.Component, err error) {
	return echo("form"), nil
}

// POSTSubmit is /form/submit
//
// The plain shape: signals in, nothing out but an error.
func (p PageForm) POSTSubmit(
	_ *http.Request,
	signals datapages.Signals[struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}],
) error {
	p.App.record("submit name=%q age=%d", signals.Values.Name, signals.Values.Age)
	return nil
}

// PUTReplace is /form/replace
func (p PageForm) PUTReplace(_ *http.Request) error {
	p.App.record("replace")
	return nil
}

// PATCHTouch is /form/touch
func (p PageForm) PATCHTouch(_ *http.Request) error {
	p.App.record("touch")
	return nil
}

// DELETERemove is /form/remove
func (p PageForm) DELETERemove(_ *http.Request) error {
	p.App.record("remove")
	return nil
}

// POSTBump is /form/{id}/bump
//
// Path variables and query parameters reach actions the same way they reach page loads.
func (p PageForm) POSTBump(
	_ *http.Request,
	path datapages.Path[struct {
		ID int `path:"id"`
	}],
	query datapages.Query[struct {
		By int `query:"by"`
	}],
) error {
	p.App.record("bump id=%d by=%d", path.Values.ID, query.Values.By)
	return nil
}

// POSTRender is /form/render
//
// An action that answers with a document rather than with a patch.
func (p PageForm) POSTRender(_ *http.Request) (
	body datapages.Component, err error,
) {
	p.App.record("render")
	return echo("rendered by an action"), nil
}

// POSTGo is /form/go
func (p PageForm) POSTGo(_ *http.Request) (
	redirect datapages.Redirect, err error,
) {
	p.App.record("go")
	return datapages.Redirect{URL: "/log/", Status: http.StatusSeeOther}, nil
}

// POSTGoStream is /form/go-stream
//
// An action that holds the stream of its own request and redirects.
// The response head is gone by then, hence the navigation travels through the stream.
func (p PageForm) POSTGoStream(_ *http.Request, sse datapages.SSE) (
	redirect datapages.Redirect, err error,
) {
	p.App.record("go-stream")
	return datapages.Redirect{URL: "/log/"}, nil
}

// POSTPatch is /form/patch
//
// An action that writes on the SSE connection of the request itself.
func (p PageForm) POSTPatch(
	_ *http.Request,
	sse datapages.SSE,
	signals datapages.Signals[struct {
		Count int `json:"count"`
	}],
) error {
	p.App.record("patch count=%d", signals.Values.Count)
	if err := sse.PatchElement(
		templ.Raw(fmt.Sprintf(`<div id="out">count %d</div>`, signals.Values.Count+1)),
	); err != nil {
		return err
	}
	return sse.PatchSignals(struct {
		Count int `json:"count"`
	}{Count: signals.Values.Count + 1})
}

// POSTPatchAt is /form/patch-at
//
// An action that patches a named target in a given mode.
// An empty selector patches by element id, an empty mode morphs.
func (p PageForm) POSTPatchAt(
	_ *http.Request,
	sse datapages.SSE,
	signals datapages.Signals[struct {
		Selector string `json:"selector"`
		Mode     string `json:"mode"`
	}],
) error {
	p.App.record("patchAt selector=%q mode=%q", signals.Values.Selector, signals.Values.Mode)
	return sse.PatchElementAt(
		templ.Raw(`<div id="out">patched</div>`),
		signals.Values.Selector,
		datapages.PatchMode(signals.Values.Mode),
	)
}

// POSTSignalsRaw is /form/signals-raw
//
// Signals given as JSON go out as they are.
func (p PageForm) POSTSignalsRaw(_ *http.Request, sse datapages.SSE) error {
	p.App.record("signals raw")
	return sse.PatchSignals(json.RawMessage(`{"count":7}`))
}

// POSTSignalsMissing is /form/signals-missing
func (p PageForm) POSTSignalsMissing(_ *http.Request, sse datapages.SSE) error {
	p.App.record("signals missing")
	return sse.PatchSignalsIfMissing(struct {
		Count int `json:"count"`
	}{Count: 3})
}

// POSTSignalsBad is /form/signals-bad
//
// A value that does not marshal must come back as an error.
func (p PageForm) POSTSignalsBad(_ *http.Request, sse datapages.SSE) error {
	p.App.record("signals bad")
	return sse.PatchSignals(make(chan int))
}

// POSTRemove is /form/remove
//
// An action that removes an element from the DOM.
func (p PageForm) POSTRemove(_ *http.Request, sse datapages.SSE) error {
	p.App.record("remove")
	return sse.RemoveElement("#gone")
}

// POSTPing is /ping
//
// An action on the app rather than on a page.
// It is reachable from every page and belongs to none.
func (a *App) POSTPing(_ *http.Request) error {
	a.record("ping")
	return nil
}

// DELETEAll is /all
func (a *App) DELETEAll(_ *http.Request) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.log = nil
	return nil
}
