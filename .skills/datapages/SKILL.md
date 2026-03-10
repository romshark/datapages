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
  Handlers return `templ.Component`. You write `.templ` files that compile to Go
  via `templ generate`. Datapages does **not** run this automatically — you must
  run `templ generate` yourself after creating or modifying `.templ` files.
  Docs: https://templ.guide/developer-tools/llm/
- **Datastar** (`github.com/starfederation/datastar-go/datastar`) - Frontend
  reactivity via HTML attributes and SSE. Actions use `data-on-click` and
  `data-action` attributes in templates. Docs: https://data-star.dev
  See also: [datastar/SKILL.md](../datastar/SKILL.md)

## Architecture

- **Hypermedia-First (MPA):** This is a multi-page application architecture. The backend drives the UI by sending HTML fragments, signal updates, and real-time events over SSE. There is no separate REST API layer - all interactions happen through Datastar SSE streams managed by Datapages.
- **Morphing & Patching:** Datastar uses morphing to update the DOM - it compares the incoming HTML fragment with the existing DOM and applies minimal changes, preserving focus, scroll position, and CSS transitions. Prefer HTML fragment patches (morphs) over signal updates because morphs carry both structure and data, keeping the server as the single source of truth. Use signal updates only for lightweight, transient UI state (e.g. toggling a loading spinner, updating a counter) where re-rendering HTML would be wasteful. "Fat morphs" - sending a larger HTML fragment that includes surrounding context - are often simpler and more robust than trying to surgically update individual elements. Often a single template per page that renders the entire body is the best approach because it reduces complexity and avoids coordinating multiple partial updates.
- **Backend Reactivity:** The server renders HTML and manages application state. The frontend is a thin reactive layer that responds to backend updates. The backend determines what the user can do by controlling DOM patches, maintaining a single source of truth.
- **Simplicity First:** Keep Datastar expressions simple - complex logic belongs in backend handlers or external scripts. Use a "props down, events up" pattern: pass data into functions via arguments, return results or dispatch custom events. State that makes sense to keep on the client (e.g. UI toggles, form input) should be realized using client-side signals, state that should be persisted or shared should live on the server, and state necessary for actions should be communicated via signals.

## Step 1: Initialize

Run this:

```sh
datapages init --non-interactive --name myapp --module github.com/user/myapp
```

Prometheus metrics generation is enabled by default.
Use `--prometheus=false` to disable it.

It creates `app/app.go`, `app/app.templ`, `datapages.yaml`, `.env`, `compose.yaml`,
`Makefile`, and `cmd/server/main.go`.

If the project already has `datapages.yaml`, skip this step.

### `datapages.yaml` Structure

The generated config looks like this:

```yaml
app: app # App source package path
gen:
  package: datapagesgen # Generated code package path
  prometheus: true # Enable Prometheus metrics generation
cmd: cmd/server # Server cmd package path
watch: # Dev server settings for live-reload
  exclude:
    - ".git/**"
    - ".*"
    - "*~"
```

The fields agents are most likely to change (usually not necessary):

- `app` — path to the app source package (default `app`)
- `gen.package` — where generated code goes (default `datapagesgen`)
- `gen.prometheus` — set `false` to disable metrics generation
- `cmd` — path to the server command package (default `cmd/server`)

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

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return indexPage(), nil
}
```

Important:
1. The doc comment says `// PageIndex is /`. The word `is` matters. The route follows.
2. The struct has `App *App`. Every page needs this field. No exceptions.
3. The GET method uses a value receiver. Not a pointer.
4. Datapages rejects the package without an `App` or `PageIndex` struct types.

## Step 3: Add Session (Optional)

Define a `Session` struct if you need authentication, otherwise don't define it.

```go
type Session struct {
	UserID   string
	IssuedAt time.Time
}
```

Both fields are required. Without either, the parser rejects it.
Add custom fields if you want. Custom fields must have `json` tags.

Now handlers can accept `session Session` or `sessionToken string` as parameters, and return `newSession Session` or `closeSession bool` as return values.

## Step 4: Add Pages

One struct per page. One doc comment per page. One GET handler per page.

```go
// PageLogin is /login
type PageLogin struct{ App *App }

func (PageLogin) GET(r *http.Request) (body templ.Component, err error) {
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

The minimum is `(body templ.Component, err error)`.
You can add more. Pick only what you need.

```go
body templ.Component // always first
head templ.Component // optional
redirect string // optional
redirectStatus int // only with redirect
newSession Session // optional
closeSession bool // optional
enableBackgroundStreaming bool // optional
disableRefreshAfterHidden bool // optional
err error // always last
```

Examples:

```go
// body + head
(body, head templ.Component, err error)

