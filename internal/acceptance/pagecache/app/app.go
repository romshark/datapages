// Package app defines handlers for page cache acceptance tests. Each handler
// renders its inputs so tests can verify the HTTP response.
package app

import (
	"net/http"
	"strconv"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
)

type App struct{}

// IndexVersion is the version PageIndex uses for its cached response.
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
// An action that returns a redirect has no stream. Its response sends cache
// writes before navigation.
func (PageIndex) POSTRedirect(
	_ *http.Request, pageCache datapages.PageCacheWriter,
) (redirect datapages.Redirect, err error) {
	pageCache.ClearAll()
	pageCache.Set("/", echo("cached", "written by a redirect"), 3)
	return datapages.Redirect{URL: "/"}, nil
}

// PageError404 is /not-found
//
// This handler tests cache writes from the inline 404 response.
type PageError404 struct{ App *App }

func (PageError404) GET(
	r *http.Request, pageCache datapages.PageCacheWriter,
) (body datapages.Component, err error) {
	pageCache.Clear(r.URL.Path)
	return echo("missing", r.URL.Path), nil
}

// PageOffline is /offline
//
// Declaring it generates WithOffline with this route.
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
// An app-level action returning a document embeds its cache writes in the body.
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

// ListShimVersion is the version PageList uses for its shim.
const ListShimVersion = 21

// PageList is /list
//
// Caches a shim under a URL carrying a query. The worker keys entries on the full URL,
// and the shim's trigger has to request that URL with its query.
type PageList struct{ App *App }

func (PageList) GET(
	_ *http.Request,
	pageCache datapages.PageCacheWriter,
	query datapages.Query[struct {
		Page string `query:"page"`
	}],
) (body datapages.Component, err error) {
	pageCache.SetShim("/list/?page="+query.Values.Page,
		echo("shim", "list shim"), ListShimVersion)
	return echo("page", query.Values.Page), nil
}

// POSTBranch is /branch
//
// Picks between a body and a redirect at run time. The signature cannot say which,
// so both branches have to deliver what the handler queued.
func (PageIndex) POSTBranch(
	_ *http.Request,
	pageCache datapages.PageCacheWriter,
	query datapages.Query[struct {
		Redirect string `query:"redirect"`
	}],
) (body datapages.Component, redirect datapages.Redirect, err error) {
	pageCache.Set("/", echo("cached", "written by a branching action"), 15)
	if query.Values.Redirect != "" {
		return nil, datapages.Redirect{URL: "/"}, nil
	}
	return echo("out", "body"), datapages.Redirect{}, nil
}
