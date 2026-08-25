---
name: datapages
description: >-
  Write Datapages application source packages and use the Datapages CLI.
  Activate when the user wants to build a web app with Datapages,
  when you see a datapages.yaml file, or when the project imports the datapages module.
---

# Writing a Datapages Application

You write Go application logic and templates. Datapages generates the server.
Follow these steps in order. Do not skip steps.

For the full specification of all parameters, return types, supported field types, and
configuration options, see [SPECIFICATION.md](../../SPECIFICATION.md).

Datapages apps use two other technologies. This skill does not teach them.
Learn them separately.

- **Templ** (`github.com/a-h/templ`) - Go HTML templating.
  Handlers return `datapages.Component`. You write `.templ` files that compile to Go
  via `templ generate`. Datapages does **not** run this automatically — you must
  run `templ generate` yourself after creating or modifying `.templ` files.
  Docs: https://templ.guide/developer-tools/llm/
- **Datastar** (`github.com/starfederation/datastar-go/datastar`) - Frontend
  reactivity via HTML attributes and SSE. Actions go into `data-on:<event>`
  attributes (`data-on:click`, `data-on:submit`). The hyphen form is reserved for
  plugins (`data-on-intersect`, `data-on-interval`, `data-on-signal-patch`).
  Docs: https://data-star.dev
  See also: [datastar/SKILL.md](../datastar/SKILL.md)

## Architecture

- **Hypermedia-First (MPA):** This is a multi-page application architecture. The backend drives the UI by sending HTML fragments, signal updates, and real-time events over SSE. There is no separate REST API layer - all interactions happen through Datastar SSE streams managed by Datapages.
- **Morphing & Patching:** Datastar uses morphing to update the DOM - it compares the incoming HTML fragment with the existing DOM and applies minimal changes, preserving focus, scroll position, and CSS transitions. Prefer HTML fragment patches (morphs) over signal updates because morphs carry both structure and data, keeping the server as the single source of truth. Use signal updates only for lightweight, transient UI state (e.g. toggling a loading spinner, updating a counter) where re-rendering HTML would be wasteful. "Fat morphs" - sending a larger HTML fragment that includes surrounding context - are often simpler and more robust than trying to surgically update individual elements. Often a single template per page that renders the entire body is the best approach because it reduces complexity and avoids coordinating multiple partial updates.
- **Backend Reactivity:** The server renders HTML and manages application state. The frontend is a thin reactive layer that responds to backend updates. The backend determines what the user can do by controlling DOM patches, maintaining a single source of truth.
- **Simplicity First:** Keep Datastar expressions simple - complex logic belongs in backend handlers or external scripts. Use a "props down, events up" pattern: pass data into functions via arguments, return results or dispatch custom events. State that makes sense to keep on the client (e.g. UI toggles, form input) should be realized using client-side signals, state that should be persisted or shared should live on the server, and state necessary for actions should be communicated via signals.
- **No JavaScript:** Avoid solving problems with JavaScript. All application logic belongs in Go on the server. Datastar's declarative attributes (`data-on`, `data-bind`, `data-show`, etc.) and signal expressions handle the frontend. Only reach for JavaScript when there is genuinely no other way — e.g. clipboard access, focus management, or interfacing with a third-party browser API that Datastar cannot express.

## Step 1: Initialize

Run this:

```sh
datapages init --non-interactive --name myapp --module github.com/user/myapp
```

Prometheus metrics generation is enabled by default.
Use `--prometheus=false` to disable it.

It creates `app/app.go`, `app/app.templ`, `datapages.yaml`, `.env`, `compose.yaml`,
`Makefile`, `.vscode/extensions.json` and `.github/workflows/ci.yml`, appends `.env`
to `.gitignore`, and runs `datapages gen`, which writes `cmd/server/main.go`.

If the project already has `datapages.yaml`, skip this step.

### Where Configuration Lives

Nothing about the build is configured. Every setting is read from the code that
already states it.

Static file serving is turned on by an `embed.FS` whose doc comment names the
URL path it is served at, the way a page type names its route:

```go
// app/assets.go

// StaticFS is /static/
//go:embed static/*
var StaticFS embed.FS
```

The comment gives the `URLPrefix` constant of the generated `assets`
subpackage. The directive gives the `embed.FS` subdirectory and the dev-mode
disk path. A package with no such variable serves no files.

The app package, the metrics mode and the package to generate into are not
configured anywhere. They are the `App`, `Metrics` and `S` type arguments of the
`datapages.NewServer` call in `cmd/server/main.go`, which `datapages gen` reads:

```go
datapages.NewServer[
	app.App,                     // App
	datapages.DisableSessions,   // SessionData
	datapages.DisablePrometheus, // Metrics
	datapagesgen.Server,         // S
](...)
```

