// Package app exercises the page cache: what a handler queues through the
// pageCache parameter, and how the generated server delivers it to the service
// worker for each kind of handler.
//
// Every handler echoes what it read into an element, so a test can assert over
// HTTP what the framework handed the handler.
package app

import (
	"net/http"
	"strconv"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

// IndexVersion is the version PageIndex stamps on its cached copy.
// A client reporting it holds the current copy and the handler queues nothing.
const IndexVersion = 7

func echo(id, s string) datapages.Component {
	return templ.Raw(`<pre id="` + id + `">` + s + `</pre>`)
}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(
	r *http.Request, pageCache datapages.PageCacheWriter,
) (body datapages.Component, err error) {
	held := pageCache.Version()
	if held != IndexVersion {
		pageCache.Set("/", echo("cached", "index offline"), IndexVersion)
	}
	return echo("held", strconv.FormatUint(held, 10)), nil
}

// POSTStream is /stream-write
//
// An action that opens an SSE stream delivers its writes over that stream.
func (PageIndex) POSTStream(
	_ *http.Request, sse datapages.SSE, pageCache datapages.PageCacheWriter,
) error {
	pageCache.Set("/", echo("cached", "written by an action"), 2)
	return sse.PatchElement(echo("out", "done"))
}

// POSTRedirect is /redirect-write
//
// An action that returns a redirect has no stream to write to.
// Its writes ride in the redirect response, ahead of the navigation.
func (PageIndex) POSTRedirect(
	_ *http.Request, pageCache datapages.PageCacheWriter,
) (redirect datapages.Redirect, err error) {
	pageCache.ClearAll()
	pageCache.Set("/", echo("cached", "written by a redirect"), 3)
	return datapages.Redirect{URL: "/"}, nil
}

// PageError404 is /not-found
//
// The page rendered inline for a URL no page claims. It writes the cache from there,
// which is the path that has no handler function of its own.
type PageError404 struct{ App *App }

func (PageError404) GET(
	r *http.Request, pageCache datapages.PageCacheWriter,
) (body datapages.Component, err error) {
	pageCache.Clear(r.URL.Path)
	return echo("missing", r.URL.Path), nil
}

// PageOffline is /offline
//
// Declaring it generates WithOffline, which hands this route to the worker.
type PageOffline struct{ App *App }

func (PageOffline) GET(_ *http.Request) (body datapages.Component, err error) {
	return echo("offline", "offline"), nil
}

// POSTAppPrecache is /app-precache
//
// An app-level action that returns neither a redirect nor a body. The framework
// opens a stream for it, which is the only way its writes can reach the worker.
func (*App) POSTAppPrecache(
	_ *http.Request, pageCache datapages.PageCacheWriter,
) error {
	pageCache.Set("/", echo("cached", "written by an app-level action"), 11)
	return nil
}

// POSTAppBody is /app-body
//
// An app-level action answering with a document. Its writes are baked into it.
func (*App) POSTAppBody(
	_ *http.Request, pageCache datapages.PageCacheWriter,
) (body datapages.Component, err error) {
	pageCache.Set("/", echo("cached", "written by an app-level body action"), 12)
	return echo("out", "done"), nil
}

// POSTStreamRedirect is /stream-redirect-write
//
// An action that holds a stream and redirects through it. The navigation is an
// event on that stream, so the writes have to be flushed onto it first.
func (PageIndex) POSTStreamRedirect(
	_ *http.Request, sse datapages.SSE, pageCache datapages.PageCacheWriter,
) (redirect datapages.Redirect, err error) {
	pageCache.Set("/", echo("cached", "written before a stream redirect"), 14)
	return datapages.Redirect{URL: "/"}, nil
}
