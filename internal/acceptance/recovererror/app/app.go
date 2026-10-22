// Package app exercises RecoverError: the hook that turns a failed Datastar
// request into something the visitor can see.
//
// A script makes a Datastar request. An HTTP error status therefore reaches nothing
// the visitor is looking at. RecoverError answers on the SSE connection instead.
// That is how an application shows a toast rather than failing in silence.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

// RecoverError renders the failure into the page the visitor is on.
func (a *App) RecoverError(
	err error,
	sse datapages.SSE,
) error {
	kind := "unknown"
	var panicErr datapages.PanicError
	switch {
	case errors.As(err, &panicErr):
		kind = "panic"
	case errors.Is(err, datapages.ErrBadRequest):
		kind = "bad request"
	case errors.Is(err, datapages.ErrNotFound):
		kind = "not found"
	case errors.Is(err, errUnrecoverable):
		// Returning an error from RecoverError is how an application says it
		// could not show anything. The server logs it and leaves the response
		// as written, a 200 event stream here.
		return errors.New("cannot render a toast for this")
	case errors.Is(err, errRecoverPanics):
		panic("RecoverError panicked")
	}
	return sse.PatchElement(
		templ.Raw(`<div id="toast">` + kind + `</div>`),
	)
}

var (
	errUnrecoverable = errors.New("unrecoverable")
	errRecoverPanics = errors.New("RecoverError panics on this")
)

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<pre id="echo">index</pre>`), nil
}

// PageError500 is /server-error
//
// Its query parameter parses through PanicText, which makes the page panic
// while it renders the answer to another failure.
type PageError500 struct{ App *App }

func (PageError500) GET(
	_ *http.Request,
	_ datapages.Query[struct {
		Q PanicText `query:"se"`
	}],
) (body datapages.Component, err error) {
	return templ.Raw(`<pre id="echo">server error</pre>`), nil
}

// POSTBad is /bad
func (PageIndex) POSTBad(_ *http.Request) error {
	return datapages.ErrBadRequest
}

// POSTMissing is /missing
func (PageIndex) POSTMissing(_ *http.Request) error {
	return datapages.ErrNotFound
}

// POSTPlain is /plain
func (PageIndex) POSTPlain(_ *http.Request) error {
	return errors.New("plain failure")
}

// POSTUnrecoverable is /unrecoverable
func (PageIndex) POSTUnrecoverable(_ *http.Request) error {
	return errUnrecoverable
}

// POSTPanic is /panic
func (PageIndex) POSTPanic(_ *http.Request) error {
	panic("the action panicked")
}

// POSTRecoverPanic is /recover-panic
func (PageIndex) POSTRecoverPanic(_ *http.Request) error {
	return errRecoverPanics
}

// POSTConfigPanic is /config-panic
func (PageIndex) POSTConfigPanic(_ *http.Request, sse datapages.SSE) error {
	return sse.PatchElement(templ.Raw(`<div id="toast">ran</div>`))
}

func (PageIndex) StreamConfigPOSTConfigPanic(
	_ *http.Request,
) (datapages.StreamConfig, error) {
	panic("the action stream config panicked")
}

// PagePanic is /panic-page
type PagePanic struct{ App *App }

func (PagePanic) GET(_ *http.Request) (body datapages.Component, err error) {
	panic("the page panicked")
}

// PageRenderPanic is /render-panic
type PageRenderPanic struct{ App *App }

func (PageRenderPanic) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.ComponentFunc(func(context.Context, io.Writer) error {
		panic("the component panicked")
	}), nil
}

// PageBoom is /boom
type PageBoom struct{ App *App }

func (PageBoom) GET(_ *http.Request) (body datapages.Component, err error) {
	return nil, errors.New("the page could not be built")
}

// EventPinged is "pinged"
type EventPinged struct {
	Text string `json:"text"`
}

// PageStreamPanic is /stream-panic
type PageStreamPanic struct{ App *App }

func (PageStreamPanic) GET(_ *http.Request) (body datapages.Component, err error) {
	return templ.Raw(`<pre id="echo">stream panic</pre>`), nil
}

func (PageStreamPanic) StreamConfig(r *http.Request) (datapages.StreamConfig, error) {
	if r.URL.Query().Has("config-panic") {
		panic("the stream config panicked")
	}
	return datapages.StreamConfig{}, nil
}

func (PageStreamPanic) StreamOpen(_ *http.Request, _ datapages.StreamID) error {
	panic("the stream open hook panicked")
}

func (PageStreamPanic) OnPinged(_ EventPinged, _ datapages.SSE) error { return nil }

// PanicText parses a path or query value and panics on a value that starts with "x",
// the way a parser of the application fails on input it doesn't expect.
type PanicText string

func (t *PanicText) UnmarshalText(b []byte) error {
	if strings.HasPrefix(string(b), "x") {
		panic("the text parser panicked")
	}
	*t = PanicText(b)
	return nil
}

// PanicJSON is PanicText for a signal.
type PanicJSON string

func (j *PanicJSON) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if strings.HasPrefix(s, "x") {
		panic("the JSON parser panicked")
	}
	*j = PanicJSON(s)
	return nil
}

// ThingPath, ThingQuery and ThingSignals parse through PanicText and PanicJSON.
type (
	ThingPath struct {
		ID PanicText `path:"id"`
	}
	ThingQuery struct {
		Q PanicText `query:"q"`
	}
	ThingSignals struct {
		S PanicJSON `json:"s"`
	}
)

// PageThing is /thing/{id}
//
// Every handler of the page reads its inputs through PanicText and PanicJSON.
type PageThing struct{ App *App }

func (PageThing) GET(
	_ *http.Request,
	path datapages.Path[ThingPath],
	_ datapages.Query[ThingQuery],
	_ datapages.Signals[ThingSignals],
) (body datapages.Component, err error) {
	return templ.Raw(`<pre id="echo">` +
		templ.EscapeString(string(path.Values.ID)) + `</pre>`), nil
}

// POSTAct is /thing/{id}/act
//
// It takes an SSE stream, which opens once the inputs are read.
func (PageThing) POSTAct(
	_ *http.Request,
	_ datapages.Path[ThingPath],
	_ datapages.Query[ThingQuery],
	_ datapages.Signals[ThingSignals],
	sse datapages.SSE,
) error {
	return sse.PatchElement(templ.Raw(`<div id="toast">ran</div>`))
}

// POSTPlain is /thing/{id}/plain
func (PageThing) POSTPlain(
	_ *http.Request,
	_ datapages.Path[ThingPath],
	_ datapages.Query[ThingQuery],
) error {
	return nil
}

// GETFile is /thing/{id}/file
func (PageThing) GETFile(
	_ *http.Request,
	_ datapages.Path[ThingPath],
	_ datapages.Query[ThingQuery],
) (datapages.File, error) {
	return datapages.File{
		Type: "text/plain; charset=utf-8",
		Body: memBody{strings.NewReader("file")},
	}, nil
}

func (PageThing) StreamOpen(
	_ *http.Request,
	_ datapages.StreamID,
	_ datapages.Signals[ThingSignals],
) error {
	return nil
}

func (PageThing) OnPinged(_ EventPinged, _ datapages.SSE) error { return nil }

// POSTAppAct is /app-act
func (a *App) POSTAppAct(_ *http.Request, _ datapages.Query[ThingQuery]) error {
	return nil
}

// GETAppFile is /app-file
func (a *App) GETAppFile(
	_ *http.Request, _ datapages.Query[ThingQuery],
) (datapages.File, error) {
	return datapages.File{
		Type: "text/plain; charset=utf-8",
		Body: memBody{strings.NewReader("file")},
	}, nil
}

// PageError404 is /not-found
//
// Its query parameter parses through PanicText under a key of its own,
// which leaves PageError500 free to render the answer.
// With "gone" it returns ErrNotFound, which gets the plain 404 status.
type PageError404 struct{ App *App }

func (PageError404) GET(
	_ *http.Request,
	query datapages.Query[struct {
		Q PanicText `query:"nf"`
	}],
) (body datapages.Component, err error) {
	if query.Values.Q == "gone" {
		return nil, datapages.ErrNotFound
	}
	return templ.Raw(`<pre id="echo">not found</pre>`), nil
}

// PagePointer is /pointer/{id}
//
// Its path and query fields are pointers to a type that parses itself.
type PagePointer struct{ App *App }

func (PagePointer) GET(
	_ *http.Request,
	path datapages.Path[struct {
		ID *PanicText `path:"id"`
	}],
	query datapages.Query[struct {
		Q *PanicText `query:"q"`
	}],
) (body datapages.Component, err error) {
	q := "absent"
	if query.Values.Q != nil {
		q = string(*query.Values.Q)
	}
	return templ.Raw(`<pre id="echo">` +
		templ.EscapeString(string(*path.Values.ID)+" "+q) + `</pre>`), nil
}

// memBody serves content held in memory, which has nothing to close.
type memBody struct{ *strings.Reader }

func (memBody) Close() error { return nil }
