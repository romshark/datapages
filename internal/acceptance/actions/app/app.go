// Package app exercises action handlers: their methods,
// their parameters and their return values.
//
// Every action records what it received. A test calls the action and then
// reads the record back over a page load. The assertion is that the handler ran with
// the arguments the request carried, not that some string appears in generated source.
package app

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
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

func echo(s string) templ.Component {
	return templ.Raw("<pre id=\"echo\">" + s + "</pre>")
}

// Head is the head every page and every rendering action shares.
func (a *App) Head(_ *http.Request) templ.Component {
	return templ.Raw(`<title>actions</title>`)
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body templ.Component, err error) {
	return echo("index"), nil
}

// PageLog is /log
//
// Reads back what the actions recorded.
type PageLog struct{ App *App }

func (p PageLog) GET(_ *http.Request) (body templ.Component, err error) {
	return echo(p.App.entries()), nil
}

// PageForm is /form
type PageForm struct{ App *App }

func (PageForm) GET(_ *http.Request) (body templ.Component, err error) {
	return echo("form"), nil
}

// POSTSubmit is /form/submit
//
// The plain shape: signals in, nothing out but an error.
func (p PageForm) POSTSubmit(
	_ *http.Request,
	signals struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	},
) error {
	p.App.record("submit name=%q age=%d", signals.Name, signals.Age)
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
	path struct {
		ID int `path:"id"`
	},
	query struct {
		By int `query:"by"`
	},
) error {
	p.App.record("bump id=%d by=%d", path.ID, query.By)
	return nil
}

// POSTRender is /form/render
//
// An action that answers with a document rather than with a patch.
func (p PageForm) POSTRender(_ *http.Request) (
	body templ.Component, err error,
) {
	p.App.record("render")
	return echo("rendered by an action"), nil
}

// POSTGo is /form/go
func (p PageForm) POSTGo(_ *http.Request) (
	redirect string, redirectStatus int, err error,
) {
	p.App.record("go")
	return "/log/", http.StatusSeeOther, nil
}

// POSTPatch is /form/patch
//
// An action that writes on the SSE connection of the request itself.
func (p PageForm) POSTPatch(
	_ *http.Request,
	sse *datastar.ServerSentEventGenerator,
	signals struct {
		Count int `json:"count"`
	},
) error {
	p.App.record("patch count=%d", signals.Count)
	if err := sse.PatchElementTempl(
		templ.Raw(fmt.Sprintf(`<div id="out">count %d</div>`, signals.Count+1)),
	); err != nil {
		return err
	}
	return sse.MarshalAndPatchSignals(struct {
		Count int `json:"count"`
	}{Count: signals.Count + 1})
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
