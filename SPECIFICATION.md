# Datapages Specification

## Source Package

The generator requires an application source package containing `App` and `type PageIndex struct`.

`App`, page, abstract page, and event types must not have type parameters. The generator rejects them with "type parameters are not supported". Other package types may have type parameters.

### App

`App` may define global HTML `<head>` tags:

```go
func (*App) Head(
	r *http.Request,
	session datapages.Session[Data], // Optional
) datapages.Head {
	return globalHeadTags()
}
```

Parameters are identified by type; names and order are unrestricted.

`RecoverError` receives errors from Datastar requests when defined, including Datapages sentinels, and may write feedback over SSE. It does not handle page loads: the browser would render its SSE frames as the document. For a failed page load whose response has not started, the server renders `PageError500` when defined or writes a plain HTTP error otherwise. If `RecoverError` fails, the server logs its error and leaves the response as written.

A panic in `GET`, an action, `StreamOpen`, or `OnXXX` follows the handler error path. When `RecoverError` handles it, the error is a `datapages.PanicError` containing the value and stack. The stack is logged.

A panic during page writing is logged. A plain page load retains its status and truncated body. On a Datastar request, a defined `RecoverError` appends any SSE frames it writes to the truncated body. A panicking stream is closed. `StreamClose` runs on the request goroutine after the last event handler of the stream. Its panics are recovered and logged.

Graceful shutdown waits for in-flight requests, open SSE streams, and `StreamClose` hooks. `ListenAndServe` waits at most `httpserve.DefaultShutdownTimeout` (10s) by default. `datapages.WithShutdownTimeout` changes this limit. If the limit expires, the server logs the shutdown error and returns. A direct call to `Shutdown` uses the deadline of its context.

```go
func (*App) RecoverError(
	err error,
	sse datapages.SSE,
) error {
	return sse.PatchElement(errorToast(err))
}
```

Parameters are identified by type; names and order are unrestricted.

### Pages

Pages use `type PageXXX struct { App *App }` and these methods:

- `GET`: handles `GET` requests.
- `POSTXXX`: handles `POST` action requests.
- `PUTXXX`: handles `PUT` action requests.
- `PATCHXXX`: handles `PATCH` action requests.
- `DELETEXXX`: handles `DELETE` action requests.
- `StreamOpen`: runs when the page SSE stream opens.
- `StreamClose`: runs when the page SSE stream closes.
- `OnXXX`: subscribes to events in the SSE listener.