// body + redirect
(body templ.Component, redirect string, err error)

// body + new session + disableRefreshAfterHidden
(body templ.Component, newSession Session, disableRefreshAfterHidden bool, err error)
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
	path struct {
		ID string `path:"id"`
	},
) (body templ.Component, err error) {
	return itemPage(path.ID), nil
}
```

The tag `path:"id"` must exactly match `{id}` in the route.

### Query Parameters

```go
func (PageSearch) GET(
	r *http.Request,
	query struct {
		Term  string `query:"t"`
		Limit int    `query:"l"`
	},
) (body templ.Component, err error) {
	return searchPage(query.Term, query.Limit), nil
}
```

The `query` tag specifies the query parameter name. `query:"t"` maps to `?t=...` in the URL. See [SPECIFICATION.md](../../SPECIFICATION.md) for all supported field types.

## Step 6: Add Actions

Actions handle POST, PUT, or DELETE. They are methods on page types similar to GET.
Give each one a doc comment with a route.

```go
// POSTSubmit is /login/submit
func (PageLogin) POSTSubmit(r *http.Request) error {
	return nil
}
```

Action names: `POST` then uppercase letter then letters and digits.
Same for `PUT` and `DELETE`.
`POSTSubmit` works. `POSTsubmit` does not. `POST_Submit` does not.
No underscores. No lowercase after `POST`/`PUT`/`DELETE`.

Actions can also be defined on `*App` (pointer receiver) for global actions not tied to a specific page:

```go
// POSTSignOut is /sign-out/{$}
func (*App) POSTSignOut(r *http.Request, session Session) (
	closeSession bool,
	redirect string,
	err error,
) {
	return true, "/login", nil
}
```

### Action Parameters

Parameters may be in any order. Skip what you don't need.

```go
r *http.Request
sse *datastar.ServerSentEventGenerator // optional
sessionToken string // optional
session Session // optional
path struct { ID string `path:"id"` } // optional
query struct { P int `query:"p"` } // optional
signals struct { V string `json:"v"` } // optional
dispatch func(EventFoo) error // optional
```

Import `"github.com/starfederation/datastar-go/datastar"` for SSE.

### Action Return Types

Pick only what you need.

```go
body templ.Component // optional
head templ.Component // optional
redirect string // optional
redirectStatus int // only with redirect
newSession Session // optional
closeSession bool // optional
err error // always last
```

Examples:

Simple:
```go
) error {
```

Redirect:
```go
) (redirect string, redirectStatus int, err error) {
	return "/", 303, nil
}
```

New session:
```go
) (newSession Session, redirect string, err error) {
	return Session{UserID: "u1", IssuedAt: time.Now()}, "/", nil
}
```

Close session:
```go
) (closeSession bool, redirect string, err error) {
	return true, "/login", nil
}
```

## Step 7: Add Signals

Signals are Datastar frontend state. Inline struct with `json` tags.

```go
// POSTSubmit is /form/submit
func (PageForm) POSTSubmit(
	r *http.Request,
	signals struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	},
) error {
	return nil
}
```

Add `reflectsignal` to a query field to bind it to a Datastar signal. The query parameter initializes the signal value on page load, and when the signal changes, the browser URL is updated to reflect the new value:

```go
func (PageSearch) GET(
	r *http.Request,
	query struct {
		Term string `query:"t" reflectsignal:"term"`
	},
	signals struct {
		Term string `json:"term"`
	},
) (body templ.Component, err error) {
	return searchPage(query.Term), nil
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

Add `dispatch` as the last parameter.

```go
// POSTSend is /chat/send
func (PageChat) POSTSend(
	r *http.Request,
	dispatch func(EventMessageSent) error,
) error {
	return dispatch(EventMessageSent{Message: "hello"})
}
```

Multiple events:

```go
dispatch func(EventMessageSent, EventUserActive) error
```

### Handle Events on Pages

Method name starts with `On`. The `event` and `sse` parameters are required. Optional parameters: `session Session`, `sessionToken string`, `signals struct{...}`.
Parameters may appear in any order.
The `signals` parameter carries the client's Datastar signal values from the initial SSE connection request, not from the time each event fires.

```go
func (PageChat) OnMessageSent(
	event EventMessageSent,
	sse *datastar.ServerSentEventGenerator,
	session Session, // Optional
	sessionToken string, // Optional
	signals struct{...}, // Optional
) error {
	return sse.PatchElementTempl(messageComponent(event.Message))
}
```

### Target Specific Users

Add `TargetUserIDs []string` with `json:"-"`. Requires a Session type because the server uses `Session.UserID` to match connected users against the target list.
Separate `TargetUserIDs` from payload fields with an empty line for readability.

```go
// EventDirectMessage is "messaging.direct"
type EventDirectMessage struct {
	TargetUserIDs []string `json:"-"`

	Content string `json:"content"`
}
```

## Step 9: Share Handlers Across Pages

When multiple pages need the same event handler or action, define it once on an abstract type and embed it. This avoids duplicating handler methods across pages.
Abstract types are not pages. No `Page` prefix. No route.

```go
type Base struct{ App *App }

func (Base) OnMessageSent(
	event EventMessageSent,
	sse *datastar.ServerSentEventGenerator,
) error {
	return sse.PatchElementTempl(notificationComponent())
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
	sse *datastar.ServerSentEventGenerator,
) error {
	// Custom logic before
	log.Println("chat-specific handling")
	// Delegate to the embedded Base handler
	return p.Base.OnMessageSent(event, sse)
}
```

## Step 10: Add Custom Error Pages (Optional)

Without these, Datapages serves default error responses. Define custom error pages to match your app's look and feel and provide helpful navigation back to valid pages.

```go
// PageError404 is /not-found
type PageError404 struct{ App *App }

func (PageError404) GET(r *http.Request) (body templ.Component, err error) {
	return notFoundPage(), nil
}
```

Same pattern for `PageError500`.

## Step 11: Add Global Head (Optional)

Adds shared `<head>` content (meta tags, stylesheets, scripts) to every page, so you don't have to repeat it in each page's `head` return value. Pointer receiver on App.

```go
func (*App) Head(
	r *http.Request,
	sessionToken string, // optional
	session Session, // optional
) templ.Component {
	return globalHead()
}
```

## Step 12: Add Error Recovery (Optional)

When a handler returns an error during a Datastar SSE request, a plain HTTP 500 is invisible to the user - there is no visible feedback, only a console log that normal users never see. `Recover500` lets you handle this gracefully by patching in an error UI (e.g. a toast notification) over SSE instead.

```go
func (*App) Recover500(
	err error,
	sse *datastar.ServerSentEventGenerator,
) error {
	return sse.PatchElementTempl(errorToast(err))
}
```

## Step 13: Configure the Server Entry Point

`datapages gen` generates `cmd/server/main.go` on the first run. After that, you own this file - it is not regenerated or overwritten. Edit it to configure dependencies, middleware, and server options.

The generated `main.go` imports two key packages from your project:

```go
import (
	"your-module/app" // your application package
	"your-module/datapagesgen" // generated server package
)
```

### Create the Server

`datapagesgen.NewServer` requires your app, a message broker, and (if you defined a `Session` type) a session manager:

```go
// Without sessions:
s := datapagesgen.NewServer(a, messageBroker, opts...)

// With sessions:
s := datapagesgen.NewServer(a, messageBroker, sessionManager, opts...)
```

### Message Broker

A message broker is always required. It delivers events between pages and handles SSE fan-out.

Use NATS JetStream for the message broker:

```go
import "github.com/romshark/datapages/modules/msgbroker/natsjs"
```

An in-memory broker (`github.com/romshark/datapages/modules/msgbroker/inmem`) exists but should only be used in single-instance setups that don't require persistence. Prefer NATS JetStream in most cases.

### Session Manager

Required only if you defined a `Session` type. Use NATS KV for the session manager:

```go
import "github.com/romshark/datapages/modules/sessmanager/natskv"
```

An in-memory session manager (`github.com/romshark/datapages/modules/sessmanager/inmem`) exists but should only be used in single-instance setups where losing sessions on restart is acceptable. Prefer NATS KV in most cases.

### Server Options

Pass options to `NewServer` to configure middleware, CSRF protection, static files, TLS, etc.:

```go
var opts []datapagesgen.ServerOption

// Middleware — adds custom HTTP middleware
opts = append(opts, datapagesgen.WithMiddleware(func(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("access", slog.String("path", r.URL.Path))
		next.ServeHTTP(w, r)
	})
}))

// CSRF protection (required for session-based apps)
opts = append(opts, datapagesgen.WithCSRFProtection(datapagesgen.CSRFConfig{
	TokenManager:   tm,
	DevBypassToken: os.Getenv("CSRF_DEV_BYPASS"),
}))

// Authentication (required when Session type is defined)
opts = append(opts, datapagesgen.WithAuth(datapagesgen.AuthConfig{}))

// Custom logger (consider slog.LevelDebug when datapagesgen.IsDevMode() is true)
opts = append(opts, datapagesgen.WithLogger(slog.Default()))

// Custom HTTP server (Addr and Handler are always overwritten)
opts = append(opts, datapagesgen.WithHTTPServer(&http.Server{
	ReadHeaderTimeout: 10 * time.Second,
}))

// Custom Datastar JS bundle URL (defaults to CDN)
opts = append(opts, datapagesgen.WithDatastarJS("https://cdn.example.com/datastar.js"))

// Prometheus metrics on a dedicated HTTP server
opts = append(opts, datapagesgen.WithPrometheus(datapagesgen.PrometheusConfig{
	Host: ":9091",
}))
```

### Listen and Serve

```go
s.ListenAndServe(ctx, "localhost:8080")
// or with TLS:
s.ListenAndServeTLS(ctx, "localhost:8443", certPath, keyPath)
```

## Step 14: Serve Static Files (Optional)

If your app needs to serve static assets (CSS, JS, images, fonts), place them in a directory inside your app package (e.g. `app/static/`) and use Go's `embed` package to bundle them into the binary. This ensures the server is self-contained and deployable as a single binary without needing to ship a separate assets directory.

Create an `app/assets.go` file:

```go
package app

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var embedFS embed.FS

// FSStatic returns an http.FileSystem from the embedded filesystem.
// Use this in production.
func FSStatic() (http.FileSystem, error) {
	subFS, err := fs.Sub(embedFS, "static")
	if err != nil {
		return nil, err
	}
	return http.FS(subFS), nil
}

// FSStaticDev returns an http.FileSystem that reads directly from disk.
// Use this in development for live reloading without recompilation.
func FSStaticDev() http.FileSystem {
	return http.Dir("./app/static")
}
```

Then register the static filesystem in `cmd/server/main.go` using a server option:

```go
fsStatic, err := app.FSStatic()
if err != nil {
	slog.Error("preparing static fs", slog.Any("err", err))
	os.Exit(1)
}
opts = append(opts,
	datapagesgen.WithStaticFS("/static/", fsStatic, app.FSStaticDev()))
```

`WithStaticFS` takes a URL path prefix, the production filesystem, and an optional development filesystem. The dev filesystem serves files with caching disabled and falls back to the production filesystem if `nil`.

Reference static files in templates using the URL path prefix (e.g. `/static/style.css`).

## Step 15: Generate and Run

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

## Step 16: Use Generated URL Packages

`datapages gen` produces two packages with type-safe URL builders. **Always use these instead of hardcoding URLs.**

### `datapagesgen/href` — Page Links

Generated functions return URL strings for `<a href>` attributes. One function per page.

```templ
// Simple page
<a href={ href.Index() }>Home</a>
<a href={ href.Login() }>Log in</a>

// Page with path variable (e.g. PagePost is /post/{slug})
<a href={ href.Post(post.Slug) }>{ post.Title }</a>

// Page with query parameters
<a href={ href.Messages(href.QueryMessages{Chat: chatID}) }>Messages</a>
```

Query parameter structs are generated as `href.Query<PageName>`. Zero-value fields are omitted from the URL.

### `datapagesgen/action` — Datastar Actions

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
    action.WithOption(action.OptPayload, "'auto'"),
) }>Submit</button>
```

All generated action functions accept variadic `action.WithOption(key, value)` arguments to pass [Datastar action options](https://data-star.dev/reference/actions#options). The key is an `action.Opt` constant (e.g. `OptContentType`, `OptFilterSignals`, `OptSelector`, `OptHeaders`, `OptOpenWhenHidden`, `OptPayload`, `OptRetry`, `OptRetryInterval`, `OptRetryScaler`, `OptRetryMaxWaitMs`, `OptRetryMaxCount`, `OptRequestCancellation`). The value is a raw JavaScript expression string (use `"'auto'"` for a JS string, `"true"` for a boolean).

Naming convention: `{METHOD}Page{PageName}{HandlerName}` for page actions, `{METHOD}App{HandlerName}` for app-level actions. Query parameter structs are generated as `action.Query<FunctionName>`.
