# Datapages Specification

## Source Package

Generator requires a path to an application source package
that must contain an `App` type and the `type PageIndex struct`.

### App

The `App` type may optionally provide a method for custom global HTML `<head>` tags:

```go
func (*App) Head(
	r *http.Request,
	sessionToken string, // Optional
	session Session, // Optional
) templ.Component {
	return globalHeadTags()
}
```

The `RecoverError` method allows you to recover from handler errors to improve UX by
giving better feedback over SSE. All action handler errors (including httperr sentinels)
are routed through `RecoverError` when it is defined and the request is
a Datastar request. If `RecoverError` returns an error, the server falls back to
an HTTP error response using the appropriate status code.

```go
func (*App) RecoverError(
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
- `PATCHXXX`: handles `PATCH` action requests.
- `DELETEXXX`: handles `DELETE` action requests.
- `StreamOpen`: runs when the page SSE stream opens.
- `StreamClose`: runs when the page SSE stream closes.
- `OnXXX`: subscribes to events in the SSE listener.

Any action method, `OnXXX`, `StreamOpen`, or `StreamClose` may also take
`state *T` to opt the page into per-tab server-side state — see
[Parameter: `state *T`](#parameter-state-t).

`XXX` is just a name placeholder.

Page types must only contain the exported `App *App` field, no more, no less.
Methods can be enriched with capabilities through parameters.

URLs must be specified by a strictly formatted comment
in [net/http Mux pattern syntax](https://pkg.go.dev/net/http#hdr-Patterns-ServeMux):

The page type `PageIndex` (for URL `/`) is required.

Page types `PageError500` and `PageError404` are optional special error pages for the
response codes `500` and `404` respectively.
Otherwise datapages will use its own defaults.

Handler method parameters and return values are defined and enforced by datapages.
Parameters and return values may be in any order. Using unsupported parameter or
return value names and types will result in generator errors.

The `GET` method parameter lists must include `r *http.Request`
and may include the following optional parameters:

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

Action handlers can also be defined on `*App` (pointer receiver) for global actions
not tied to a specific page:

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

The SSE action handlers `POSTXXX`, `PUTXXX`, `PATCHXXX` and `DELETEXXX` method parameter lists must
include `r *http.Request` and may include the following optional parameters:

```go
// POSTActionName is <path>
func (PageIndex) POSTActionName(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator, // Optional
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
) error {
	// ...
}
```

Action handlers that omit the `sse` parameter can instead redirect,
return HTML, and set or remove sessions.

**Session mutation and SSE are mutually exclusive in action handlers.**
When the `sse` parameter is present, the handler opens a long-lived SSE stream —
HTTP headers (including session cookies) have already been sent, so `newSession`
and `closeSession` return values cannot be used.

```go
// POSTActionName is <path>
func (PageIndex) POSTActionName(
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

All `OnXXX` method parameter lists must include
the `event` parameter of an event type and
`sse *datastar.ServerSentEventGenerator`. Parameters may be in any order.
The `XXX` placeholder must always match the event name after the type's `Event` prefix.

```go
func (PageIndex) OnSomethingHappened(
	event EventSomethingHappened,
	sse *datastar.ServerSentEventGenerator,
	streamID uint64, // Optional
	sessionToken string, // Optional
	session Session, // Optional
) error {
	// ...
}
```

`StreamOpen` runs after the page SSE stream has been established and before
any event handler is invoked.
It may return only `error`. If it returns an error, stream setup stops immediately and
the stream is closed.
Datapages handles the error like any other Datastar request error: if `RecoverError`
is defined it is invoked, otherwise the server falls back to its internal-error path.
The `streamID` is a per-process unique identifier for the SSE stream instance.
Use it to correlate `StreamOpen` and `StreamClose` for the same stream.
It's intended for internal server-side bookkeeping only and
should not be exposed to clients.

```go
func (PageIndex) StreamOpen(
	r *http.Request,
	streamID uint64,
	sse *datastar.ServerSentEventGenerator, // Optional
	sessionToken string, // Optional
	session Session, // Optional
	signals struct{...}, // Optional
	dispatch(
		EventSomethingHappened,
		EventSomethingElseHappened,
		//...
	) error, // Optional
) error {
	// ...
}
```

`StreamClose` runs when the page SSE stream closes.
It may return only `error`.
If it returns an error, datapages logs the error server-side.

```go
func (PageIndex) StreamClose(
	r *http.Request,
	streamID uint64,
	sessionToken string, // Optional
	session Session, // Optional
	dispatch(
		EventSomethingHappened,
		EventSomethingElseHappened,
		//...
	) error, // Optional
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

#### Parameter: `state *T`

```go
state *T
```

Provides per-page-instance server-side state. A **page instance** corresponds to
an open browser tab: two tabs on the same page receive independent `*T`
values. State is held in server memory. Handlers on the same instance are
serialized by a per-instance mutex, so fields may be read and written without
additional synchronization inside a handler.

A page opts into state by declaring an exported struct and referencing it via
`state *T` on one or more action methods, `OnXXX` handlers, `StreamOpen`, or
`StreamClose`. The type may carry any exported name — `StateIndex` and
`TabContext` are both accepted:

```go
type StateIndex struct {
    Filter string
    Count  int
}

func (PageIndex) StreamOpen(
    r *http.Request,
    streamID uint64,
    state *StateIndex,
) error { /* ... */ }

func (PageIndex) POSTIncrement(
    r *http.Request,
    state *StateIndex,
) error {
    state.Count++
    return nil
}

func (PageIndex) OnSomething(
    event EventSomething,
    sse *datastar.ServerSentEventGenerator,
    state *StateIndex,
) error { /* read state.Count, etc. */ }
```

**Declaration rules**:

- The state type is an exported struct declared at the source package level.
  Its name is free; the generator derives its runtime symbols from it.
- The parameter name is `state` and the type is a pointer to a named
  struct. A value-type parameter is a generator error.
- All handlers on a page, including those inherited from embedded abstract
  pages, must reference the same state type.
- Abstract (embedded) page types may reference a state type on their own
  handlers; the binding flows into every concrete page that embeds them.
- Global `*App` actions may take `state *T`. The runtime resolves the slot
  using the calling tab's `Datapages-Instance` header against the map for
  `T`; an App action succeeds only when the calling tab is bound to a page
  that uses the same `T`, and otherwise receives `409 Conflict` with
  `Datapages-Retry: reconnect`. An App action that must be callable from
  every page must remain stateless.
- `GET` handlers must not take `state`: no instance exists at render time.
- A page that takes `state` must declare at least one of `StreamOpen`,
  `StreamClose`, or an `OnXXX` event handler. The SSE stream anchors the
  state lifecycle — without a stream there is nothing to bind the slot to,
  and actions would be rejected indefinitely with `409 Conflict`.

**Generic abstract pages**. A generic abstract may declare `state *S` on
its handlers where `S` is one of its type parameters. Each concrete page
then embeds the abstract with a concrete type argument
(e.g. `Base[UserContext]`); the parser substitutes `S` with that argument
at the embed site. This lets a single shared abstract layer cooperate with
different per-page state shapes without forcing every page to use the same
state fields.

A generic abstract may embed another one and pass its own type parameter down
(`type Mid[S any] struct{ Base[S] }`); a page embedding `Mid[UserContext]`
binds `Base[UserContext]`. An abstract page may be embedded by pointer
(`*Base[UserContext]`). The type argument itself must not be a pointer:
handlers take `state *S`, so `Base[*UserContext]` would ask for `**UserContext`
and is a parser error.

**Parameter: `stateID string`**. A stateful handler may take `stateID
string` alongside `state *T`. The parameter names the calling tab in message
broker subjects and is used to dispatch events targeted at that tab (see
`SubjectStateID`). `stateID` requires the handler to also take `state *T`.

The value is derived from the `Datapages-Instance` id with the server's HMAC
key and is stable for the tab's lifetime. It is not the id itself.
Subjects reach broker logs, stream storage, traces and metrics,
and presenting the id is what claims a tab's state.
Knowing a `stateID` only allows addressing events at that tab.

**Subject field: `SubjectStateID string`**. An event may declare a subject
field named exactly `SubjectStateID` of type `string`. At SSE stream
connect the server subscribes to `<base>.<state_id>`. Only the tab whose
state-id matches the dispatched value receives the event. Rules:

- `SubjectStateID` must be a singular `string`; `[]string` is rejected.
- `SubjectStateID` must not carry a `signal:"..."` tag.
- `SubjectStateID` must be the only subject field on the event — mixing
  with `SubjectUser`, other signal-scoped fields, or additional subject
  fields is rejected.
- Any page with an `OnXXX` for a `SubjectStateID` event must be stateful.

**Lifecycle**:

1. On `GET` of a stateful page, the server mints a fresh HMAC-signed
   identifier, sets it on the `Datapages-Instance` response header, and
   embeds it in the HTML so the client echoes it on subsequent requests.
2. The generated client shim attaches `Datapages-Instance` to every
   subsequent Datastar action request and to the SSE stream connect.
3. On `StreamOpen`, the server checks a `*T` (the page's bound state type)
   out of a `sync.Pool`, zero-resets it, and registers the `id -> slot`
   mapping. When `StreamOpen` declares `state`, the freshly zeroed pointer
   is passed to it.
4. For stateful action and `OnXXX` calls, the server verifies the header,
   looks up the slot, acquires its mutex, and invokes the user handler with
   `state`. A missing slot (for example, an action fired before `StreamOpen`
   completes) yields `409 Conflict` with `Datapages-Retry: reconnect`.
5. On `StreamClose`, the server arms a grace timer (default 30s). A
   reconnect with the same id inside the window reuses the slot as-is
   without reset. Otherwise the state is returned to the pool.

**Configuration**. `WithStateConfig` is required on `NewServer` when any
handler takes `state`:

```go
s := datapagesgen.NewServer(a, msgBroker,
    datapagesgen.WithStateConfig(datapagesgen.StateConfig{
        HMACKey:                hmacKey,          // required, non-empty
        GracePeriod:            30 * time.Second, // optional, default 30s
        MaxConcurrentInstances: 10_000,           // optional, default 10_000
    }),
)
```

`NewServer` panics when an app with stateful pages receives no `StateConfig`.

`HMACKey` signs the instance identifier. Key rotation or process restart
invalidates every live instance; connected clients recover by reloading the
page on the next rejected request.

`MaxConcurrentInstances` caps how many instances exist at the same time,
across all state types. A page load plus an SSE connect creates one, which
anyone who reaches the server can ask for. An instance keeps its place for
`GracePeriod` after its stream closes, which means a client that opens and
closes streams in a loop holds several at once. Size the cap by the memory
one state value costs.

A stream connect that would exceed the cap receives `503 Service Unavailable`
with `Retry-After`. Datastar retries the connect on its own.
A reconnect within `GracePeriod` reuses its instance and is never refused.
Actions of a tab that already holds an instance keep working.
Nothing in the app is notified, which makes the cap a limit to watch rather than
one to rely on.

**Sticky sessions on multi-server deployments**. State lives in process
memory, so each client's requests must land on the same backend. A load
balancer that hashes on a client-stable value — the session cookie or the
`Datapages-Instance` header — satisfies this. A round-robin balancer will
produce frequent `409` rejections followed by reloads.

**Security**. The instance id is embedded in the HTML response and held in
an in-memory JavaScript module variable — it is not stored in cookies,
`localStorage`, `sessionStorage`, or `IndexedDB`. This prevents another tab
on the same origin from observing another tab's id. The HMAC signature
rejects forged values.

#### Parameter: `signals struct {...}`

```go
signals struct {
	Foo string `json:"foo"`
	Bar int	`json:"bar"`
}
```

Provides the captured [Datastar signals](https://data-star.dev/guide/reactive_signals)
from the page. Signal fields map directly to Datastar signal names via their `json` tags.
Any named or anonymous struct is accepted,
but every field must have a json struct field tag.
Any JSON-serializable field type is supported, including nested structs, slices, and maps.

Nested structs map to nested Datastar signals using dot notation:

```go
signals struct {
	Form struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"form"`
}
```

This maps to Datastar signals `$form.name` and `$form.email`, initialized in
templates with `data-signals:form.name="''"` and `data-signals:form.email="''"`,
or as a single object `data-signals="{form: {name: '', email: ''}}"`.
The Go handler receives the nested values as `signals.Form.Name` and
`signals.Form.Email`.

#### Parameter: `path struct {...}`

```go
path struct {
	ID string `path:"id"`
}
```

Provides URL path parameters. These parameters must be defined in the URL comment.
Both named and anonymous struct types are accepted.

Each field must be exported with a `path:"..."` struct tag
where the tag value names the corresponding route variable
(e.g. `path:"id"` binds to `{id}` in the URL pattern).

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

or any type implementing `encoding.TextUnmarshaler`.
Values are parsed from their string representation in the URL.
If a value cannot be parsed into the target type, the request
returns HTTP 400 Bad Request.

#### Parameter: `query struct {...}`

```go
query struct {
	Filter string `query:"f"`
	Limit  int	`query:"l"`
}
```

Provides URL query parameters.
Both named and anonymous struct types are accepted.

Each field must be exported with a `query:"..."` struct tag
where the tag value names the query parameter key
(e.g. `query:"f"` reads from `?f=...`).

The same field types as [`path`](#parameter-path-struct-) are supported.

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

This parameter is allowed on `POSTXXX`, `PUTXXX`, `PATCHXXX`, and `DELETEXXX` page methods
handling [action requests](https://data-star.dev/reference/actions) and
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

Events can declare subject fields to build targeted NATS subjects.
Any field whose name starts with `Subject` is a subject field.
Subject fields must have type `string` or `[]string`.
Subject fields must be defined before any payload fields.

- `[]string` — multiple values; the Cartesian product of all `[]string` subject field
  values is computed at dispatch time and each combination produces a separate publish.
- `string` — single value; used directly in the subject without looping.

When an event is dispatched, subject field values are appended (in field definition
order) to the event's base subject, separated by dots.

For example, given `// EventNotify is "notify"`:

```go
dispatch(EventNotify{
	SubjectUser:   "u1",
	SubjectRoom:   []string{"r1", "r2"},
	SubjectDevice: "mobile",
})
```

The following subjects are dispatched:

```
notify.u1.r1.mobile
notify.u1.r2.mobile
```

The `string` field `SubjectUser` contributes one value, the `[]string` field
`SubjectRoom` expands into two combinations, and the `string` field
`SubjectDevice` contributes one value.

`SubjectUser` is a special subject field: its presence makes the event stream
require authentication (only authenticated users whose ID appears in the subject
will receive the event). It can be `string` (single user) or `[]string` (multiple users).

```go
// Multiple recipients:
type EventMessageSent struct {
	SubjectUser     []string `json:"subject_user"`
	SubjectChatRoom []string `json:"subject_chat_room"`

	Message string `json:"message"`
	Sender  string `json:"sender"`
}

// Single recipient:
type EventDirectMessage struct {
	SubjectUser string `json:"subject_user"`

	Text   string `json:"text"`
	Sender string `json:"sender"`
}
```

##### Signal-scoped subject fields

A subject field (other than `SubjectUser`) can carry a `signal:"<name>"` struct tag
to bind its value to a client-side Datastar signal. When a client connects to the
SSE stream, the server reads the signal value and uses it to build the subscription
subject. This enables per-instance event routing without authentication.

The signal name must start with a lowercase letter and contain only lowercase letters,
digits, underscores, or periods (e.g. `signal:"instance_id"`, `signal:"form.calc_id"`).

Signal-scoped subject fields must have type `string` (singular), since the signal
provides exactly one value. `SubjectUser` must not have a signal tag.

```go
// EventCalcUpdated is "calc.updated"
type EventCalcUpdated struct {
	SubjectInstance string `json:"subject_instance" signal:"instance_id"`

	Result float64 `json:"result"`
}
```

When the SSE stream handler runs, it reads `instance_id` from the client's signals,
validates it is non-empty, and subscribes to `calc.updated.<instance_id>`.

Signal-scoped events can be mixed with private (`SubjectUser`) events and plain
public events on the same page. They can also coexist with non-signal subject fields:

```go
// EventRoomUpdate is "room.update"
type EventRoomUpdate struct {
	SubjectUser []string `json:"subject_user"`
	SubjectRoom []string `json:"subject_chat_room"`
	SubjectCalc string   `signal:"calc_id"`

	Data string `json:"data"`
}
```

**Restrictions:**

- `SubjectUser` must not have a `signal:"..."` tag.
- No two subject fields may share the same `signal:"..."` tag value.
- Signal tag names must match `[a-z][a-z0-9_.]*`.

The following is invalid because a subject field appears after a payload field:

```go
type EventInvalid struct {
	Message     string   `json:"message"`
	SubjectUser []string // ERROR: subject field after payload field
}
```

**Example 1** — Given `// EventExample is "example"`:

```go
dispatch(EventExample{
	SubjectUser:                  []string{"u1", "u2"},
	SubjectAnything:              []string{"a1"},
	SubjectSomethingElseEntirely: []string{"s1", "s2", "s3"},
})
```

The following subjects are dispatched:

```
example.u1.a1.s1
example.u1.a1.s2
example.u1.a1.s3
example.u2.a1.s1
example.u2.a1.s2
example.u2.a1.s3
```

**Example 2** — Given `// EventExample2 is "example2"`:

```go
dispatch(EventExample2{
	SubjectAnything: []string{"a1", "a2"},
	SubjectUser:     []string{"u1", "u2"},
})
```

The following subjects are dispatched:

```
example2.a1.u1
example2.a1.u2
example2.a2.u1
example2.a2.u2
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
	SubjectUser     []string `json:"subject_user"`
	SubjectChatRoom []string `json:"subject_chat_room"`

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
		SubjectUser:     chatroom.ParticipantIDs,
		SubjectChatRoom: []string{signals.ChatRoom},
		Message:               signals.InputText,
		Sender:                session.UserID,
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

Regular error values that will be logged and followed by the error handling procedure
(500 Internal Server Error, or `RecoverError` if defined).

To return a specific HTTP status code instead of 500, use the sentinel errors from
the generated `httperr` package (`datapagesgen/httperr`):

```go
import "myapp/datapagesgen/httperr"

func (p PageIndex) POSTInput(...) error {
	if !valid {
		return httperr.BadRequest // 400
	}
	if !allowed {
		// 403, preserves original
		return fmt.Errorf("%w: %w", httperr.Forbidden, errOriginal)
	}
	if !found {
		return httperr.NotFound // 404
	}
	return nil
}
```

Available sentinels:
- `httperr.BadRequest` — 400
- `httperr.Forbidden` — 403
- `httperr.NotFound` — 404
- `httperr.Conflict` — 409

Return a sentinel directly, or wrap into the original error. When `RecoverError` is
defined, all errors (including httperr sentinels) are routed through it first. If
`RecoverError` is not defined or fails, the server responds with the appropriate HTTP
status code using the standard status text.

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

## Linting

`datapages lint` parses the application model and reports all errors without generating
code. It validates the same rules as `datapages gen`, making it useful for CI checks
and editor integration.

This includes all structural validations (missing types, invalid signatures,
path comments, event definitions, parameter types, etc.) as well as
template-specific checks on `.templ` files:

- **Hardcoded href**: a static `href="/path"` on an `<a>` tag or an expression
  `href={ "/path" }` / `href={ SomeConst }` whose value resolves to a disallowed URL.
  Use the generated `href` package instead (e.g. `href={ href.PageLogin() }`).
- **Unverifiable href expression**: an expression `href` on an `<a>` tag that contains
  a function call not from the `href` package (e.g. `href={ templ.SafeURL("/about") }`,
  `href={ loginHref() }`, `href={ fmt.Sprintf(...) }`). The linter cannot statically
  verify these, so they must use `href` package functions.
- **`href.External` with internal URL**: `href.External("/login")` wrapping a URL that
  looks app-internal.
- **Hardcoded action URLs**: using a hardcoded URL in a Datastar action context
  (e.g. `@post('/foo/bar')`) instead of the generated `action` package
  (e.g. `action={ action.POSTPageProfileSave() }`).
- **Form action attribute**: using a `<form action=...>` attribute (constant or
  expression). Datapages does not support plain HTML form submissions — use
  `data-on:submit` with Datastar actions instead.
- **Action context**: using an `action.XXX()` call in an attribute that is not a Datastar
  action context (`data-on:<event>`, `data-on-<plugin>`, `data-init`). For example,
  `action.POSTPageIndexSubmit()` in an `href` attribute.
- **Href context**: using an `href.XXX()` call in a Datastar action context
  (`data-on:<event>`, `data-on-<plugin>`, `data-init`). Href functions return URL paths,
  not Datastar action strings — use `action.XXX()` instead.
- **Action on wrong page**: using an action that belongs to a different page
  (e.g. `action.POSTPageProfileSave()` in a template rendered by `PageSettings`).
  App-level actions are allowed on any page.

### Allowed href values

The following href values are allowed without the `href` package and will not
produce lint errors:

- Fragment-only: `#section`, `#`
- Protocol-relative: `//cdn.example.com`
- Absolute with scheme: `https://...`, `mailto:...`, `tel:...`, `sms:...`, `ftp://...`
- `const` values that resolve to one of the above
- Backtick and double-quoted string literals that resolve to one of the above

The following are always disallowed:

- Root-relative paths: `/login`, `/static/style.css`
- Relative paths: `relative`, `./x`, `../x`
- Query-only: `?tab=settings`
- Empty string: `""`
- `javascript:` URLs

### Expression href validation

Expression href attributes (`href={ expr }`) are parsed as Go AST and validated:

1. **`href` package calls** (`href.PageXxx()`, `href.External(...)`, `href.Asset(...)`)
   are always allowed. For `href.External`, the first argument is checked if it is a
   string literal or constant — if it resolves to a disallowed URL, an error is reported.
2. **Any other function call** (e.g. `templ.SafeURL(...)`, `fmt.Sprintf(...)`,
   `loginHref()`) is rejected because the result cannot be statically verified.
3. **String literals and constants** are resolved and checked against the allowed/disallowed
   rules above.
4. **Bare identifiers** are resolved via `const` values. **Qualified
   identifiers** (e.g. `urls.LoginURL`) are resolved via exported constants from
   imported packages. Variables are not trusted (their value cannot be determined statically).

### Suppressing Lint Errors

Use `//datapages:nolint` in a templ file to suppress the next element's lint errors.
An optional trailing explanation comment is allowed:

```templ
//datapages:nolint
<a href="/legacy-path">Legacy</a>

//datapages:nolint // migrating to href package in #1234
<a href="/another-legacy">Another</a>
```

The directive applies to the immediately following non-whitespace sibling element.
It suppresses all attribute-level lint errors (hardcoded href, unverifiable href,
`href.External` with internal URL, hardcoded action, unverifiable action,
form action, action/href context mismatch) — it does **not** suppress
cross-page action ownership errors.

## Technical Limitations

- For now, with CSRF protection enabled, you will not be able to use plain HTML forms,
  since the CSRF token is auto-injected for Datastar `fetch` requests
  (where `Datastar-Request` header is `true`).
  You must use Datastar actions for any sort of server interactivity.

- The href linter cannot detect absolute links to your own domain
  (e.g. `href="https://mydomain.com/login"`). These bypass the linter because they
  have an explicit URL scheme, which the linter treats as external.
  Use the generated `href.PageXxx()` builders instead.