The generated code carries the Prometheus instrumentation for
`datapages.EnablePrometheus` and none for `datapages.DisablePrometheus`. Unlike an option slice, a type argument is always
written at the call site, so the generator can read the choice.

Generated code always goes into a `datapagesgen` package directly under the app
package it belongs to, so the `S` type argument must name it:

```
app/                app/datapagesgen/
app/frontend/       app/frontend/datapagesgen/
```

One module may build any number of applications, one app package each.
`datapages gen` generates all of them; `datapages watch` runs one, so a module
building more than one needs `datapages watch --app frontend`.

`datapages.yaml` keeps only what the tooling needs:

```yaml
cmd: cmd/server # Server cmd package path
watch: # Dev server settings for live-reload
  exclude:
    - ".git/**"
    - ".*"
    - "*~"
```

- `cmd` - where `datapages gen` writes the first `cmd/server/main.go`, and which
  command `datapages watch` builds while no `NewServer` call is written in a
  `main` package yet (default `cmd/server`). Once such a call exists, the
  command it is written in is the entry point and this key is unused. A module
  building several applications leaves it out.
- `watch` - dev server settings

## Step 2: Define Minimal App

Open `app/app.go`. The package name is `app`.

Write the `App` struct. Add all your dependencies to it.

```go
package app

import (
	"net/http"
	"github.com/a-h/templ"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return indexPage(), nil
}
```

Important:
1. The doc comment says `// PageIndex is /`. The word `is` matters. The route follows.
2. The struct has `App *App`. Every page needs this field. No exceptions.
3. The GET method uses a value receiver. Not a pointer.
4. Datapages rejects the package without an `App` or `PageIndex` struct types.

## Step 3: Add Session (Optional)

Declare an alias if you need authentication, otherwise skip this step.

```go
type SessionData struct {
	Name string
}

type Session = datapages.Session[SessionData]
```

`datapages.Session[Data]` is read-only and provides `UserID()`, `IsGuest()`,
`Token()`, `IssuedAt()`, `ExpiresAt()` and `Data()`. `Data` is your own payload,
use `struct{}` when you need none. All handlers must use the same `Data` type,
so declare the alias once and use it everywhere.

Handlers accept `session Session` as a parameter, matched by its type with a
free name, and return
`newSession datapages.NewSession[SessionData]` or `closeSession datapages.CloseSession`
as return values.
`NewSession` carries `UserID`, an optional `ExpiresAt` and `Data`,
datapages generates the token and stamps the issuance time.

## Step 4: Add Pages

One struct per page. One doc comment per page. One GET handler per page.

```go
// PageLogin is /login
type PageLogin struct{ App *App }

func (PageLogin) GET(r *http.Request) (body datapages.Component, err error) {
	return loginPage(), nil
}
```

Page names: `Page` then an uppercase letter then letters and digits.
`PageLogin` works. `Pagelogin` does not. `Page_Login` does not.
No underscores. No lowercase after `Page`.

Routes use Go standard library `net/http.ServeMux` pattern syntax.
`/item/{id}` captures a path segment. `/{path...}` captures the rest.
See https://pkg.go.dev/net/http#hdr-Patterns-ServeMux for the full spec.

### GET Return Values

The minimum is `(body datapages.Component, err error)`.
You can add more. Pick only what you need.
Return values are matched by their type, the names below are conventional.

```go
body datapages.Component // always first
head datapages.Head // optional
redirect datapages.Redirect // optional
newSession datapages.NewSession[Data] // optional
closeSession datapages.CloseSession // optional
enableBackgroundStreaming datapages.EnableBackgroundStreaming // optional
disableRefreshAfterHidden datapages.DisableRefreshAfterHidden // optional
err error // always last
```

Examples:

```go
// body + head
(body datapages.Component, head datapages.Head, err error)

// body + redirect
(body datapages.Component, redirect datapages.Redirect, err error)

// body + new session + disableRefreshAfterHidden
(
	body datapages.Component,
	newSession datapages.NewSession[Data],
	disableRefreshAfterHidden datapages.DisableRefreshAfterHidden,
	err error,
)
```

## Step 5: Path Variables and Query Parameters

These work in both GET handlers and action handlers.

### Path Variables

Put them in the route. Read them in the handler.

```go
// PageItem is /item/{id}
type PageItem struct{ App *App }

func (PageItem) GET(
	r *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
) (body datapages.Component, err error) {
	return itemPage(path.Values.ID), nil
}
```

The tag `path:"id"` must exactly match `{id}` in the route.

### Query Parameters