An action, `OnXXX`, `StreamOpen`, or `StreamClose` may take `datapages.State[T]` for per-tab state; see [Parameter: `datapages.State[T]`](#parameter-datapagesstatet).

`XXX` denotes a name suffix.

A `Page*` declaration must use a struct type literal. An alias to a struct literal is recognized as a page; an alias to a named type or a non-struct type is rejected.

A page type must declare exactly one named field: `App *App`. Embedded types are permitted under [Abstract Page Types](#abstract-page-types).

URLs require a comment in [net/http ServeMux pattern syntax](https://pkg.go.dev/net/http#hdr-Patterns-ServeMux).

`PageIndex` is required for `/`.

`PageError500`, `PageError404` and `PageOffline` are optional reserved page
names. `PageError500` and `PageError404` render the 500 and 404 responses.
`PageOffline` is the fallback that the [service worker](#service-worker) serves
for an uncached URL while offline. Datapages supplies defaults when these pages
are absent.

Each declares its route by comment like any other page. `PageError500` and
`PageOffline` always render with a zero `Session`: the former runs after handling
has already failed, and the latter is precached once by the service worker and
served to every visitor, so neither may depend on who is signed in.

A page with an SSE stream serves `_$/` under its route. A page with both public and user-addressed events also serves `_$/anon/` for signed-out visitors. Page and action routes cannot conflict with these endpoints. A page whose route ends in a `{name...}` wildcard cannot have a stream.

Handler parameters and return values may appear in any order. Unsupported names or types are generator errors.

`GET` requires `r *http.Request` and a `datapages.Component` return value. Other parameters and return values are optional:

```go
func (PageIndex) GET(
	r *http.Request,
	session datapages.Session[Data], // Optional
	path datapages.Path[struct{...}], // Required only when path variables are used in the URL
	query datapages.Query[struct{...}], // Optional
	signals datapages.Signals[struct{...}], // Optional
	somethingHappened datapages.Dispatcher[EventSomethingHappened], // Optional
	somethingElseHappened datapages.Dispatcher[EventSomethingElseHappened], // Optional
) (
	body datapages.Component,
	head datapages.Head, // Optional
	redirect datapages.Redirect, // Optional
	newSession datapages.NewSession[Data], // Optional
	closeSession datapages.CloseSession, // Optional
	enableBackgroundStreaming datapages.EnableBackgroundStreaming, // Optional
	disableRefreshAfterHidden datapages.DisableRefreshAfterHidden, // Optional
	err error, // Optional
) {
	// ...
}
```

Global actions may be defined on `*App`:

```go
// POSTSignOut is /sign-out/{$}
func (*App) POSTSignOut(r *http.Request, session Session) (
	closeSession datapages.CloseSession,
	redirect datapages.Redirect,
	err error,
) {
	return true, datapages.Redirect{URL: "/login"}, nil
}
```

Page action handlers (`POSTXXX`, `PUTXXX`, `PATCHXXX`, `DELETEXXX`) require
`r *http.Request` and permit these optional parameters. A handler returning
only `error` may use this signature:

```go
// POSTActionName is <path>
func (PageIndex) POSTActionName(
	r *http.Request,
	sse datapages.SSE, // Optional
	session datapages.Session[Data], // Optional
	path datapages.Path[struct{...}], // Required only when path variables are used in the URL
	query datapages.Query[struct{...}], // Optional
	signals datapages.Signals[struct{...}], // Optional
	state datapages.State[T], // Optional
	stateID string, // Optional with state
	somethingHappened datapages.Dispatcher[EventSomethingHappened], // Optional
	somethingElseHappened datapages.Dispatcher[EventSomethingElseHappened], // Optional
) error {
	// ...
}
```

An action with neither `signals` nor `sse` does not require `Datastar-Request: true` and can receive HTML form submissions. It is guarded by [`net/http.CrossOriginProtection`](https://pkg.go.dev/net/http#CrossOriginProtection): requests reported as same-site or cross-site by `Sec-Fetch-Site`, or with an `Origin` that differs from `Host`, receive 403. Requests with neither header are allowed. Authenticated forms also fail the CSRF check when sessions and CSRF protection are enabled. All other actions require `Datastar-Request: true`; requests without it receive 406 Not Acceptable.

**Actions with `sse` cannot return `newSession` or `closeSession`.** The SSE stream sends headers before the handler returns. A `redirect` return value navigates through the stream, as `sse.Redirect` does.

An action without `sse` may redirect, return HTML, and change sessions:

```go
// POSTActionName is <path>
func (PageIndex) POSTActionName(
	r *http.Request,
	session datapages.Session[Data], // Optional
	path datapages.Path[struct{...}], // Required only when path variables are used in the URL
	query datapages.Query[struct{...}], // Optional
	signals datapages.Signals[struct{...}], // Optional
	state datapages.State[T], // Optional
	stateID string, // Optional with state
	somethingHappened datapages.Dispatcher[EventSomethingHappened], // Optional
	somethingElseHappened datapages.Dispatcher[EventSomethingElseHappened], // Optional
) (
	body datapages.Component, // Optional
	head datapages.Head, // Optional, requires body
	redirect datapages.Redirect, // Optional
	newSession datapages.NewSession[Data], // Optional
	closeSession datapages.CloseSession, // Optional
	err error,
) {
	// ...
}
```

`OnXXX` requires exactly one event parameter and `sse datapages.SSE`, and must return exactly one `error`. The event parameter is identified by type; `XXX` must match the event type's name after `Event`. Parameter order and the event parameter's name are unrestricted.

```go
func (PageIndex) OnSomethingHappened(
	event EventSomethingHappened,
	sse datapages.SSE,
	streamID datapages.StreamID, // Optional
	state datapages.State[T], // Optional
	stateID string, // Optional with state
	session datapages.Session[Data], // Optional
) error {
	// ...
}
```

`StreamOpen` runs after the SSE stream is established and before event handlers. It may return `error` or nothing. On error, setup stops, the stream closes, and `RecoverError` handles the error if defined. Otherwise the server uses its internal-error path. `StreamClose` does not run if `StreamOpen` returns an error or panics. `StreamOpen` must release acquired resources before returning an error and defer their release if it can panic.

`datapages.StreamID` identifies an SSE stream within a process. Its parameter name is unrestricted. It may correlate `StreamOpen` with `StreamClose` and must not be exposed to clients.

A stream hook must take `datapages.StreamID`, `datapages.State[T]`, or both.

```go
func (PageIndex) StreamOpen(
	r *http.Request,
	streamID datapages.StreamID, // Optional when state is declared
	state datapages.State[T], // Optional when streamID is declared
	stateID string, // Optional with state
	sse datapages.SSE, // Optional
	session datapages.Session[Data], // Optional
	signals datapages.Signals[struct{...}], // Optional
	somethingHappened datapages.Dispatcher[EventSomethingHappened], // Optional
	somethingElseHappened datapages.Dispatcher[EventSomethingElseHappened], // Optional
) error {
	// ...
}
```

`StreamClose` runs on the request goroutine after the stream's last event handler, provided `StreamOpen` succeeded or is absent. It may return `error` or nothing. Returned errors are logged.

```go
func (PageIndex) StreamClose(
	r *http.Request,
	streamID datapages.StreamID, // Optional when state is declared
	state datapages.State[T], // Optional when streamID is declared
	stateID string, // Optional with state
	session datapages.Session[Data], // Optional
	somethingHappened datapages.Dispatcher[EventSomethingHappened], // Optional
	somethingElseHappened datapages.Dispatcher[EventSomethingElseHappened], // Optional
) error {
	// ...
}
```

#### Abstract Page Types

A struct in the app package whose name does not start with `Page` and that declares `App *App` is an abstract page type. Pages may embed it to inherit its handlers.

#### Parameter: `datapages.State[T]`

```go
state datapages.State[T]
```

`State[T]` holds per-tab server-side state. Each open tab has an independent `*T` in `Values`. Writes persist across handlers of that tab. A per-instance mutex serializes those handlers, so access within a handler needs no additional synchronization.

An instance belongs to a tab, is not session-bound, and survives sign-out.

**`State.Values` must not outlive its handler.** The mutex does not protect goroutines started by a handler. Retaining the pointer in application state or a component rendered later can retain it past the tab's lifetime. Copy needed fields instead.

A page enables state by declaring an exported struct and using it as `T` in an action, `OnXXX`, `StreamOpen`, or `StreamClose`:

```go
type StateIndex struct {
	Filter string
	Count  int
}

func (PageIndex) StreamOpen(r *http.Request, state datapages.State[StateIndex]) error {
	return nil
}
```

**Declaration rules:**

- `T` must be an exported struct declared at the source package level.
- The parameter name is unrestricted. `T` must directly name the app package's struct. Pointers, struct literals, and types from other packages are rejected.
- All handlers on a page, including inherited handlers, must use the same `T`.
- State used by an abstract page binds every page that embeds it.
- Global `*App` actions may take `datapages.State[T]`. The caller must be bound to a page using the same `T`; otherwise the action receives `409 Conflict` with `Datapages-Retry: reconnect`. A global action callable from every page must be stateless.
- `GET` cannot take state because no instance exists at render time.
- A page using state gets an SSE stream even without stream hooks or `OnXXX`. Connect allocates the state slot; disconnect releases it.

**Parameter: `stateID string`.** A stateful handler may take `stateID string` alongside `datapages.State[T]` to dispatch events targeted at the tab via `datapages.SubjectStateID`.

`stateID` is the first 16 bytes of the SHA-256 hash of `Datapages-Instance`, encoded as unpadded base64url. It remains stable for the tab's lifetime. Knowing it permits event addressing but not access to the tab's state.

**Subject field: `datapages.SubjectStateID`.** An event may declare a field of this type with any name. On SSE connect, the server subscribes to `<base>.<state_id>`; only the matching tab receives the event.

- A `datapages.SubjectStateID` field must not carry a `signal:"..."` tag.
- It must be the event's only subject field.
- A page handling the event must use state.
- A page handling the event cannot also handle a user-addressed or signal-scoped event. It may handle public events.

**Lifecycle:**

1. `GET` generates an identifier, sets `Datapages-Instance` in the response, and embeds it in the HTML. No state is allocated yet.
2. The client includes the header in later Datastar actions and SSE connects.
3. On connect, the server checks for 22 base64url characters, allocates a zeroed `*T`, and registers the `id -> slot` mapping before `StreamOpen` or event handlers run.
4. Before a stateful action or `OnXXX`, the server validates the header, looks up and locks the slot, and passes its state to the handler. A missing slot yields `409 Conflict` with `Datapages-Retry: reconnect`.
5. On disconnect, the server removes the instance and releases its reference to the state. Reconnect with the same ID allocates a new `*T`; instances are never reused across streams.

By default, hiding a tab closes its stream and releases its state. Visibility reload creates a new instance with zeroed state. State needed after reload must be reconstructible from the URL or signals read by `StreamOpen`. Returning `enableBackgroundStreaming=true` keeps the stream and state alive and disables visibility reload. Returning only `disableRefreshAfterHidden=true` suppresses the reload without preserving state.

**Configuration.** The default instance limit is `datapages.DefaultMaxConcurrentInstances`. Set it with `datapages.WithStateConfig(datapages.StateConfig{MaxConcurrentInstances: n})`.

**Identifier.** `Datapages-Instance` is 16 random bytes from `crypto/rand`, encoded as 22 unpadded base64url characters. It is not signed; possession authorizes access to the tab's state.

The server accepts any well-formed ID, including one issued elsewhere. The stream creates state on the server it reaches, so `GET` and the stream may reach different servers. Validation bounds map keys to 22 base64url characters.

A process restart drops all instances. Reconnect creates zeroed state under the same ID. An action arriving before reconnect receives `409` and reloads once.

`MaxConcurrentInstances` caps live instances across state types, separately for each server. A client can hold at most one instance per open stream. Zero selects `DefaultMaxConcurrentInstances`; a negative value removes the cap. Size the cap by state memory use and limit per-client connections separately.

Connects exceeding the cap receive `503 Service Unavailable` with `Retry-After`. Stateful stream initialization uses `{retry:'error'}`. Datastar ignores `Retry-After` and retries after 1s, doubling to a 30s ceiling, for 10 attempts (about three minutes). After retries are exhausted, the next stateful action receives `409` and reloads. Existing instances continue to work. The application receives no cap notification. `Server.StateLiveInstances()` reports the live count; servers with Prometheus export it as `datapages_state_instances`. The configured cap is not exported. The gauge counts each server in the process that registered metrics.

**Multi-server routing.** State is process-local. A tab's stream and actions must reach the same backend. Hashing `Datapages-Instance` routes both to the same server; `GET` can reach any server because it has no ID yet. Session or affinity-cookie routing also works. Round-robin routing causes `409` responses and reloads.

**Rate limiting.** Datapages enforces only the global instance cap. Middleware added with `WithMiddleware` can enforce a per-session limit by matching the stream path; it wraps the whole router.

**Security.** An inline script captures the ID from the HTML response and removes itself. The ID is not stored in cookies, browser storage, or a persistent DOM node. Another tab cannot observe it. Access to another tab's state requires its 128-bit random ID.

The script runs at parse time. It needs `script-src 'unsafe-inline'`, or a nonce; see [Content Security Policy](#content-security-policy).

#### Parameter: `datapages.Signals[struct {...}]`

```go
signals datapages.Signals[struct {
	Foo string `json:"foo"`
	Bar int    `json:"bar"`
}]
```

`Signals` captures [Datastar signals](https://data-star.dev/guide/reactive_signals) in `Values`. The parameter name is unrestricted. Its type argument may be a named or anonymous struct; every field requires a `json` tag. Any JSON-serializable field type is supported, including nested structs, slices, and maps.

Nested structs map to nested Datastar signals using dot notation:

```go
signals datapages.Signals[struct {
	Form struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"form"`
}]
```

This maps to `$form.name` and `$form.email`, accessible as `signals.Values.Form.Name` and `signals.Values.Form.Email`.

Action bodies carrying signals are limited to 1 MiB by default. Oversized bodies receive 400. `datapages.WithBodySizeLimit` changes the server-wide limit.

#### Parameter: `datapages.Path[struct {...}]`

```go
path datapages.Path[struct {
	ID string `path:"id"`
}]
```

`Path` provides URL path parameters in `Values`. Each must appear in the URL comment. The parameter name is unrestricted; its type argument may be a named or anonymous struct.

Each field must be exported and tagged with its route variable: `path:"id"` binds to `{id}`.

Supported field types are:

- `string`
- `bool`
- `int`
- `int8`
- `int16`
- `int32`
- `int64`
- `uint`
- `uint8`
- `uint16`
- `uint32`
- `uint64`
- `float32`
- `float64`

Types implementing `encoding.TextUnmarshaler` are also supported. Parse failures return 400. Float fields reject `Inf`, `+Inf`, `-Inf`, and `NaN`.

Generated `href` and `action` builders encode `encoding.TextMarshaler` values with that interface. Other values, including named strings, use their basic kind because the builders cannot import app-defined types.

#### Parameter: `datapages.Query[struct {...}]`

```go
query datapages.Query[struct {
	Filter string `query:"f"`
	Limit  int    `query:"l"`
}]
```

`Query` provides URL query parameters in `Values`. The parameter name is unrestricted; its type argument may be a named or anonymous struct.

Each field must be exported and tagged with its query key: `query:"f"` reads `?f=...`.

The same field types as [`datapages.Path`](#parameter-datapagespathstruct-) are supported.

The `reflectsignal` tag binds a signal to a query parameter:

```go
signals datapages.Signals[struct {
	SelectedItem string `json:"selecteditem"`
}],
query datapages.Query[struct {
	SelectedItem string `query:"s" reflectsignal:"selecteditem"`
}]
```

Here, `s` and `selecteditem` are synchronized.

The browser updates only the reflected query keys. It removes a key when its signal becomes empty. Other query parameters remain, including fields declared without `reflectsignal`. The URL fragment remains.

Reflected integer, float, and bool fields seed JavaScript numbers or booleans unless they implement `encoding.TextMarshaler`. Text marshalers and all other field types seed strings.

`datapages gen` rejects two query fields with the same `reflectsignal` value. They would generate duplicate `data-signals` attributes, and the browser would ignore the second value.

The query field's seed must have a JSON kind that the signal field can decode. `datapages gen` rejects incompatible pairs. For example, it rejects a string query field reflected into a `bool` signal because the browser would submit `"true"` and the action would return 400 while decoding the signals. Fields that implement `json.Unmarshaler` and interface fields accept every JSON kind.

Signal `json` tags must match `[A-Za-z_][A-Za-z0-9_]*` and must not contain `__`. `json:"-"` is rejected.

Datastar treats `__` as an attribute modifier delimiter. A leading single underscore is allowed, but Datastar omits that signal from requests unless `filterSignals` includes it.

Datapages writes uppercase letters with a preceding hyphen in attribute names: `json:"newTitle"` becomes `data-signals:new-title` and `$newTitle`. Hyphens in signal `json` tags are rejected. Each step of a `reflectsignal` path must not start with an uppercase letter.

Periods are rejected in signal names. Nested structs define signal paths:

```go
signals datapages.Signals[struct {
	Foo struct {
		Bar struct {
			Bazz string `json:"bazz"`
		} `json:"bar"`
		Fuzz string `json:"fuzz"`
	} `json:"foo"`
}]
```

This declares `foo.bar.bazz` and `foo.fuzz`. A `json:"foo.bar"` tag would instead declare a single key that the client does not send.

A `reflectsignal` tag references the full path, such as `reflectsignal:"foo.fuzz"`.

Query tags may contain any characters, including quotes; their values are escaped when written into the page.

#### Parameter: `session datapages.Session[Data]`

```go
session datapages.Session[Data]
```

`Session` provides cookie-based authentication. Its parameter name is unrestricted.

`Session` is read-only and exposes `UserID()`, `IsGuest()`, `Token()`, `IssuedAt()`, `ExpiresAt()`, and `Data()`. Only a [`newSession`](#return-value-newsession-datapagesnewsessiondata) return value changes it. Use `struct{}` for `Data` when no application payload is needed.

See [datapages.go](datapages.go) for method definitions.

Expired sessions are unauthenticated and their cookies are removed. A zero `ExpiresAt()` never expires; its cookie lasts until the browser closes.

An action without a session parameter checks CSRF against the cookie without reading the session store; a closed or expired session cookie passes this check. An action with a session parameter reads the store and rejects such sessions.

Expired sessions are removed on read. Datapages does not call the session manager's `DeleteExpired`; applications must schedule cleanup for abandoned sessions.

All handlers must use the same `Data` type because the server has one session manager. A type alias is permitted:

```go
type SessionData struct {
	Name string
}

type Session = datapages.Session[SessionData]
```

#### Parameter: `sse datapages.SSE`

```go
sse datapages.SSE
```

`SSE` is allowed on page action methods, `OnXXX`, `StreamOpen`, and `App.RecoverError`. Global `*App` actions and `StreamClose` cannot take it.

`datapages.SSE` exposes Datastar operations without a Datastar dependency in handler signatures.

It provides `Context`, `PatchElement`, `PatchElementAt`, `RemoveElement`, `ExecuteScript`, `PatchSignals`, `PatchSignalsIfMissing`, `Redirect` and `Prefetch`, alongside the `PatchMode` constants.

`PatchElement(c)` morphs each rendered element into the element with its ID. `PatchElementAt(c, selector, mode)` specifies a target and patch mode:

```go
return sse.PatchElementAt(toast(msg), "#toaster", datapages.PatchModeAppend)
```

`PatchElementAt` and `RemoveElement` reject selectors containing `\r` or `\n` with `datapages.ErrSelectorLineBreak`.

See [datapages.go](datapages.go) for method definitions.

#### Parameter: `pageCache datapages.PageCacheWriter`

```go
pageCache datapages.PageCacheWriter
```

This parameter is allowed on `GET` page methods and on `POSTXXX`, `PUTXXX`, `PATCHXXX`, and `DELETEXXX` action methods, including app-level actions. It queues writes to the client's [service worker](https://developer.mozilla.org/en-US/docs/Web/API/Service_Worker_API) cache. A cached body is a component supplied by the handler and need not match the live body. Datapages registers the service worker.

The interface (from `github.com/romshark/datapages`):

```go
// PageCacheWriter writes to the client's service worker cache. It is passed
// to GET page methods and action methods as the pageCache parameter. Writes
// are deferred and applied atomically once the handler returns without error.
type PageCacheWriter interface {
	// Version returns the cached version of the current request's URL.
	// It returns 0 when the URL is not cached.
	Version() uint64

	// Set caches body for url with version. Version reports that value on the
	// next request for url. url must come from the generated href package.
	// The worker serves this entry only while offline.
	Set(url string, body Component, version uint64)

	// SetShim caches body for url like [Set], but permits the worker to serve it
	// while online. The worker then fetches the live page and replaces the shim's
	// head and body through Datastar. Datapages adds the fetch trigger.
	// body must not state anything that is only true offline.
	SetShim(url string, body Component, version uint64)

	// Clear removes url from the cache.
	Clear(url string)

	// ClearAll removes every page cache entry.
	ClearAll()
}
```

Writes are deferred until the handler returns without error. The worker then applies all `Set`, `Clear` and `ClearAll` calls from that request together, with `ClearAll` first. An error discards the queued writes.

Each cached URL stores a version chosen by the application.
`Version()` reports the version the client holds for the current request's URL
(0 if none); compare it against the resource's server-side version to decide whether
to re-`Set` it.

The handler signature selects delivery in this order. The same rules apply to a
page method and an action declared on `App`:

- `GET`: embeds the writes in the page HTML for the worker to apply on load.
- Action with `sse`: sends the writes over that stream.
- Action with a redirect: sends the writes in the `text/javascript` response. Navigation waits up to 500ms for the worker to apply them. This rule also applies when the action can return a body.
- Action with only a body: embeds the writes in the rendered document.
- Action with neither: sends the writes over an SSE stream opened by Datapages. Only a Datastar request can read this response.

A page can lazily cache itself on visit, versioned by its own data so it
refreshes whenever that data changes:

```go
// PageItem is /item/{id}
func (p PageItem) GET(
	r *http.Request,
	pageCache datapages.PageCacheWriter,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
) (body datapages.Component, err error) {
	item := p.App.item(path.Values.ID)
	if pageCache.Version() < item.Revision {
		// The cached version predates the current item revision.
		pageCache.Set(href.PageItem(path.Values.ID), itemOffline(item), item.Revision)
	}
	return itemView(item), nil
}
```

An action handler can `Set` URLs other than the one being requested. This caches pages the user has not opened. `Version()` only refers to the current request's URL, so such an action has no per-URL gate and every `Set` it makes is written unconditionally. The version passed is stored on the entry and reported back the next time that URL is requested, so a later visit can skip re-caching it.

```go
// POSTPrecache is /precache
//
// Caches every ticket page the signed-in user owns.
func (a *App) POSTPrecache(
	r *http.Request,
	session datapages.Session[Data],
	pageCache datapages.PageCacheWriter,
) error {
	for _, t := range a.userTickets(r.Context(), session.UserID()) {
		pageCache.Set(href.PageTicket(t.Slug), ticketOffline(t), t.Revision)
	}
	return nil
}
```

See [Service Worker](#service-worker) for how these entries are stored and served.

#### Parameter: `datapages.Dispatcher[EventXXX]`

```go
xxx datapages.Dispatcher[EventXXX]
```

`Dispatcher[EventXXX]` publishes events handled by `OnXXX`. The parameter name is unrestricted. `EventXXX` must be an event type declared in the application package or a reachable imported package. Shared events use the same type; see [Events declared outside the application package](#events-declared-outside-the-application-package).

```go
type Dispatcher[Event any] interface {
	Dispatch(event Event) error
	DispatchCtx(ctx context.Context, event Event) error
}
```

`Dispatch` uses the request context in actions and the request context without cancellation in stream hooks. `DispatchCtx` uses its explicit context, including for publication after a handler returns or with a separate deadline.

Event fields require `json` tags. The event type comment must have the form `// EventXXX is "xxx"`, where `xxx` is the NATS subject prefix:

```go
// EventExample is "example"
type EventExample struct {
	Information string `json:"info"`
}
```

##### Events declared outside the application package

An event type may be declared in a directly or transitively imported package:

```go
// package events, imported by both app packages
// EventAnnouncement is "announcement"
type EventAnnouncement struct {
	Text string `json:"text"`
}

// app/admin
func (PageIndex) OnAnnouncement(
	event events.EventAnnouncement, sse datapages.SSE,
) error

func (PageIndex) POSTAnnounce(
	r *http.Request, announcement datapages.Dispatcher[events.EventAnnouncement],
) error
```

Subject, payload, and subject fields are read from the declaring package.

Applications using the same event type share its subject. Separate event types with the same subject are rejected.

Restrictions:

- The type must be reachable through application-package imports.
- An application cannot use two event types with the same name, regardless of declaring package.

A type alias such as `type EventX = other.EventX` refers to the original event's subject, payload, and declaring package. It may occur anywhere along the import path.

Applications sharing a user-addressed event must use the same user ID namespace.

Events may declare subject fields for targeted NATS subjects:

| type | segment |
| ---- | ------- |
| `datapages.Subject` | a segment value |
| `datapages.SubjectUser` | the ID of the user the event is addressed to |

Subject fields must be exported and precede payload fields. Names are unrestricted; types determine their role.

Each dispatch publishes to one subject. Subject field values follow the base subject in field order, separated by dots. For base `notify` and values `u1`, `r1`, and `mobile`, the subject is `notify.u1.r1.mobile`.

A subject field value may contain any byte. `.`, `*`, `>`, `%`, and whitespace are escaped into one literal segment. Escaping `%` distinguishes a literal value such as `a%2Eb` from an encoded `a.b`. Empty values cause dispatch to fail without publishing.

Publish and subscription use the same escaping. Values that need none remain unchanged; `a.b` becomes `a%2Eb` in the broker subject.

Addressing multiple values requires one dispatch per value. Each dispatch can fail independently, and each recipient receives only its addressed payload.

`datapages.SubjectUser` requires authentication and routes to the named user. An application dispatching such an event must define a Session type.

User IDs are escaped like other subject values and may be email addresses. Their encoded length is bounded; see [`datapages.ValidateUserID`](#validating-a-user-id).

##### Signal-scoped subject fields

A non-user subject field may use `signal:"<name>"` to bind its subscription value to a Datastar signal. On SSE connect, the server reads the signal and builds the subscription subject.

The tag must name a signal declared by the page, including periods in a nested path (for example, `signal:"form.calc_id"`).

```go
// EventCalcUpdated is "calc.updated"
type EventCalcUpdated struct {
	Instance datapages.Subject `json:"instance" signal:"instance_id"`

	Result float64 `json:"result"`
}
```

Here, SSE connect subscribes to `calc.updated.<instance_id>` after escaping the signal. An empty value receives 400. A `*` is escaped as one literal segment.

Signal-scoped events may coexist with user-addressed and public events on a page. Signal-scoped fields may coexist with other subject fields:

```go
// EventRoomUpdate is "room.update"
type EventRoomUpdate struct {
	Recipient datapages.SubjectUser `json:"recipient"`
	Room      datapages.Subject     `json:"chat_room"`
	Calc      datapages.Subject     `signal:"calc_id"`

	Data string `json:"data"`
}
```

**Restrictions:**

- A user-addressed subject field must not have a `signal:"..."` tag.
- No two subject fields may share the same `signal:"..."` tag value.
- `signal` tag values must be period-separated signal names. Each step follows the `Signals` `json` tag rule.
- Two event types cannot claim the same subject anywhere in a module. `datapages gen` and `datapages lint` report conflicts across applications. To share a subject, use one event declaration; see [Events declared outside the application package](#events-declared-outside-the-application-package).
- An event with subject fields reserves every subject below its base. `"notify"` with one subject field conflicts with `"notify.user"`.

Subject fields after payload fields are invalid. A field named like a subject field must have a subject-field type. Types declared from subject-field types, such as `type UserID datapages.SubjectUser`, are rejected.

Each dispatcher publishes one event type. A handler may declare one dispatcher per event type, but cannot declare two for the same type. Events publish in dispatch order. Failure does not undo earlier publishes or prevent later ones.

##### Event delivery

Delivery is at most once, without replay, to streams subscribed at publish time.

A stream misses events when:

- its tab is hidden and its stream is closed by default; see [`enableBackgroundStreaming`](#get-return-value-enablebackgroundstreaming-datapagesenablebackgroundstreaming);
- its subscription buffer is full. `messaging.DefaultBrokerChanBuffer` is 16 messages; built-in brokers may configure another size. The broker drops overflow instead of blocking the publisher; `OnXXX` consumes events serially.

Misses are not reported to the page. The next render must include full state; by default it occurs when the tab becomes visible again. See [`disableRefreshAfterHidden`](#get-return-value-disablerefreshafterhidden-datapagesdisablerefreshafterhidden).

Prometheus-enabled applications export drops as `datapages_event_broker_deliveries_dropped_total`.

#### Return Value: `body datapages.Component`

The [Templ](https://templ.guide/) component for page contents.

#### Return Value: `head datapages.Head`

The [Templ](https://templ.guide/) component for the page's `<head>`. `datapages.Head` is a distinct name for `datapages.Component`. Return values are identified by type; names are unrestricted.

An action returning `head` must also return `body`.

#### Return Value: `redirect datapages.Redirect`

```go
redirect datapages.Redirect
```

Redirects to `redirect.URL` with `redirect.Status`. The zero value is a no-op.

```go
return datapages.Redirect{URL: href.PageIndex()}, nil
```

`Status` defaults to `302 Found`; non-redirect codes are replaced by 302. Datastar action requests (`Datastar-Request: true`) navigate by assigning `window.location` and ignore `Status`.

See [datapages.go](datapages.go) for field definitions.

#### Return Value: `newSession datapages.NewSession[Data]`

```go
newSession datapages.NewSession[Data]
```

Signs in a client when `UserID` is nonempty; otherwise it is a no-op. Datapages generates the token and issuance time. The handler supplies `UserID`, optional `ExpiresAt`, and `Data`. The session the request arrived with is closed first, which ends its streams. See [datapages.go](datapages.go).

`ExpiresAt` becomes the `Max-Age` and `Expires` of the session cookie; a zero `ExpiresAt` writes a cookie the browser drops when it closes. The session record stays in the store either way.

#### Validating a User ID

```go
func ValidateUserID(userID string) error
```

A user ID may contain any byte; invalid subject bytes are escaped. `datapages.ValidateUserID` rejects:

- `datapages.ErrUserIDEmpty`: empty ID.
- `datapages.ErrUserIDTooLong`: escaped ID exceeds `datapages.MaxUserIDEncodedLen`.

`newSession` applies the same check and rejects invalid IDs.

#### Return Value: `closeSession datapages.CloseSession`

```go
closeSession datapages.CloseSession
```

If `true`, closes the session and removes its cookie. Otherwise it is a no-op.

#### Return Value `error` or `err error`

Ordinary errors are logged and produce 500 unless `RecoverError` handles them. Sentinels select the status of plain HTTP error responses.

To specify an HTTP status, return a datapages sentinel:

```go
func (p PageIndex) POSTInput(...) error {
	if !valid {
		return datapages.ErrBadRequest // 400
	}
	if !allowed {
		// 403, preserves original
		return fmt.Errorf("%w: %w", datapages.ErrForbidden, errOriginal)
	}
	if !found {
		return datapages.ErrNotFound // 404
	}
	return nil
}
```

Sentinels:
- `datapages.ErrBadRequest` - 400
- `datapages.ErrForbidden` - 403
- `datapages.ErrNotFound` - 404
- `datapages.ErrConflict` - 409

Do not wrap multiple sentinels into one error. If multiple occur, precedence is `ErrBadRequest`, `ErrForbidden`, `ErrNotFound`, then `ErrConflict`.

Sentinels may be returned directly or wrapped. `RecoverError` handles errors from Datastar requests when defined. Other requests use `PageError500` with status 500 if defined and the response has not started. When neither handler applies and the response has not started, the server writes the corresponding status and standard status text. If `RecoverError` fails, the server logs its error and leaves the response as written.

#### `GET` Return Value: `enableBackgroundStreaming datapages.EnableBackgroundStreaming`

Valid only on `GET`.

```go
enableBackgroundStreaming datapages.EnableBackgroundStreaming
```

If `true`, keeps the SSE stream open while the tab is hidden. This permits `OnXXX` updates and increases client resource use. Returning it from the `GET` of a page without a stream is an error.

Equivalent to Datastar's [`openWhenHidden`](https://data-star.dev/reference/actions).

Closed streams lose events; see [Event delivery](#event-delivery).

`true` also disables refresh after the tab becomes visible. Return `disableRefreshAfterHidden=false` explicitly to retain that refresh.

#### `GET` Return Value: `disableRefreshAfterHidden datapages.DisableRefreshAfterHidden`

Valid only on `GET`.

```go
disableRefreshAfterHidden datapages.DisableRefreshAfterHidden
```

Datapages refreshes a page that has a stream when its tab becomes visible again, which renders the events the closed stream missed. Returning `true` disables that refresh and may leave the page stale; see [Event delivery](#event-delivery). A page without a stream never refreshes and returning this value from its `GET` is an error.

Refresh uses the [`visibilitychange`](https://developer.mozilla.org/en-US/docs/Web/API/Document/visibilitychange_event) event.

### Content Security Policy

Datapages writes inline scripts. The CSRF script goes into the head of a page with a session. A stateful page gets the instance ID script. Datastar compiles every `data-*` expression at run time.

Without `WithCSPNonce` a policy must allow `script-src 'unsafe-inline' 'unsafe-eval'`.

`WithCSPNonce` takes a function reporting the nonce of a request:

```go
datapages.WithCSPNonce(func(r *http.Request) string { return nonceOf(r) })
```

Datapages may call the function several times while writing one response. It must return the same value for every call with the same request. Application middleware should mint the nonce once, store it in the request context and write the same value into its `Content-Security-Policy` header. Datapages reads it back through the function and writes it on the `html` element as `data-nonce` and on every script Datapages writes as `nonce`. An empty return writes the page without nonces.

`data-nonce` turns on Datastar's CSP mode. Datastar compiles an expression by appending a script element with that nonce instead of calling `Function`, which removes the need for `'unsafe-eval'`. It requires Datastar 1.0.3 or later. An older bundle throws `Datastar CSP requires a nonempty html data-nonce.` or compiles with `Function` regardless.

A nonce in the policy makes the browser ignore `'unsafe-inline'` for that directive. The browser runs inline scripts that carry the nonce and rejects injected inline scripts without it.

The nonce must differ per response and must not be guessable. Do not cache a response that contains a nonce: replay would reuse it.

Offline support applies the nonce to scripts written by the server. The nonce cannot reach a page served from the service worker's cache; see [Service Worker](#service-worker).

## Dev Mode

Datapages dev mode is enabled when `DATAPAGES_DEV_MODE` or `TEMPL_DEV_MODE` is nonempty.

templ reads only `TEMPL_DEV_MODE`, during package initialization. Set it before starting the process to enable templ hot reload and Datapages dev mode. `DATAPAGES_DEV_MODE` enables only Datapages dev mode.

`datapages watch` uses templier, which sets `TEMPL_DEV_MODE` before it starts the application.

Dev mode reads static assets from the source tree and sets `Cache-Control: no-store` on asset responses, ignoring `datapages.WithAssetsCache`. The server logs a startup warning. A production process inheriting either variable may lack the source directory.

Outside dev mode, Datapages adds no `Cache-Control` or `ETag` header unless `datapages.WithAssetsCache` is set.

Files passed to `datapages.WithAssets` come from `embed.FS`, which reports a zero modification time. `http.ServeContent` therefore adds no `Last-Modified` header. The response gives the client no validator to reuse on a later request.

`datapages.WithAssetsCache` adds `Cache-Control` and an `ETag` computed from the file contents. A matching `If-None-Match` request receives 304 with no body. `Disabled` suppresses both headers; `DisableETag` suppresses the `ETag`.

Generated asset URLs do not contain a content hash. With a positive `MaxAge`, browsers may reuse stale content until it expires unless the file name changes with the file contents.

## Linting

`datapages lint` reports application model errors without generating code. It applies the same rules as `datapages gen`.

It also checks `.templ` files:

- **Hardcoded href**: an `<a>` href such as `href="/path"`, `href={ "/path" }`, or a constant resolving to a disallowed URL.
- **Unverifiable href expression**: an `<a>` href calling a function outside the `href` package, such as `templ.SafeURL(...)` or `fmt.Sprintf(...)`.
- **`href.External` with internal URL**: for example, `href.External("/login")`.
- **Hardcoded action URL**: for example, `@post('/foo/bar')` in a Datastar action context.
- **Unverifiable action expression**: an expression in a Datastar action context other than a plain `action.XXX()` call.
- **Action call with a prefix or suffix**: an `action.XXX()` call concatenated with a string. This has a separate diagnostic naming the concatenated side. `action.WithBefore(expr)` and `action.WithAfter(expr)` place expressions inside the generated action string.
- **Form action attribute**: any `<form action=...>` attribute.
- **Action context**: an `action.XXX()` call outside a Datastar action context.
- **Href context**: an `href.XXX()` call in a Datastar action context.
- **Action on wrong page**: a page action in another page's template. App-level actions are allowed on every page.

Datastar action contexts are:

- `data-on:<event>` for any DOM event;
- `data-on-intersect`, `data-on-interval`, and `data-on-signal-patch`;
- `data-init`.

Plugin and `data-init` attributes may have Datastar modifiers, such as `data-on-intersect.once` and `data-on-interval__duration.500ms`.

### Allowed href values

Allowed without the `href` package:

- Fragment-only: `#section`, `#`
- Protocol-relative: `//cdn.example.com`
- Absolute with any scheme except `javascript:`: for example, `http://...`, `https://...`, `mailto:...`, `tel:...`, `ftp://...`, `ftps://...`
- Constants, backtick literals, and double-quoted literals resolving to the above

Templ preserves `http`, `https`, `mailto`, `tel`, `ftp`, and `ftps` schemes in expression hrefs. Other schemes pass lint but render as `about:invalid#TemplFailedSanitizationURL`. Literal attributes bypass this sanitizer.

Disallowed:

- Root-relative paths: `/login`, `/static/style.css`
- Relative paths: `relative`, `./x`, `../x`
- Query-only: `?tab=settings`
- Empty string: `""`
- `javascript:` URLs

### Expression href validation

Expression hrefs (`href={ expr }`) are parsed as Go AST:

1. Any call into the generated `href` package is allowed. A literal or constant first argument to `href.External` is checked against the disallowed values above.
2. Other function calls are rejected.
3. String literals and constants are checked against the rules above.
4. Bare identifiers resolve through local constants; qualified identifiers resolve through exported constants in imported packages. Variables are rejected because their values cannot be determined statically.

### Suppressing Lint Errors

`//datapages:nolint` suppresses lint errors on the next element in a templ file. A trailing explanation is optional:

```templ
//datapages:nolint
<a href="/legacy-path">Legacy</a>

//datapages:nolint // migrating to href package in #1234
<a href="/another-legacy">Another</a>
```

The directive applies to the next non-whitespace sibling element. It suppresses attribute-level lint errors, but not cross-page action ownership errors.

## Technical Limitations

### Plain Forms and CSRF

With sessions and CSRF protection enabled, authenticated plain form submissions fail the CSRF check. Guest forms can reach actions that declare neither `signals` nor `sse`. CSRF tokens are injected for Datastar `fetch` requests with `Datastar-Request: true`; authenticated forms must use Datastar actions.

### Absolute URLs in Href Linting

The href linter treats absolute URLs as external, including URLs on the application's own domain. Use `href.PageXxx()` for internal links.

### Build-Constrained Application Files

The app package cannot contain build-constrained files. Pages, actions, and events are read for the host platform, so platform-specific declarations may disappear without an error on other platforms. `datapages.NewServer` calls are read from all files except those under `//go:build ignore` and may occur in platform-specific commands.

## Service Worker

The service worker backs the [`pageCache`](#parameter-pagecache-datapagespagecachewriter) parameter. It runs only in a secure context (HTTPS or localhost); otherwise the offline API does nothing.

The worker scope covers the whole origin. Its script response sets `Service-Worker-Allowed: /`, regardless of the script URL.

The offline middleware writes the registration and connectivity scripts into every HTML response. A response that already carries a `Content-Encoding` passes through unchanged, since an encoded body cannot be edited as bytes. Register a compressing middleware before `WithOffline`: middleware runs in the order it is given, so the compressor then compresses the rewritten page.

Offline support writes inline scripts. The connectivity and worker registration scripts go into every HTML response. Queued cache writes go into a `GET` response or an action body. Each script uses the nonce from [`WithCSPNonce`](#content-security-policy). `WithOffline` reads the nonce when each request arrives, independent of option order. `offline.Config.CSPNonce` takes precedence.

A cached page is rendered once and replayed. It cannot contain a valid per-response nonce. Its hydration trigger and connectivity script are written without one. The service worker provides no policy header, and the browser does not require a nonce. A `Content-Security-Policy` in a `meta` element would block those scripts.

The `X-Datapages-Worker-Version` request header controls installation and updates. The installed worker sets its `uint64` version on every request. This version is independent of the Datapages release and the per-URL versions passed to `Set`. The server compares the header with its current worker version:

- Header absent: the server adds registration to the current HTML response.
- Header lower than the server version: the server adds registration for the current worker script.
- Header equal: the response omits registration.

The worker holds one cache of offline bodies keyed by URL. Each entry stores the rendered HTML and its version.

Writes reach the worker in one of three ways, depending on how the handler responds:

- `GET`: the queued entries are embedded in the page and an inline script passes them to the worker after load.
- Action opening an SSE stream: they are sent over that stream.
- Action returning a redirect: the `text/javascript` response sends the writes. Navigation waits up to 500ms for the worker to apply them before loading the destination.

The worker applies a request's `Set`, `Clear` and `ClearAll` calls together, once the handler returns without error. `Set` writes or overwrites one entry, `Clear` deletes one, `ClearAll` empties the cache.

On every navigation the worker sets the `X-Datapages-Offline-Version` request header to the version it holds for the requested URL, or omits it when the URL is not cached. The server reads it back through `Version()` (which returns 0 when the header is absent).

Both request headers change the response body. A response that depends on one lists it in `Vary`. A page whose handler takes `pageCache` sets `Vary: X-Datapages-Offline-Version`. The offline middleware sets `Vary: X-Datapages-Worker-Version` on HTML responses. A shared cache can then separate responses for different clients.

Serving a navigation works as follows:

- The URL holds a `SetShim` entry: the worker returns it immediately and fetches the live page in parallel. The Datapages trigger requests the URL with `X-Datapages-Shim-Hydrate`. The worker answers with `<head>` and `<body>` Datastar patches from the live response. The head arrives first because it contains the signed-in visitor's CSRF script. If worker termination removes the prefetched response, the worker fetches the page again. A failed offline fetch leaves the shim visible, so the shim must not make an offline-only claim.
- Online, no `SetShim` entry: the worker passes the request to the network and returns the live response. A `Set` entry is never served while online.
- Offline and the URL is cached: the worker returns the stored offline body.
- Offline and the URL is not cached: the worker returns `PageOffline`. The worker caches this page during installation. Its route comes from the generated `WithOffline` option. Datapages uses a minimal default when the application does not define `PageOffline` or installation could not cache it.

Cached pages also require their assets. The worker caches same-origin requests on first load except Datastar actions, page hydration and event streams, which always use the network. For cross-origin requests, it caches only destinations enabled by the application. The defaults are stylesheets, scripts, fonts and images. `Config.ExcludePaths` excludes same-origin path prefixes. Files listed in `Config.Assets` are cached during worker installation. Any other asset must load once while online before it is available offline.

For a cached same-origin asset, the worker returns the cached response and then updates it from the network. A later request receives the update. The worker does not update opaque cross-origin responses because it cannot compare them.
