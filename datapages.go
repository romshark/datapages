package datapages

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

// Component is anything that renders itself, such as a templ.Component.
type Component interface {
	// Render renders the template to w.
	Render(ctx context.Context, w io.Writer) error
}

// Session is the authenticated session of the client, passed to handlers as the
// session parameter. It's read-only: return a [NewSession] to change it.
//
// Data carries whatever the application needs to keep for the duration of the
// session, use struct{} when it needs nothing:
//
//	func (p PageIndex) GET(
//		r *http.Request, session datapages.Session[struct{}],
//	) (body datapages.Component, err error)
//
// The zero value is the session of an unauthenticated client:
//
//	if session.IsGuest() {
//		return httperr.Forbidden // Unauthenticated client
//	}
type Session[Data any] struct {
	userID    string
	token     string
	issuedAt  time.Time
	expiresAt time.Time
	data      Data
}

// UserID identifies the authenticated user. It's empty for guest clients.
func (s Session[Data]) UserID() string { return s.userID }

// IsGuest reports whether the client is unauthenticated.
func (s Session[Data]) IsGuest() bool { return s.userID == "" }

// Token is the session token from the client's cookie. It's empty for guest clients.
func (s Session[Data]) Token() string { return s.token }

// IssuedAt is the time the session was created at. It's zero for guest clients.
func (s Session[Data]) IssuedAt() time.Time { return s.issuedAt }

// ExpiresAt is the time the session becomes invalid at. Datapages treats a client
// whose session has expired as unauthenticated and removes the session cookie.
// It's zero for guest clients and for sessions that never expire.
func (s Session[Data]) ExpiresAt() time.Time { return s.expiresAt }

// Data is the application-defined payload of the session.
func (s Session[Data]) Data() Data { return s.data }

// MakeSession assembles a session from its parts. It's called by generated code,
// applications return a [NewSession] instead.
func MakeSession[Data any](
	userID, token string, issuedAt, expiresAt time.Time, data Data,
) Session[Data] {
	return Session[Data]{
		userID:    userID,
		token:     token,
		issuedAt:  issuedAt,
		expiresAt: expiresAt,
		data:      data,
	}
}

// NewSession is returned by handlers as the newSession return value to sign a client in.
// Datapages generates the session token and stamps the issuance time,
// then hands the result back to handlers as a [Session].
//
//	func (p PageLogin) POSTSubmit(...) (
//		newSession datapages.NewSession[SessionData],
//		redirect datapages.Redirect,
//		err error,
//	) {
//		return datapages.NewSession[SessionData]{
//			UserID: user.ID,
//			Data:   SessionData{Name: user.Name},
//		}, datapages.Redirect{URL: href.PageIndex()}, nil
//	}
//
// A zero UserID is a no-op, no session is created.
type NewSession[Data any] struct {
	// UserID identifies the authenticated user.
	UserID string

	// ExpiresAt is the time the session becomes invalid at.
	// Leave it zero to let the session live until it's closed explicitly.
	ExpiresAt time.Time

	// Data is the application-defined payload of the session.
	Data Data
}

// Redirect is returned by handlers as the redirect return value to navigate the
// client to another URL:
//
//	func (p PageLogin) POSTSubmit(...) (
//		redirect datapages.Redirect, err error,
//	) {
//		return datapages.Redirect{URL: href.PageIndex()}, nil
//	}
//
// The zero value is a no-op, the client stays on the current page.
type Redirect struct {
	// URL is the target the client is navigated to.
	// An empty URL is a no-op, no redirect is performed.
	URL string

	// Status is the HTTP status code of the redirect response.
	// Zero, or any code that isn't a redirect status, means [net/http.StatusFound].
	//
	// Status is ignored for requests issued by a Datastar action
	// (those carrying the header "Datastar-Request: true"),
	// because they can't follow an HTTP redirect.
	// Those navigate client-side by assigning window.location instead.
	Status int
}

// SSE is the server-sent-event handle passed to action (POST/PUT/PATCH/DELETE)
// and event (OnXXX) handlers.
type SSE interface {
	// Context returns the context of the SSE stream.
	Context() context.Context

	// PatchElement patches the elements rendered by c into the DOM.
	// They are morphed by default, use [WithMode] to patch them differently.
	PatchElement(c Component, opts ...PatchOption) error

	// RemoveElement removes the elements matching the CSS selector from the DOM.
	RemoveElement(selector string) error

	// ExecuteScript runs a script on the client.
	ExecuteScript(script string) error

	// PatchSignals updates client-side signals from v, which is marshaled to JSON.
	// It overwrites the signals the client already has.
	// To send JSON as is, pass an [encoding/json.RawMessage]:
	//
	//	sse.PatchSignals(json.RawMessage(`{"count":42}`))
	PatchSignals(v any) error

	// PatchSignalsIfMissing works like [SSE.PatchSignals] but only sets the
	// signals the client doesn't have yet. The rest keep their values.
	PatchSignalsIfMissing(v any) error

	// Redirect navigates the client to url by assigning window.location.href,
	// which pushes a new browser history entry.
	// To replace the current entry instead, navigate with [SSE.ExecuteScript]:
	//
	//	sse.ExecuteScript(fmt.Sprintf("window.location.replace(%q)", url))
	Redirect(url string) error

	// Prefetch asks the browser to prefetch urls through the speculation rules API.
	// Browsers without support for it ignore the request.
	//
	// https://developer.mozilla.org/en-US/docs/Web/API/Speculation_Rules_API
	Prefetch(urls ...string) error
}