```go
func (PageSearch) GET(
	r *http.Request,
	query datapages.Query[struct {
		Term  string `query:"t"`
		Limit int    `query:"l"`
	}],
) (body datapages.Component, err error) {
	return searchPage(query.Values.Term, query.Values.Limit), nil
}
```

The `query` tag specifies the query parameter name. `query:"t"` maps to `?t=...` in the URL. See [SPECIFICATION.md](../../SPECIFICATION.md) for all supported field types.

## Step 6: Add Actions

Actions handle POST, PUT, PATCH, or DELETE. They are methods on page types similar to GET.
Give each one a doc comment with a route.

```go
// POSTSubmit is /login/submit
func (PageLogin) POSTSubmit(r *http.Request) error {
	return nil
}
```

Action names: `POST` then uppercase letter then letters and digits.
Same for `PUT`, `PATCH`, and `DELETE`.
`POSTSubmit` works. `POSTsubmit` does not. `POST_Submit` does not.
No underscores. No lowercase after `POST`/`PUT`/`PATCH`/`DELETE`.

Actions can also be defined on `*App` (pointer receiver) for global actions not tied to a specific page:

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

### Action Parameters

Parameters may be in any order. Skip what you don't need.
Path, query and signals are recognized by their type, the parameter name is
free; their values sit in the `Values` field.

```go
r *http.Request
sse datapages.SSE // optional
session Session // optional
path datapages.Path[struct { ID string `path:"id"` }] // optional
query datapages.Query[struct { P int `query:"p"` }] // optional
signals datapages.Signals[struct { V string `json:"v"` }] // optional
somethingHappened datapages.Dispatcher[EventSomethingHappened] // optional
```

Import `"github.com/romshark/datapages"` for `datapages.SSE`.

