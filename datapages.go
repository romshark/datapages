package datapages

import (
	"context"

	"github.com/a-h/templ"
)

// SSE is the server-sent-event handle passed to action (POST/PUT/PATCH/DELETE)
// and event (OnXXX) handlers. It abstracts the underlying Datastar generator so
// application code never depends on the datastar package directly.
type SSE interface {
	// Context returns the context of the SSE stream.
	Context() context.Context
	// PatchElementTempl patches (morphs) the elements rendered by c into the DOM.
	PatchElementTempl(c templ.Component, opts ...PatchOption) error
	// ExecuteScript runs a script on the client.
	ExecuteScript(script string) error
	// MarshalAndPatchSignals updates client-side signals from v.
	MarshalAndPatchSignals(v any) error
	// Redirect navigates the client to url.
	Redirect(url string) error
}

// PatchConfig is the accumulated configuration of a [SSE.PatchElementTempl]
// call. The generated runtime translates it to the underlying Datastar options.
type PatchConfig struct {
	Selector   string
	SelectorID string
	ModeAppend bool
}

// PatchOption configures [SSE.PatchElementTempl] without exposing the datastar
// package to application code.
type PatchOption func(*PatchConfig)

// WithSelector targets the element(s) matching a CSS selector.
func WithSelector(selector string) PatchOption {
	return func(c *PatchConfig) { c.Selector = selector }
}

// WithSelectorID targets the element with the given id.
func WithSelectorID(id string) PatchOption {
	return func(c *PatchConfig) { c.SelectorID = id }
}

// WithModeAppend appends the patched elements instead of morphing.
func WithModeAppend() PatchOption {
	return func(c *PatchConfig) { c.ModeAppend = true }
}

// Service-worker protocol HTTP request headers.
const (
	// HeaderOfflineVersion carries the version the service worker holds for the
	// requested URL. It is surfaced to handlers through PageCacheWriter.Version.
	HeaderOfflineVersion = "X-Datapages-Offline-Version"

	// HeaderWorkerVersion carries the installed service worker's own version.
	// The server compares it against the worker version it ships to decide
	// whether to install, update or leave the worker untouched.
	HeaderWorkerVersion = "X-Datapages-Worker-Version"
)

// PageCacheWriter writes to the client's service-worker cache. It is passed
// to GET page methods and action methods as the pageCache parameter. Writes
// are deferred and applied atomically once the handler returns without error.
type PageCacheWriter interface {
	// Version returns the version at which the current request's URL is cached
	// in the service worker (0 if not cached).
	Version() uint64

	// Set caches body for url and stamps it with version, which Version reports
	// back on the next request. url must come from the generated href package. The
	// entry is served only while offline and may differ from the live page.
	Set(url string, body templ.Component, version uint64)

	// SetShim caches body for url like [Set], but marks it servable while online.
	// The service worker serves the entry at once, then fetches the live page and
	// morphs it in. Datapages adds the trigger for that fetch. body is a
	// placeholder rendering, usually the page chrome with skeletons in place of
	// slow parts. It is shown online too and must not state anything that is only
	// true offline.
	SetShim(url string, body templ.Component, version uint64)

	// Clear removes a single url from the cache.
	Clear(url string)

	// ClearAll wipes the entire cache.
	ClearAll()
}
