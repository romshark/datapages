# Datapages

[![CI](https://github.com/romshark/datapages/actions/workflows/ci.yml/badge.svg)](https://github.com/romshark/datapages/actions/workflows/ci.yml)
[![golangci-lint](https://github.com/romshark/datapages/actions/workflows/golangci-lint.yml/badge.svg)](https://github.com/romshark/datapages/actions/workflows/golangci-lint.yml)
[![Coverage Status](https://coveralls.io/repos/github/romshark/datapages/badge.svg?branch=main)](https://coveralls.io/github/romshark/datapages?branch=main)
[![Go Report Card](https://goreportcard.com/badge/github.com/romshark/datapages)](https://goreportcard.com/report/github.com/romshark/datapages)
[![Go Reference](https://pkg.go.dev/badge/github.com/romshark/datapages.svg)](https://pkg.go.dev/github.com/romshark/datapages)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
![Alpha](https://img.shields.io/badge/status-alpha-orange)

> **🧪 Alpha Software:** Datapages is still in early development 🚧.<br>
> APIs are subject to change and you may encounter bugs.

A [Templ](https://templ.guide) + Go + [Datastar](https://data-star.dev) web framework
for building dynamic, server-rendered web applications in pure Go.

**Focus on your business logic, generate the boilerplate**
Datapages parses your app source package and generates all the wiring.
Routing, sessions and authentication, SSE streams, CSRF protection,
type-safe URL and action helpers -
so your application code stays clean and takes full advantage of Go's strong
static typing and high performance.

## Getting Started

### Install

```sh
go install github.com/romshark/datapages@latest
```

### Initialize New Project

```sh
datapages init
```

## CLI Commands

| Command | Description |
|---|---|
| `datapages init`    | Initialize a new project with scaffolding and configuration. |
| `datapages gen`     | Parse the app model and generate the datapages package.      |
| `datapages watch`   | Start the live-reloading development server.                 |
| `datapages lint`    | Validate the app model without generating code.              |
| `datapages version` | Print CLI version information.                               |

## Configuration

Datapages reads configuration from `datapages.yaml` in the module root:

```yaml
app: app            # Path to the app source package (default)
gen:
  package: datapagesgen # Path to the generated package (default)
  prometheus: true      # Enable Prometheus metrics generation (default)
cmd: cmd/server     # Path to the server cmd package (default)
```

When `prometheus` is set to `false`, the generated server code will not include
Prometheus imports, metric variables, or the `WithPrometheus` server option.
Use `datapages init --prometheus=false` to scaffold a project without Prometheus.

The optional `watch` section configures the development server
(host, proxy timeout, TLS, compiler flags, custom watchers, etc.).

## Demo: Classifieds

This repository features a demo application resembling an online classifieds marketplace
under `example/classifieds`.
The code you'd write is in
[example/classifieds/app](https://github.com/romshark/datapages/tree/main/example/classifieds/app)
(the "source package").
The code that the generator produces is in
[example/classifieds/datapagesgen](https://github.com/romshark/datapages/tree/main/example/classifieds/datapagesgen).

To run the demo in development mode, use:

```sh
cd example/classifieds
make dev
```

You can then access:
- Preview: http://localhost:52000/
- Grafana Dashboards: http://localhost:3000/
- Prometheus UI: http://localhost:9091/

You can install [k6](https://k6.io/) and run `make load` in the background
to generate random traffic.
Increase the number of virtual users (`VU`) to apply more load to the server when needed.

To run the demo in production mode, use:

```sh
make stage
```

## Source Package

Generator requires a path to an application source package
that must contain an `App` type and the `type PageIndex struct`.

### App

The `App` type may optionally provide a method for custom global HTML `<head>` tags:

```go
func (*App) Head(
	r *http.Request,
	session Session,
) (templ.Component, error) {
	return globalHeadTags(session.UserID), error
}
```

The `Recover500` method allows you to recover `500 Internal Server` errors to improve UX by giving better feedback. If `Recover500` returns an error the server falls back to the ugly standard procedure.

```go
func (*App) Recover500(
	err error,
	sse *datastar.ServerSentEventGenerator,
) error {
	return sse.PatchElementTempl(errorToast(err))
}
```

### Pages

Individual pages are defined with `type PageXXX struct { App *App }` and
special methods:

- `GET`: handles `GET` requests.
- `POSTXXX`: handles `POST` action requests.
- `PUTXXX`: handles `PUT` action requests.
- `DELETEXXX`: handles `DELETE` action requests.
- `OnXXX`: subscribes to events in the SSE listener.

`XXX` is just a name placeholder.

Page types must only contain the exported `App *App` field, no more, no less.
Methods can be enriched with capabilities through parameters.

URLs must be specified by a strictly formatted comment
in [net/http Mux pattern syntax](https://pkg.go.dev/net/http#hdr-Patterns-ServeMux):

The page type `PageIndex` (for URL `/`) is required.

Page types `PageError500` and `PageError404` are optional special error pages for the
response codes `500` and `404` respectively.
Otherwise datapages will use its own defaults.

Handler method parameters and their order are defined and enforced by datapages.
Using unsupported parameter names and types will result in generator errors.

The `GET` method parameter lists must always start with `r *http.Request`,
followed by other parameters:

```go
func (PageIndex) GET(
	r *http.Request,
	sessionToken string, // Optional
	session Session, // Optional
	path struct{...}, // Required only when path variables are used in the URL
	query struct{...}, // Optional
	signals struct {...}, // Optional
	dispatch(
		EventSomethingHappened,
		EventSomethingElseHappened,
		//...
	) error // Optional
) (
	body templ.Component,
	head templ.Component, // Optional
	redirect string, // Optional
	redirectStatus int, // Optional
	newSession Session, // Optional
	closeSession bool, // Optional
	enableBackgroundStreaming bool, // Optional
	disableRefreshAfterHidden bool, // Optional
	err error
) {
	// ...
}
```

The SSE action handlers `POSTXXX`, `DELETEXXX` and `PUTXXX` method parameter lists must
always start with `r *http.Request`, followed by other parameters:

```go
// POSTActionName is <path>
func (PageIndex) POSTActionName(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator, // Optional
	session Session, // Optional
	path struct{...}, // Required only when path variables are used in the URL
	query struct{...}, // Optional
	signals struct {...}, // Optional
	dispatch(
		EventSomethingHappened,
		EventSomethingElseHappened,
		//...
	) error // Optional
) error {
	// ...
}
```

Action handler may omit the `sse` parameter and instead redirect,
return HTML, set/remove sessions.

```go
// POSTActionName is <path>
func (PageIndex) POSTActionName(
	r *http.Request,
	session Session, // Optional
	path struct{...}, // Required only when path variables are used in the URL
	query struct{...}, // Optional
	signals struct {...}, // Optional
	dispatch(
		EventSomethingHappened,
		EventSomethingElseHappened,
		//...
	) error // Optional
) (
	body templ.Component, // Optional
	head templ.Component, // Optional
	redirect string, // Optional
	redirectStatus int, // Optional
	newSession Session, // Optional
	closeSession bool, // Optional
	err error,
) {
	// ...
}
```

All `OnXXX` method parameter lists must always start with
the `event` parameter of an event type, followed by
`sse *datastar.ServerSentEventGenerator` and other parameters.
The `XXX` placeholder must always match the event name after the type's `Event` prefix.

```go
func (PageIndex) OnSomethingHappened(
	event EventSomethingHappened,
	sse *datastar.ServerSentEventGenerator,
	session Session, // Optional
) error {
	// ...
}
```

#### Abstract Page Types

Abstract page types can be embedded in page types to share functionality across pages:

```go
type Base struct{ App *App }

func (Base) OnSomethingHappened(
	event EventSomethingHappened,
	sse *datastar.ServerSentEventGenerator,
	session Session,
) error {
	// ...
}

// PageFoo is /foo
type PageFoo struct {
	App *App
	Base
}

func (PageFoo) GET(r *http.Request) (body templ.Component, err error) {
	return pageFoo(), nil
}

// PageBar is /bar
type PageBar struct {
	App *App
	Base
}

func (PageBar) GET(r *http.Request) (body templ.Component, err error) {
	return pageBar(), nil
}
```

The embeddable abstract page type must always have `App *App`
same as concrete page types.

---

<details>
	<summary>Example</summary>

```go
// EventSomethingHappened is "something.happened"
type EventSomethingHappened struct {
	WhoCausedIt string `json:"who-caused-it"`
}

// PageExample is /example
type PageExample struct { App *App }

func (p PageExample) GET(r *http.Request) (body templ.Component, err error) {
	data, err := p.App.fetchData("")
	if err != nil {
		return nil, err
	}
	return examplePageTemplate(data), nil
}

// POSTInputChanged is /example/input-changed
func (p PageExample) POSTInputChanged(
	r *http.Request,
	session Session,
	signals struct {
		InputValue string `json:"inputvalue"`
	}
) (body templ.Component, err error) {
	// Patch the page with a fat morph directly on action.
	data, err := p.App.fetchData(signals.InputValue)
	if err != nil {
		return nil, err
	}
	return examplePageTemplate(data), nil
}

// POSTButtonClicked is /example/button-clicked
func (p PageExample) POSTButtonClicked(
	r *http.Request,
	session Session,
	dispatch(EventSomethingHappened) error,
) error {
	// Update everyone that something happened.
	return dispatch(EventSomethingHappened{WhoCausedIt: session.UserID})
}

func (p PageExample) OnSomethingHappened(
	event EventSomethingHappened,
	sse *datastar.ServerSentEventGenerator,
	session Session,
) error {
	// When something happens, patch the page.
	return sse.PatchElementTempl(updateTemplate())
}
```

</details>

#### Parameter: `signals struct {...}`

```go
signals struct {
	Foo string `json:"foo"`
	Bar int	`json:"bar"`
}
```

Provides the captured [Datastar signals](https://data-star.dev/guide/reactive_signals)
from the page.
Any named or anonymous struct is accepted,
but every field must have a json struct field tag.

#### Parameter: `path struct {...}`

```go
path struct {
	ID string `path:"id"`
}
```

Provides URL path parameters. These parameters must be defined in the URL comment.

#### Parameter: `query struct {...}`

```go
query struct {
	Filter string `query:"f"`
	Limit  int	`query:"l"`
}
```

Provides URL query parameters. These parameters must be defined in the URL comment.

The `reflectsignal` struct field tag can be used to define what signal shall reflect
into the query parameter:

```go
signals struct {
	SelectedItem string `json:"selecteditem"`
},
query struct {
	SelectedItem string `query:"s" reflectsignal:"selecteditem"`
}
```

The above example will automatically synchronize the query parameter `s` with the
signal `selecteditem`.

#### Parameter: `session Session`

```go
session Session
```

Provides authentication information from cookies.

If used, must be defined at the source package level as:

```go
type Session struct {
	UserID   string
	IssuedAt time.Time

	// Custom metadata.
	FooBar Bazz `json:"foo-bar"`
}
```

The `Session` type must have `UserID string` and `IssuedAt time.Time` fields.
`IssuedAt` is required because CSRF protection is bound to the session issuance time.
Any other field is treated as a custom payload.

#### Parameter: `sessionToken string`

```go
sessionToken string
```

Provides the session token from cookies.
Empty string if the request doesn't contain an authentication cookie.

If used `type Session struct` must be defined at the source package level.

```go
type Session struct {
	UserID     string    `json:"sub"` // Required.
	IssuedAt   time.Time `json:"iat"` // Required.
	Expiration time.Time `json:"exp"` // Optional.
}
```

#### Parameter: `sse *datastar.ServerSentEventGenerator`

```go
sse *datastar.ServerSentEventGenerator
```

This parameter is allowed only on `POSTXXX` page methods handling
`POST` [action requests](https://data-star.dev/reference/actions) and
`OnXXX` event handler page methods.
This gives you a handle to patch page elements, execute scripts, etc.

#### Parameter: `dispatch func(...) error`

```go
dispatch func(EventXXX, /*...*/) error
```

This parameter provides a function for dispatching events and
only accepts `EventXXX` types as parameters. These events can be handled
by `OnXXX` page methods.

An event type must use json struct field tags, and be strictly commented with
`// EventXXX is "xxx"` (where `"xxx"` is the NATS subject prefix):

```go
// EventExample is "example"
type EventExample struct {
	Information string `json:"info"`
}
```

Events that are targeted as specific user groups only, must declare the `TargetUserIDs`
field:

```go
type EventMessageSent struct {
	TargetUserIDs []string `json:"-"`

	Message string `json:"message"`
	Sender  string `json:"sender"`
}
```

You may provide multiple event types which are dispatched in the order of definition:

```go
dispatch func(EventTypeA, EventTypeB, EventTypeC) error
```

---

<details>
<summary>Example</summary>

```go
// EventMessageSent is "chat.sent"
type EventMessageSent struct {
	TargetUserIDs []string `json:"-"`

	Message string `json:"message"`
	Sender  string `json:"sender"`
}

// PageChat is /chat
type PageChat struct { App *App }

func (PageChat) POSTSendMessage(
	r *http.Request,
	e EventMessageSent,
	session Session,
	signals struct {
		InputText string `json:"inputtext"`
		ChatRoom  string `json:"chatroom"`
	},
	dispatch(EventMessageSent) error,
) error {
	if !isUserAllowedToSendMessages(session.UserID) {
		return errors.New("unauthorized")
	}
	if signals.InputText == "" {
		return nil // No-op.
	}
	return dispatch(EventMessageSent{
		TargetUserIDs: chatroom.ParticipantIDs,
		Message:	   signals.InputText,
		Sender:		session.UserID,
	})
}

func (PageChat) OnMessageSent(
	event EventMessageSent,
	sse *datastar.ServerSentEventGenerator,
	session Session,
) error {
	// Use sse to patch the new message into view.
}
```

</details>

#### Parameter: `metrics struct {...}` (Experimental)

This feature is in its design phase and not implemented yet.

```go
metrics struct {
	// Help description goes in this comment
	ExampleRequestsTotal interface {
		CounterAdd(delta float64, result string)
	} `name:"example_requests_total"`

	ExampleConnectionsOpen interface {
		GaugeSet(value float64)
	} `name:"example_connections_open" subsystem:"network"`

	ExampleOrderSize interface {
		HistogramObserve(value float64, )
	} `name:"order_size", buckets:"0,1,5,50,100,1000"``

	//...
},
```

Datapages can inject typed metric handles into page/action/event handlers,
similar to `signals`, `dispatch`, etc.
You declare what you need at the handler boundary, and the generator automatically
defines the Prometheus collectors and registers them.

The methods of the interface define the metric kind:

##### Counter

```go
interface {
	CounterAdd(label1, label2 string, /* ... */)
}
```

##### Gauge

```go
interface {
	GaugeSet(value float64, label1, label2 string, /* ... */)
}
```

##### Histogram

```go
interface {
	HistogramObserve(value float64, label1, label2 string, /* ... */)
}
```

Buckets can be defined using the `bucket` struct tag as a comma-separated list of values.

#### Return Value: `body templ.Component`

Specifies the [Templ](https://templ.guide/) template to use for the contents of the page.

#### Return Value: `head templ.Component`

Specifies the [Templ](https://templ.guide/) template to use for `<head>` tag of the page.

#### Return Value: `redirect string`

Allows for redirecting to different URLs.

#### Return Value: `redirectStatus int`

Specifies the redirect status code.
Can only be used in combination with `redirect`.

#### Return Value: `newSession Session`

```go
newSession Session
```

Adds response headers to set a session cookie if `newSession.UserID` is not empty,
otherwise no-op.

#### Return Value: `closeSession bool`

```go
closeSession bool
```

Closes the session and removes any session cookie if `true`, otherwise no-op.

#### Return Value `error` or `err error`

Regular error values that will be logged and followed by the error handling procedure.

#### `GET` Return Value: `enableBackgroundStreaming bool`

Can only be used for `GET` methods.

```go
enableBackgroundStreaming bool
```

By default, `OnXXX` event handlers can't deliver updates to background tabs.
If `true`, the SSE stream is always kept open. This prevents missed updates when the tab
is inactive, but increases battery and resource usage, especially on mobile devices.

This is equivalent to datastar's [`openWhenHidden`](https://data-star.dev/reference/actions)).

`enableBackgroundStreaming=true` will automatically disable the auto-refresh after
hidden. If you want to prevent this, you have to explicitly add
`disableRefreshAfterHidden` to the return values and set it to `false`.

#### `GET` Return Value: `disableRefreshAfterHidden bool`

Can only be used for `GET` methods.

```go
disableRefreshAfterHidden bool
```

By default, Datapages refreshes the page when it becomes active again after being in the
background (for example, when switching back from another tab).
This is useful when `enableBackgroundStreaming` is `false`, since SSE events may be missed
while the tab is inactive and the page state can become stale.
You can disable this behavior by returning `disableRefreshAfterHidden=true`.

Datapages relies on the
[`visibilitychange`](https://developer.mozilla.org/en-US/docs/Web/API/Document/visibilitychange_event)
event to perform the automatic refresh.

## Technical Limitations

- For now, with CSRF protection enabled, you will not be able to use plain HTML forms,
  since the CSRF token is auto-injected for Datastar `fetch` requests
  (where `Datastar-Request` header is `true`).
  You must use Datastar actions for any sort of server interactivity.

## Modules

Datapages ships pluggable modules with swappable implementations:

- [`SessionManager[S]`](modules/sessmanager/sessmanager.go)
  - [`natskv`](https://pkg.go.dev/github.com/romshark/datapages/modules/sessmanager/natskv) -
  - [`inmem`](https://pkg.go.dev/github.com/romshark/datapages/modules/sessmanager/inmem) -
    In-memory sessions (lost on restart; single-instance only)
    NATS KV store with AES-128-GCM encrypted cookies
- [`MessageBroker`](modules/msgbroker/msgbroker.go)
  - [`natsjs`](https://pkg.go.dev/github.com/romshark/datapages/modules/msgbroker/natsjs) -
  - [`inmem`](https://pkg.go.dev/github.com/romshark/datapages/modules/msgbroker/inmem) -
    In-memory fan-out message broker (single-instance only)
    NATS JetStream backed message broker
- [`TokenManager`](modules/csrf/csrf.go)
  - [`hmac`](https://pkg.go.dev/github.com/romshark/datapages/modules/csrf/hmac) -
    HMAC-SHA256 with BREACH-resistant masking
- [`TokenGenerator`](modules/sessmanager/sessmanager.go)
  - [`sesstokgen`](https://pkg.go.dev/github.com/romshark/datapages/modules/sesstokgen) -
    Cryptographically random session tokens (256-bit)



## Development

### Prerequisites

- [Go](https://go.dev/dl/) (see version in `go.mod`)

### Contributing

See [CLAUDE.md](CLAUDE.md) for code style, testing
conventions, commit message format, and project structure.

Use the `example/classifieds/` application as a real-world
test fixture when developing Datapages.
