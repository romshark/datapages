// Package app takes its types from packages named after ones app_gen.go imports.
// Unaliased, one identifier in the generated file would name two packages
// and every type built from them would resolve to nothing.
//
// The app package carrying such a name is a different case: it needs a package
// clause this module cannot have twice, and the parser fixture
// pkgname_collides covers it. Everything else the model reaches is here:
// the session data type, and the path, query and signal field types.
//
// A route wildcard named after a package belongs to hreflocals, which is where
// the names the URL writers bind are covered.
package app

import (
	"fmt"
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/pkgnames/stream"
	"github.com/romshark/datapages/internal/acceptance/pkgnames/strings"
)

type App struct{}

// Session carries its data from a package named after runtime/stream.
type Session = datapages.Session[stream.SessionData]

// EventTicked is "ticked"
//
// The event gives PageIndex a stream, which is a second generated handler that
// renders the session data type: it reads the session before subscribing.
type EventTicked struct {
	Code strings.Code `json:"code"`
}

func echo(format string, args ...any) datapages.Component {
	return templ.Raw(`<pre id="echo">` + fmt.Sprintf(format, args...) + `</pre>`)
}

// PageIndex is /
type PageIndex struct{ App *App }

// GET takes the session, which is how the parser finds the Session type the
// generated server is generic over.
func (PageIndex) GET(
	_ *http.Request, session Session,
) (body datapages.Component, err error) {
	return echo("index nickname=%q", session.Data().Nickname), nil
}

// StreamOpen takes the session, which makes the generated stream handler read
// one before it subscribes.
func (PageIndex) StreamOpen(
	_ *http.Request, _ datapages.StreamID, session Session,
) error {
	_ = session
	return nil
}

func (PageIndex) OnTicked(event EventTicked, sse datapages.SSE) error {
	return sse.PatchElement(echo("ticked code=%s", event.Code))
}

// POSTTick is /tick
func (PageIndex) POSTTick(
	_ *http.Request,
	signals datapages.Signals[struct {
		Code strings.Code `json:"code"`
	}],
	ticked datapages.Dispatcher[EventTicked],
) error {
	return ticked.Dispatch(EventTicked{Code: signals.Values.Code})
}

// PageItem is /item/{id}
//
// The path, query and signal values are all typed from the package named after
// the standard library one app_gen.go imports. Each is parsed by the generated
// value readers, which name the type on every conversion they write.
type PageItem struct{ App *App }

func (PageItem) GET(
	_ *http.Request,
	path datapages.Path[struct {
		ID strings.Code `path:"id"`
	}],
	query datapages.Query[struct {
		Code  strings.Code  `query:"code"`
		Count strings.Count `query:"count"`
		Slug  strings.Slug  `query:"slug"`
	}],
) (body datapages.Component, err error) {
	return echo("id=%s code=%s count=%d slug=%s",
		path.Values.ID, query.Values.Code,
		query.Values.Count, query.Values.Slug), nil
}

// POSTSave is /item/{id}/save
func (PageItem) POSTSave(
	_ *http.Request,
	path datapages.Path[struct {
		ID strings.Code `path:"id"`
	}],
	query datapages.Query[struct {
		Count strings.Count `query:"count"`
	}],
	signals datapages.Signals[struct {
		Code strings.Code `json:"code"`
	}],
	sse datapages.SSE,
) error {
	return sse.PatchElement(echo("saved id=%s count=%d signal=%s",
		path.Values.ID, query.Values.Count, signals.Values.Code))
}