// PatchConfig is the accumulated configuration of a [SSE.PatchElement] call.
// The generated runtime translates it to the underlying Datastar options.
//
// Selector and SelectorID both name the patch target and are mutually exclusive.
// If both are set then SelectorID wins, no matter in which order the options were passed.
type PatchConfig struct {
	Selector   string
	SelectorID string
	Mode       PatchMode
}

// PatchMode determines how patched elements are applied to the DOM.
// Removal has no mode, use [SSE.RemoveElement] instead.
// Zero value is equivalent to [PatchModeOuter].
type PatchMode string

const (
	// PatchModeOuter (default) morphs the element into the existing element.
	PatchModeOuter PatchMode = "outer"

	// PatchModeInner replaces the inner HTML of the existing element.
	PatchModeInner PatchMode = "inner"

	// PatchModeReplace replaces the existing element with the new element.
	PatchModeReplace PatchMode = "replace"

	// PatchModePrepend prepends the element inside the existing element.
	PatchModePrepend PatchMode = "prepend"

	// PatchModeAppend appends the element inside the existing element.
	PatchModeAppend PatchMode = "append"

	// PatchModeBefore inserts the element before the existing element.
	PatchModeBefore PatchMode = "before"

	// PatchModeAfter inserts the element after the existing element.
	PatchModeAfter PatchMode = "after"
)

// PatchOption configures [SSE.PatchElement].
type PatchOption func(*PatchConfig)

// WithSelector targets the element(s) matching a CSS selector.
// Mutually exclusive with [WithSelectorID] (which wins if both are given).
func WithSelector(selector string) PatchOption {
	return func(c *PatchConfig) { c.Selector = selector }
}

// WithSelectorID targets the element with the given id.
// Mutually exclusive with [WithSelector] and wins if both are given.
func WithSelectorID(id string) PatchOption {
	return func(c *PatchConfig) { c.SelectorID = id }
}

// WithMode determines how the patched elements are applied to the DOM.
// Defaults to [PatchModeOuter], which morphs.
// mode must be one of the [PatchMode] constants, any other value is ignored:
//
//   - [PatchModeOuter]
//   - [PatchModeInner]
//   - [PatchModeReplace]
//   - [PatchModePrepend]
//   - [PatchModeAppend]
//   - [PatchModeBefore]
//   - [PatchModeAfter]
func WithMode(mode PatchMode) PatchOption {
	return func(c *PatchConfig) { c.Mode = mode }
}

// Sentinel errors for HTTP status codes. Return one from a handler to control
// the status code of the response, directly for a zero-alloc response or
// wrapped around the original error:
//
//	if !valid {
//		return datapages.ErrBadRequest
//	}
//	return fmt.Errorf("%w: %w", datapages.ErrBadRequest, errInvalidInput)
//
// The response body always uses the standard status text
// (for example "Bad Request" for 400), no matter what the error message says.
//
// Don't wrap multiple sentinels in one error. If you do, the first of
// ErrBadRequest, ErrForbidden, ErrNotFound wins.
//
// Any other error results in 500 Internal Server Error.
var (
	ErrBadRequest = errors.New(http.StatusText(http.StatusBadRequest)) // 400
	ErrForbidden  = errors.New(http.StatusText(http.StatusForbidden))  // 403
	ErrNotFound   = errors.New(http.StatusText(http.StatusNotFound))   // 404
)

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
	Set(url string, body Component, version uint64)

	// SetShim caches body for url like [PageCacheWriter.Set], but marks it
	// servable while online. The service worker serves the entry at once, then
	// fetches the live page and morphs it in. Datapages adds the trigger for that
	// fetch. body is a placeholder rendering, usually the page chrome with
	// skeletons in place of slow parts. It is shown online too and must not state
	// anything that is only true offline.
	SetShim(url string, body Component, version uint64)

	// Clear removes a single url from the cache.
	Clear(url string)

	// ClearAll wipes the entire cache.
	ClearAll()
}