See [Parameter: `sse datapages.SSE`](../../SPECIFICATION.md#parameter-sse-datapagessse)
for the interface. `datapages.SSE` is the only accepted SSE parameter type, in
action handlers, event handlers (`OnXXX`), stream hooks and `RecoverError`.

### Action Return Types

Pick only what you need.

```go
body datapages.Component // optional
head datapages.Head // optional
redirect datapages.Redirect // optional
newSession datapages.NewSession[Data] // optional
closeSession datapages.CloseSession // optional
err error // always last
```

Examples:

Simple:
```go
) error {
```

Redirect:
```go
) (redirect datapages.Redirect, err error) {
	return datapages.Redirect{URL: "/", Status: http.StatusSeeOther}, nil
}
```

New session:
```go
) (newSession datapages.NewSession[SessionData], redirect datapages.Redirect, err error) {
	return datapages.NewSession[SessionData]{UserID: "u1"}, datapages.Redirect{URL: "/"}, nil
}
```

Close session:
```go
) (closeSession datapages.CloseSession, redirect datapages.Redirect, err error) {
	return true, datapages.Redirect{URL: "/login"}, nil
}
```

HTTP error status (use the `datapages` sentinels):
```go
return datapages.ErrBadRequest                                    // 400, zero alloc
return datapages.ErrForbidden                                     // 403
return datapages.ErrConflict                                      // 409
return fmt.Errorf("%w: %w", datapages.ErrNotFound, errOriginal)   // 404, preserves original
```

Wrap at most one sentinel per error. With several, the first of `ErrBadRequest`,
`ErrForbidden`, `ErrNotFound`, `ErrConflict` decides the status.

Errors without a sentinel default to 500 (or `RecoverError` if defined).

## Step 7: Add Signals

Signals are Datastar frontend state. Inline struct with `json` tags.

```go
// POSTSubmit is /form/submit
func (PageForm) POSTSubmit(
	r *http.Request,
	signals datapages.Signals[struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}],
) error {
	return nil
}
```

Add `reflectsignal` to a query field to bind it to a Datastar signal. The query parameter initializes the signal value on page load, and when the signal changes, the browser URL is updated to reflect the new value:

```go
func (PageSearch) GET(
	r *http.Request,
	query datapages.Query[struct {
		Term string `query:"t" reflectsignal:"term"`
	}],
	signals datapages.Signals[struct {
		Term string `json:"term"`
	}],
) (body datapages.Component, err error) {
	return searchPage(query.Values.Term), nil
}
```

## Step 8: Add Events

Events push real-time updates over SSE.
Each event is defined by a type in the app source package.
Define the type. Write the doc comment with a quoted subject.

```go
// EventMessageSent is "messaging.sent"
type EventMessageSent struct {
	Message string `json:"message"`
}
```

Event names: `Event` then uppercase letter then letters and digits.
The subject is quoted. `"messaging.sent"` works. `messaging.sent` does not.

### Dispatch from Actions

Add a `datapages.Dispatcher[EventXXX]` parameter. Its name is free, the type is
what makes it a dispatcher.

```go
// POSTSend is /chat/send
func (PageChat) POSTSend(
	r *http.Request,
	messageSent datapages.Dispatcher[EventMessageSent],
) error {
	return messageSent.Dispatch(EventMessageSent{Message: "hello"})
}
```

One dispatcher publishes one event type, so declare one parameter per type.
Nothing is atomic across them, hence joining the errors:

```go
func (PageChat) POSTSend(
	r *http.Request,
	messageSent datapages.Dispatcher[EventMessageSent],
	userActive datapages.Dispatcher[EventUserActive],
) error {
	return errors.Join(
		messageSent.Dispatch(EventMessageSent{Message: "hello"}),
		userActive.Dispatch(EventUserActive{}),
	)
}
```

`Dispatch` publishes with the context of the handler that dispatches.
Use `DispatchCtx(ctx, event)` when the event goes out after the handler returned
or the publish needs its own deadline.

### Handle Events on Pages

Method name starts with `On`. Exactly one parameter of an event type and an
`sse datapages.SSE` are required. The event parameter is matched by its type,
the name is free. Optional parameters: `streamID datapages.StreamID`, `session Session`.
Parameters may appear in any order.

`On` handlers do **not** accept `signals`. If the handler needs client-side signal values, add them as fields on the event type and populate them in the action handler that dispatches the event.

Use `streamID` to look up per-tab state registered in `StreamOpen` (see Step 9).

```go
func (PageChat) OnMessageSent(
	event EventMessageSent,
	sse datapages.SSE,
	streamID datapages.StreamID, // Optional
	session Session, // Optional
) error {
	return sse.PatchElement(messageComponent(event.Message))
}
```

### Subject Fields

A field typed as one of the two datapages subject types is a subject field.
Its name is free. Subject fields must appear before any payload field.

| type | segment |
| ---- | ------- |
| `datapages.Subject` | a segment value |
| `datapages.SubjectUser` | the ID of the user the event is addressed to |

Subject field values are appended to the event's base NATS subject in field
order, separated by dots. Each field carries one value, so one dispatch
publishes to one subject.

A value may carry any byte. What cannot stand in a subject is escaped on the way in,
so an email address or any other dotted identifier is a valid value.
Only an empty value is refused: the dispatch returns an error and publishes nothing.

A `datapages.SubjectUser` field makes the event stream require authentication:
only the client authenticated as that user receives it, which requires a Session type.
The user ID is a subject value like any other and is escaped the same way,
so it can be an email address. Only its length is bounded: check an
ID with `datapages.ValidateUserID` before returning it as a `newSession`.

```go
// EventDirectMessage is "messaging.direct"
type EventDirectMessage struct {
	Recipient datapages.SubjectUser `json:"recipient"`

	Content string `json:"content"`
}
```

To reach several users or several rooms, dispatch once per value. The framework
never fans one dispatch out, so every publish fails on its own and the handler
decides what to do about it:

```go
for _, participant := range room.ParticipantIDs {
	err := directMessage.Dispatch(EventDirectMessage{
		Recipient: datapages.SubjectUser(participant),
		Content:   signals.Values.Text,
	})
	if err != nil {
		return err
	}
}
```

Any subject field other than `datapages.SubjectUser` can carry a
`signal:"<name>"` tag to bind it to a client-side Datastar signal: the stream
subscribes to the value that signal holds. The client supplies that value and
an empty one is refused with 400. A wildcard needs no refusing: escaped, it is
one literal segment, so a client sending `*` subscribes to that value alone.

```go
// EventCalcUpdated is "calc.updated"
type EventCalcUpdated struct {
	Instance datapages.Subject `json:"instance" signal:"instance_id"`

	Result float64 `json:"result"`
}
```

## Step 9: Add Stream Hooks (Optional)

`StreamOpen` and `StreamClose` run when a page's SSE stream opens and closes.

### When to use stream hooks

- **Per-tab server-side state.** Declare an exported struct and take
  `state *T` in `StreamOpen`, actions, and `OnXXX` handlers.
	The generator allocates one zeroed state per tab, serializes handler calls
  with a per-instance mutex, and drops the state on `StreamClose`.
	No manual tab-id signing or map bookkeeping. See Step 10.
- **CQRS read-model binding.** In a CQRS architecture, actions (commands)
  dispatch events and event handlers (queries) render the updated UI. The
  event handler needs context about *which* tab it is rendering for (e.g.
  which item is being viewed, which filters are active). Capture that context
  into `state *T` inside `StreamOpen`, then read it from the event handler
  via the same `state` parameter.
- **Resource lifecycle.** Acquire per-stream resources (subscriptions,
  connections, counters) in `StreamOpen` and release them in `StreamClose`.

### Signature

Both require `r *http.Request` and `streamID datapages.StreamID`.
Both return `error`, or nothing at all: `error` is the only return value they may declare.
The `streamID` is a per-process unique identifier for the SSE stream instance.
It is recognized by its `datapages.StreamID` type, the parameter name is free.
Use it to correlate open and close for the same stream.
It is intended for internal server-side bookkeeping only and must not be exposed
to clients, as it could leak information about server activity and connection volume.

`StreamOpen` runs after the SSE stream is established, before any event handler.
It additionally accepts these optional parameters:
`sse datapages.SSE`, `session Session`,
`signals datapages.Signals[struct{...}]`, `datapages.Dispatcher[EventXXX]`.

`StreamClose` runs when the stream closes.
It additionally accepts these optional parameters:
`session Session`, `datapages.Dispatcher[EventXXX]`.
Note: `StreamClose` does **not** accept `sse` or `signals`.

```go
func (PageIndex) StreamOpen(
	r *http.Request,
	streamID datapages.StreamID,
	sse datapages.SSE, // Optional
	session Session, // Optional
	signals datapages.Signals[struct { // Optional
		Instance string `json:"instance"`
	}],
	ping datapages.Dispatcher[EventPing], // Optional
) error {
	// Set up per-tab state, patch signals to the client, etc.
	return nil
}

func (PageIndex) StreamClose(
	r *http.Request,
	streamID datapages.StreamID,
	session Session, // Optional
	ping datapages.Dispatcher[EventPing], // Optional
) error {
	// Clean up per-tab state.
	return nil
}
```

Stream hooks can also be defined on abstract types and embedded in pages,
following the same pattern as event handlers (see next step).

## Step 10: Per-Tab Server-Side State (Optional)

When a page needs state that lives across a tab's lifetime but must not leak
between tabs — filters, cursors, which item is being viewed, a per-tab
counter — declare any exported struct type and accept `state *T` on the
handlers that read or write it.

```go
type IndexState struct {
    Filter string
    Cursor int
}

func (PageIndex) StreamOpen(
    r *http.Request,
    streamID datapages.StreamID,
    state *IndexState,
    signals datapages.Signals[struct {
        Filter string `json:"filter"`
    }],
) error {
    state.Filter = signals.Values.Filter
    return nil
}

// POSTFilter is /filter
func (PageIndex) POSTFilter(
    r *http.Request,
    sse datapages.SSE,
    state *IndexState,
    signals datapages.Signals[struct {
        Filter string `json:"filter"`
    }],
) error {
    state.Filter = signals.Values.Filter
    return sse.PatchElement(itemList(p.App.filter(state.Filter)))
}

func (PageIndex) OnItemsChanged(
    event EventItemsChanged,
    sse datapages.SSE,
    state *IndexState,
) error {
    return sse.PatchElement(itemList(p.App.filter(state.Filter)))
}
```

**Rules**:

- The state type is any exported named struct in the app package.
- A page (including any embedded abstract types) may reference at most one
  state type. Conflicts are generator errors.
- `GET` handlers cannot take `state` (no instance exists at render time).
- A page that takes `state` gets an SSE stream whether or not it declares
  `StreamOpen`, `StreamClose`, or an `OnXXX` handler. The stream is what bounds
  the instance's lifetime: it allocates the slot on connect and releases it on
  disconnect.
- Global `*App` actions can take `state *T`, but they only succeed when the
  calling tab is bound to a page that uses that same `T` — otherwise the
  runtime returns `409 Conflict`. App actions intended to work from every
  page should remain stateless.
- Always a pointer: `state *T`, never `state T`.
- A stateful handler may take `stateID string` alongside `state *T`.
  It names the calling tab in message broker subjects. The value is derived
  from the instance id and grants nothing on its own, which keeps the id
  out of broker logs and storage. Pair it with a `datapages.SubjectStateID`
  subject field on an event type:
  the generator auto-subscribes to `<base>.<state_id>` at stream connect
  so only the originating tab receives the event. Such a field must
  be the event's only subject field and the subscribing page must be
  stateful.

**What the generator does for you**:

- Allocates one zeroed state per tab, never reused by another tab.
- Signs an instance identifier on `GET` and threads it through the browser
  via a `Datapages-Instance` header (no cookies, no storage — in-memory only
  so other tabs on the same origin cannot impersonate).
- Serializes every handler call on the same instance under a per-instance
  mutex, so you never need to lock inside a handler.
- Returns `409 Conflict` with `Datapages-Retry: reconnect` if an action
  arrives before the SSE stream opens, or after the tab's state was
  released. The client shim reloads the page once per document on such a
  response, which reconnects the stream and mints a fresh instance. It does
  not retry the action, so unsaved form input is lost.
- Releases the state on `StreamClose`. An instance lives exactly as long as
  its stream, so a transient network blip resets per-tab state. Keep in `*T`
  only what a tab can afford to lose.

**Server configuration**. Stateful apps must opt in via
`datapages.WithStateConfig`:

```go
hmacKey := sha256.Sum256([]byte(hmacSecret))
opts = append(opts, datapages.WithStateConfig(datapages.StateConfig{
    HMACKey:                hmacKey[:],
    MaxConcurrentInstances: 10_000, // optional, 0 takes the default
}))
```

`NewServer` returns an error without this option. `MaxConcurrentInstances` caps how
many instances exist at the same time. A stream connect past the cap gets
`503` and Datastar retries it. Zero selects
`DefaultMaxConcurrentInstances` and a negative value removes the cap.

**Multi-server deployments**. State lives in process memory, so the load
balancer must route each client consistently to the same backend (cookie
hashing or hashing on the `Datapages-Instance` header). Round-robin load
balancing is incompatible — every other request will hit a server without the
state and get rejected with `409`.

## Step 11: Share Handlers Across Pages

When multiple pages need the same event handler or action, define it once on an abstract type and embed it. This avoids duplicating handler methods across pages.
Abstract types are not pages. No `Page` prefix. No route.

```go
type Base struct{ App *App }

func (Base) OnMessageSent(
	event EventMessageSent,
	sse datapages.SSE,
) error {
	return sse.PatchElement(notificationComponent())
}
```

Embed them in pages:

```go
// PageChat is /chat
type PageChat struct {
	App *App
	Base
}
```

Every page that embeds `Base` automatically gets `OnMessageSent` without repeating the code.
To override, redefine the method on the page - the page-level method replaces the embedded one entirely for that page, while other pages that embed `Base` keep the original. You can also wrap the embedded method by calling it from the override:

```go
func (p PageChat) OnMessageSent(
	event EventMessageSent,
	sse datapages.SSE,
) error {
	// Custom logic before
	log.Println("chat-specific handling")
	// Delegate to the embedded Base handler
	return p.Base.OnMessageSent(event, sse)
}
```

## Step 12: Add Custom Error Pages (Optional)

Without these, Datapages serves default error responses. Define custom error pages to match your app's look and feel and provide helpful navigation back to valid pages.

```go
// PageError404 is /not-found
type PageError404 struct{ App *App }

func (PageError404) GET(r *http.Request) (body datapages.Component, err error) {
	return notFoundPage(), nil
}
```

Same pattern for `PageError500`.

## Step 13: Add Global Head (Optional)

Adds shared `<head>` content (meta tags, stylesheets, scripts) to every page, so you don't have to repeat it in each page's `head` return value. Pointer receiver on App.

```go
func (*App) Head(
	r *http.Request,
	session Session, // optional
) datapages.Head {
	return globalHead()
}
```

Both parameters are matched by their type, the names and order are free.

## Step 14: Add Error Recovery (Optional)

When a handler returns an error during a Datastar SSE request, a plain HTTP error is invisible to the user - there is no visible feedback, only a console log that normal users never see. `RecoverError` lets you handle this gracefully by patching in an error UI (e.g. a toast notification) over SSE instead. All action handler errors (including the datapages sentinels) are routed through `RecoverError` when defined. Use `errors.Is(err, datapages.ErrBadRequest)` etc. inside `RecoverError` to distinguish error types.

```go
func (*App) RecoverError(
	err error,
	sse datapages.SSE,
) error {
	return sse.PatchElement(errorToast(err))
}
```

Both parameters are matched by their type, the names and order are free.

## Step 15: Configure the Server Entry Point

`datapages gen` generates `cmd/server/main.go` on the first run. After that, you own this file - it is not regenerated or overwritten. Edit it to configure dependencies, middleware, and server options.

The generated `main.go` imports two key packages from your project:

```go
import (
	"your-module/app" // your application package
	"your-module/app/datapagesgen" // generated server package
)
```

### Create the Server

`datapages.NewServer` requires your app and a message broker. Its type arguments
name your `App`, your `SessionData` (`datapages.DisableSessions` when you defined
no `Session` type), the `Metrics` mode (`datapages.EnablePrometheus` or
`datapages.DisablePrometheus`) and the generated `Server`:

```go
// Without sessions and without metrics:
s, err := datapages.NewServer[
	app.App, datapages.DisableSessions, datapages.DisablePrometheus, datapagesgen.Server,
](
	a, messageBroker, opts...,
)

// With sessions, the manager is an option:
opts = append(opts, datapages.WithSessionManager[app.SessionData](sessionManager))
s, err := datapages.NewServer[
	app.App, app.SessionData, datapages.DisablePrometheus, datapagesgen.Server,
](
	a, messageBroker, opts...,
)
```

`datapages.EnablePrometheus` requires `WithPrometheus`,
`datapages.DisablePrometheus` rejects it.

Name the session data type at the `WithSessionManager` call: it is not inferred
from a concrete manager, and naming it is what makes the compiler check the
manager against what the app declares.

`datapages gen` reads these type arguments to find the app package and the
package to generate into, so keep the call in the module.
Import `datapages` under a name, aliases included. The scan matches the call by its
qualifier and rejects a dot import, which leaves it no qualifier to match.

### Message Broker

A message broker is always required. It delivers events between pages and handles SSE fan-out.

Use core NATS for the message broker:

```go
import "github.com/romshark/datapages/modules/messaging/natscore"
```

An in-memory broker (`github.com/romshark/datapages/modules/messaging/inmem`) exists but should only be used in single-instance setups. Prefer core NATS in most cases.

### Session Manager

Required only if you defined a `Session` type. Use NATS KV for the session manager:

```go
import "github.com/romshark/datapages/modules/sessions/natskv"
```

An in-memory session manager (`github.com/romshark/datapages/modules/sessions/inmem`) exists but should only be used in single-instance setups where losing sessions on restart is acceptable. Prefer NATS KV in most cases.

### Server Options

Pass options to `NewServer` to configure middleware, CSRF protection, static files, TLS, etc.:

```go
var opts []datapages.ServerOption

// Middleware — adds custom HTTP middleware
opts = append(opts, datapages.WithMiddleware(func(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("access", slog.String("path", r.URL.Path))
		next.ServeHTTP(w, r)
	})
}))

// CSRF protection is on for every app with a Session type and needs no
// option. The token is derived from the session token.
// Configure it only to replace the tokens or to turn the protection off.
opts = append(opts, datapages.WithCSRFProtection(datapages.CSRFConfig{
	Tokens: myTokens,
}))

// Authentication (required when Session type is defined)
opts = append(opts, datapages.WithSessions(datapages.SessionsConfig{}))

// Custom logger (consider slog.LevelDebug when datapages.IsDevMode() is true)
opts = append(opts, datapages.WithLogger(slog.Default()))

// Custom HTTP server (Addr and Handler are always overwritten)
opts = append(opts, datapages.WithHTTPServer(&http.Server{
	ReadHeaderTimeout: 10 * time.Second,
}))

// Custom Datastar JS bundle URL (defaults to CDN)
opts = append(opts, datapages.WithDatastarJS("https://cdn.example.com/datastar.js"))

// Prometheus metrics on a dedicated HTTP server.
// Requires the datapages.EnablePrometheus type argument at the NewServer call.
opts = append(opts, datapages.WithPrometheus(datapages.PrometheusConfig{
	Host: ":9091",
}))
```

### Listen and Serve

```go
s.ListenAndServe(ctx, "localhost:8080")
// or with TLS:
s.ListenAndServeTLS(ctx, "localhost:8443", certPath, keyPath)
```

## Step 16: Serve Static Files (Optional)

If your app needs to serve static assets (CSS, JS, images, fonts), place them in a directory inside your app package (e.g. `app/static/`) and use Go's `embed` package to bundle them into the binary.

Create an `app/assets.go` file:

```go
package app

import "embed"

// StaticFS is /static/
//go:embed static/*
var StaticFS embed.FS
```

The doc comment names the URL path and is what turns serving on. Then hand the
filesystem to the server in `cmd/server/main.go`:

```go
opts = append(opts, datapages.WithAssets(app.StaticFS))
```

`WithAssets` carries only the `embed.FS`. The generated server applies what the
app package declared: in production it extracts the subdirectory (`assets.Dir`)
and serves the embedded files; in dev mode (`IsDevMode`) it serves from disk
(`assets.DevDir`) with caching disabled, for live reloading without
recompilation. An app package that declares no assets rejects the option.

The URL path prefix is the generated `assets.URLPrefix` constant, which comes from the doc comment of the `embed.FS` variable. The embed.FS subdirectory and dev-mode disk path come from its `//go:embed` directive.

Reference static files in templates through the generated `assets.Path` helper,
never a hardcoded path, so the prefix stays in one place:

```templ
<link rel="stylesheet" href={ assets.Path("style.css") }/>
<script src={ assets.Path("bundle.js") } defer></script>
```

In an `<a href>` use `href.Asset("style.css")` instead: the linter rejects a
hardcoded root-relative `href` there.

## Step 17: Generate and Run

Build workflow after editing `app.go` or `.templ` files:

```sh
templ generate        # compile .templ files to Go (required after .templ changes)
datapages gen         # parse app package and generate server code
go build ./cmd/server # build the server binary
```

If `datapages gen` reports errors, fix the Go source in `app/` and re-run.
Never edit files ending in `_gen.go` or files containing a `DO NOT EDIT` header comment — they are overwritten by code generation.

CLI reference:

```sh
datapages gen             # parse and generate code
datapages lint            # validate without generating
datapages watch           # live reload dev server (for humans, not AI)
datapages version         # show version info
datapages help            # show help for all commands and flags
datapages help <command>  # show help for a specific command
```

## Step 18: Use Generated URL Packages

`datapages gen` produces two packages with type-safe URL builders. **Always use these instead of hardcoding URLs.**

### `app/datapagesgen/href` — Page Links

Generated functions return URL strings for `<a href>` attributes. One function per page.

```templ
// Simple page
<a href={ href.PageIndex() }>Home</a>
<a href={ href.PageLogin() }>Log in</a>

// Page with path variable (e.g. PagePost is /post/{slug})
<a href={ href.PagePost(post.Slug) }>{ post.Title }</a>

// Page with query parameters
<a href={ href.PageMessages(href.QueryPageMessages{Chat: chatID}) }>Messages</a>
```

Each function is named after the page type, so `PageIndex` becomes `href.PageIndex`.
Query parameter structs are generated as `href.Query<PageTypeName>`.
Zero-value fields are omitted from the URL.

### `app/datapagesgen/action` — Datastar Actions

Generated functions return Datastar action strings (`@post('/...')`, `@put('/...')`, etc.) for use in `data-on:click` and similar attributes. One function per action handler.

```templ
// Simple action
<button data-on:click={ action.POSTPageLoginSubmit() }>Submit</button>

// Action with path variable
<button data-on:click={ action.POSTPagePostSendMessage(slug) }>Send</button>

// Action with query parameters
<button data-on:click={ action.POSTPageMessagesRead(
    action.QueryPOSTPageMessagesRead{MessageID: msg.ID},
) }>Mark Read</button>

// App-level action (not tied to a page)
<button data-on:click={ action.POSTAppSignOut() }>Sign Out</button>

// Action with Datastar options (e.g. payload, contentType, filterSignals)
<button data-on:click={ action.POSTPageLoginSubmit(
    action.WithContentType(action.ContentTypeForm),
    action.WithPayload("{extra: 1}"),
) }>Submit</button>

// Action with before/after expressions (joined with "; " separators)
<button data-on:click={ action.POSTPageLoginSubmit(
    action.WithBefore("$foo='asd'"),
    action.WithAfter("$foo=''"),
) }>Submit</button>
```

All generated action functions accept variadic modifiers.
One typed helper per [Datastar action option](https://data-star.dev/reference/actions#options):

| helper | argument |
| ------ | -------- |
| `action.WithContentType(ct)` | `action.ContentTypeJSON`, `action.ContentTypeForm` |
| `action.WithSelector(sel)` | CSS selector of the form to send |
| `action.WithFilterSignals(include, exclude)` | regex patterns, `exclude` may be empty |
| `action.WithHeaders(m)` | `map[string]string` |
| `action.WithOpenWhenHidden(b)` | `bool` |
| `action.WithPayload(expr)` | raw JavaScript expression |
| `action.WithRetry(r)` | `action.RetryAuto`, `RetryError`, `RetryAlways`, `RetryNever` |
| `action.WithRetryInterval(ms)` | `int` |
| `action.WithRetryScaler(x)` | `float64` |
| `action.WithRetryMaxWaitMs(ms)` | `int` |
| `action.WithRetryMaxCount(n)` | `int` |
| `action.WithRequestCancellation(rc)` | `action.RequestCancellationAuto`, `RequestCancellationCleanup`, `RequestCancellationDisabled` |
| `action.WithRequestCancellationController(expr)` | expression holding an `AbortController` |

Two more modifiers wrap the call itself:

- `action.WithBefore(expr)` prepends a JavaScript expression, joined with `"; "`.
- `action.WithAfter(expr)` appends a JavaScript expression, joined with `"; "`.

`action.WithOption(key, value string)` passes an option the helpers don't cover.
Both arguments are raw strings, the value a JavaScript expression.

Naming convention: `{METHOD}Page{PageName}{HandlerName}` for page actions, `{METHOD}App{HandlerName}` for app-level actions. Query parameter structs are generated as `action.Query<FunctionName>`.
