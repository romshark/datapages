# Datapages Specification

## Source Package

Generator requires a path to an application source package
that must contain an `App` type and the `type PageIndex struct`.

`App`, page, abstract page and event types are declared without type parameters.
Generated code names them as written, hence a type parameter list
is rejected with "type parameters are not supported".
Any other generic type of the package is free to use them.

### App

The `App` type may optionally provide a method for custom global HTML `<head>` tags:

```go
func (*App) Head(
	r *http.Request,
	session datapages.Session[Data], // Optional
) datapages.Head {
	return globalHeadTags()
}
```

Both parameters are recognized by their type, so their names and order
are up to the application.

The `RecoverError` method allows you to recover from handler errors to improve UX by
giving better feedback over SSE. All action handler errors (including the datapages sentinels)
are routed through `RecoverError` when it is defined and the request is
a Datastar request. If `RecoverError` returns an error, the server falls back to
an HTTP error response using the appropriate status code.

```go
func (*App) RecoverError(
	err error,
	sse datapages.SSE,
) error {
	return sse.PatchElement(errorToast(err))
}
```

Both parameters are recognized by their type, so their names and order
are up to the application.

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

`XXX` is just a name placeholder.

A page type must declare exactly one named field, the exported `App *App`.
Any other named field is rejected. Embedded types are the exception and are
validated separately, see [Abstract Page Types](#abstract-page-types).
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
	closeSession datapages.CloseSession,
	redirect datapages.Redirect,
	err error,
) {
	return true, datapages.Redirect{URL: "/login"}, nil
}
```

The SSE action handlers `POSTXXX`, `PUTXXX`, `PATCHXXX` and `DELETEXXX` method parameter lists must
include `r *http.Request` and may include the following optional parameters:

```go
// POSTActionName is <path>
func (PageIndex) POSTActionName(
	r *http.Request,
	sse datapages.SSE, // Optional
	session datapages.Session[Data], // Optional
	path datapages.Path[struct{...}], // Required only when path variables are used in the URL
	query datapages.Query[struct{...}], // Optional
	signals datapages.Signals[struct{...}], // Optional
	somethingHappened datapages.Dispatcher[EventSomethingHappened], // Optional
	somethingElseHappened datapages.Dispatcher[EventSomethingElseHappened], // Optional
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
	session datapages.Session[Data], // Optional
	path datapages.Path[struct{...}], // Required only when path variables are used in the URL
	query datapages.Query[struct{...}], // Optional
	signals datapages.Signals[struct{...}], // Optional
	somethingHappened datapages.Dispatcher[EventSomethingHappened], // Optional
	somethingElseHappened datapages.Dispatcher[EventSomethingElseHappened], // Optional
) (
	body datapages.Component, // Optional
	head datapages.Head, // Optional
	redirect datapages.Redirect, // Optional
	newSession datapages.NewSession[Data], // Optional
	closeSession datapages.CloseSession, // Optional
	err error,
) {
	// ...
}
```

All `OnXXX` method parameter lists must include exactly one parameter
of an event type and `sse datapages.SSE`. Parameters may be in any order.
The event parameter is recognized by its type, its name is up to the application.
The `XXX` placeholder must always match the event name after the type's `Event` prefix.

```go
func (PageIndex) OnSomethingHappened(
	event EventSomethingHappened,
	sse datapages.SSE,
	streamID datapages.StreamID, // Optional
	session datapages.Session[Data], // Optional
) error {
	// ...
}
```

`StreamOpen` runs after the page SSE stream has been established and before
any event handler is invoked.
It returns `error`, or nothing at all. `error` is the only return value it may declare.
If it returns an error, stream setup stops immediately and
the stream is closed.
Datapages handles the error like any other Datastar request error: if `RecoverError`
is defined it is invoked, otherwise the server falls back to its internal-error path.
The `streamID` is a per-process unique identifier for the SSE stream instance.
The parameter is recognized by its `datapages.StreamID` type,
its name is up to the application.
Use it to correlate `StreamOpen` and `StreamClose` for the same stream.
It's intended for internal server-side bookkeeping only and
should not be exposed to clients.

```go
func (PageIndex) StreamOpen(
	r *http.Request,
	streamID datapages.StreamID,
	sse datapages.SSE, // Optional
	session datapages.Session[Data], // Optional
	signals datapages.Signals[struct{...}], // Optional
	somethingHappened datapages.Dispatcher[EventSomethingHappened], // Optional
	somethingElseHappened datapages.Dispatcher[EventSomethingElseHappened], // Optional
) error {
	// ...
}
```

`StreamClose` runs when the page SSE stream closes.
It returns `error`, or nothing at all. `error` is the only return value it may declare.
If it returns an error, datapages logs the error server-side.

```go
func (PageIndex) StreamClose(
	r *http.Request,
	streamID datapages.StreamID,
	session datapages.Session[Data], // Optional
	somethingHappened datapages.Dispatcher[EventSomethingHappened], // Optional
	somethingElseHappened datapages.Dispatcher[EventSomethingElseHappened], // Optional
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
	sse datapages.SSE,
	session Session,
) error {
	// ...
}

// PageFoo is /foo
type PageFoo struct {
	App *App
	Base
}

func (PageFoo) GET(r *http.Request) (body datapages.Component, err error) {
	return pageFoo(), nil
}

// PageBar is /bar
type PageBar struct {
	App *App
	Base
}

func (PageBar) GET(r *http.Request) (body datapages.Component, err error) {
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

func (p PageExample) GET(r *http.Request) (body datapages.Component, err error) {
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
	signals datapages.Signals[struct {
		InputValue string `json:"inputvalue"`
	}],
) (body datapages.Component, err error) {
	// Patch the page with a fat morph directly on action.
	data, err := p.App.fetchData(signals.Values.InputValue)
	if err != nil {
		return nil, err
	}
	return examplePageTemplate(data), nil
}

// POSTButtonClicked is /example/button-clicked
func (p PageExample) POSTButtonClicked(
	r *http.Request,
	session Session,
	somethingHappened datapages.Dispatcher[EventSomethingHappened],
) error {
	// Update everyone that something happened.
	return somethingHappened.Dispatch(EventSomethingHappened{WhoCausedIt: session.UserID()})
}

func (p PageExample) OnSomethingHappened(
	event EventSomethingHappened,
	sse datapages.SSE,
	session Session,
) error {
	// When something happens, patch the page.
	return sse.PatchElement(updateTemplate())
}
```

</details>

#### Parameter: `datapages.Signals[struct {...}]`

```go
signals datapages.Signals[struct {
	Foo string `json:"foo"`
	Bar int	`json:"bar"`
}]
```

Provides the captured [Datastar signals](https://data-star.dev/guide/reactive_signals)
from the page. The parameter is recognized by its `datapages.Signals` type,
its name is up to the application. The values are read from the `Values` field.
Signal fields map directly to Datastar signal names via their `json` tags.
Any named or anonymous struct is accepted as the type argument,
but every field must have a json struct field tag.
Any JSON-serializable field type is supported, including nested structs, slices, and maps.

Nested structs map to nested Datastar signals using dot notation:

```go
signals datapages.Signals[struct {
	Form struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"form"`
}]
```

This maps to Datastar signals `$form.name` and `$form.email`, initialized in
templates with `data-signals:form.name="''"` and `data-signals:form.email="''"`,
or as a single object `data-signals="{form: {name: '', email: ''}}"`.
The Go handler receives the nested values as `signals.Values.Form.Name` and
`signals.Values.Form.Email`.

#### Parameter: `datapages.Path[struct {...}]`

```go
path datapages.Path[struct {
	ID string `path:"id"`
}]
```

Provides URL path parameters. These parameters must be defined in the URL comment.
The parameter is recognized by its `datapages.Path` type, its name is up to the
application. The values are read from the `Values` field.
Both named and anonymous struct types are accepted as the type argument.

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

#### Parameter: `datapages.Query[struct {...}]`

```go
query datapages.Query[struct {
	Filter string `query:"f"`
	Limit  int	`query:"l"`
}]
```

Provides URL query parameters.
The parameter is recognized by its `datapages.Query` type, its name is up to the
application. The values are read from the `Values` field.
Both named and anonymous struct types are accepted as the type argument.

Each field must be exported with a `query:"..."` struct tag
where the tag value names the query parameter key
(e.g. `query:"f"` reads from `?f=...`).

The same field types as [`datapages.Path`](#parameter-datapagespathstruct-) are supported.

The `reflectsignal` struct field tag can be used to define what signal shall reflect
into the query parameter:

```go
signals datapages.Signals[struct {
	SelectedItem string `json:"selecteditem"`
}],
query datapages.Query[struct {
	SelectedItem string `query:"s" reflectsignal:"selecteditem"`
}]
```

The above example will automatically synchronize the query parameter `s` with the
signal `selecteditem`.

#### Parameter: `session datapages.Session[Data]`

```go
session datapages.Session[Data]
```

Provides authentication information from cookies.
The parameter is recognized by its `datapages.Session` type,
its name is up to the application.

The session is read-only: it exposes `UserID()`, `IsGuest()`, `Token()`,
`IssuedAt()`, `ExpiresAt()` and `Data()`, and returning [`newSession`](#return-value-newsession-datapagesnewsessiondata)
is the only way to change it. `Data` is the application payload,
use `struct{}` when the application keeps nothing else in the session.

The type is defined in [datapages.go](datapages.go), which documents each method
and is the source of truth. It is also rendered on
[pkg.go.dev](https://pkg.go.dev/github.com/romshark/datapages#Session).

A client whose `ExpiresAt()` has passed is treated as unauthenticated and its
session cookie is removed, the zero value never expires.

All handlers of an application must use the same `Data` type, since the server
holds a single session manager. Declaring an alias keeps the signatures short:

```go
type SessionData struct {
	Name string
}

type Session = datapages.Session[SessionData]

func (p PageIndex) GET(r *http.Request, session Session) (
	body datapages.Component, err error,
) {
	_ = session.Data().Name
	return pageIndex(), nil
}
```

#### Parameter: `sse datapages.SSE`

```go
sse datapages.SSE
```

This parameter is allowed on `POSTXXX`, `PUTXXX`, `PATCHXXX`, and `DELETEXXX` page methods
handling [action requests](https://data-star.dev/reference/actions),
on `OnXXX` event handler page methods, on `StreamOpen` and on `RecoverError`.
`StreamClose` does not accept it.
This gives you a handle to patch page elements, execute scripts, etc.

`datapages.SSE` (from `github.com/romshark/datapages`) hides the underlying
Datastar generator so handler signatures never depend on the datastar package
directly.

It provides `Context`, `PatchElement`, `PatchElementAt`, `RemoveElement`,
`ExecuteScript`, `PatchSignals`, `PatchSignalsIfMissing`, `Redirect` and
`Prefetch`, alongside the `PatchMode` constants.

`PatchElement(c)` morphs each rendered element into the element carrying its id.
`PatchElementAt(c, selector, mode)` names the target and how it is applied:

```go
return sse.PatchElementAt(toast(msg), "#toaster", datapages.PatchModeAppend)
```

Both methods refuse a selector containing `\r` or `\n` with
`datapages.ErrSelectorLineBreak`. The selector is written on one line of the
event, and a line break ends that line: without the check, a selector built
from client data could add events of its own.

The interface is defined in [datapages.go](datapages.go), which documents each
method and is the source of truth. It is also rendered on
[pkg.go.dev](https://pkg.go.dev/github.com/romshark/datapages#SSE).

#### Parameter: `datapages.Dispatcher[EventXXX]`

```go
xxx datapages.Dispatcher[EventXXX]
```

This parameter dispatches events, which can be handled by `OnXXX` page methods.
Its name is free, the type is what makes it a dispatcher.
`EventXXX` must be an event type declared in the application package.

```go
type Dispatcher[Event any] interface {
	Dispatch(event Event) error
	DispatchCtx(ctx context.Context, event Event) error
}
```

`Dispatch` publishes with the context of the handler that dispatches: the request
context in actions, and the request context without its cancelation in stream
hooks, which run while the stream is being torn down. `DispatchCtx` publishes with
the given context, which a handler needs only when it dispatches after
returning, from a goroutine that outlives it, or when the publish needs a
deadline of its own:

```go
ctx, cancel := context.WithTimeout(r.Context(), time.Second)
defer cancel()
return somethingHappened.DispatchCtx(ctx, EventSomethingHappened{})
```

An event type must use json struct field tags, and be strictly commented with
`// EventXXX is "xxx"` (where `"xxx"` is the NATS subject prefix):

```go
// EventExample is "example"
type EventExample struct {
	Information string `json:"info"`
}
```

Events can declare subject fields to build targeted NATS subjects.
A field is a subject field when its type is one of these:

| type | segment |
| ---- | ------- |
| `datapages.Subject` | a segment value |
| `datapages.SubjectUser` | the ID of the user the event is addressed to |

The field name is free, the type decides. Subject fields must be exported and
must be defined before any payload field.

Each subject field carries exactly one value, and one dispatch publishes to
exactly one subject. When an event is dispatched, subject field values are
appended (in field definition order) to the event's base subject, separated by
dots.

For example:

```go
// EventNotify is "notify"
type EventNotify struct {
	Recipient datapages.SubjectUser `json:"recipient"`
	Room      datapages.Subject     `json:"room"`
	Device    datapages.Subject     `json:"device"`

	Text string `json:"text"`
}

notify.Dispatch(EventNotify{
	Recipient: "u1",
	Room:      "r1",
	Device:    "mobile",
})
```

publishes to the subject `notify.u1.r1.mobile`.

A subject field value may carry any byte. A value is escaped on its way into
the subject, so `.`, `*`, `>` and whitespace name one segment rather than
ending it or matching more than themselves. An email address is a valid value.
Only an empty value is refused: it names no segment, and the dispatch returns
an error and publishes nothing.

The escaping is applied to the publish and the subscription alike,
and a value that needs none is used as it is. Escaped values reach the broker
percent-encoded, so `a.b` publishes to `notify.a%2Eb`.

To reach several rooms, or several users, dispatch once per value:

```go
for _, room := range rooms {
	err := notify.Dispatch(EventNotify{
		Recipient: "u1",
		Room:      datapages.Subject(room),
		Device:    "mobile",
		Text:      "hello",
	})
	if err != nil {
		return err
	}
}
```

The framework doesn't fan a single dispatch out over multiple values.
Each publish can fail on its own, and the handler decides whether to stop, continue,
or join the errors. Fanning out also means marshaling one payload per publish,
so each recipient receives only the values addressed to them.

A `datapages.SubjectUser` field makes the event stream require authentication:
only the client authenticated as that user receives the event. An application
dispatching such an event must define a Session type.

The user ID names the subject on both sides and is escaped there like any
other subject field value, which is why it can be an email address.
Only its length is bounded, since the whole subject has to fit what the broker accepts
on one line. Use [`datapages.ValidateUserID`](#validating-a-user-id) to check
an ID before a session carries it.

```go
// EventDirectMessage is "message.direct"
type EventDirectMessage struct {
	Recipient datapages.SubjectUser `json:"recipient"`

	Text   string `json:"text"`
	Sender string `json:"sender"`
}
```

##### Signal-scoped subject fields

A subject field that doesn't address users can carry a `signal:"<name>"` struct
tag to bind its value to a client-side Datastar signal. When a client connects to
the SSE stream, the server reads the signal value and uses it to build the
subscription subject. This enables per-instance event routing without
authentication.

The signal name must start with a lowercase letter and contain only lowercase letters,
digits, underscores, or periods (e.g. `signal:"instance_id"`, `signal:"form.calc_id"`).

`datapages.SubjectUser` must not have a signal tag: it's already bound to the
authenticated user.

```go
// EventCalcUpdated is "calc.updated"
type EventCalcUpdated struct {
	Instance datapages.Subject `json:"instance" signal:"instance_id"`

	Result float64 `json:"result"`
}
```

When the SSE stream handler runs, it reads `instance_id` from the client's signals,
escapes it, and subscribes to `calc.updated.<instance_id>`. An empty signal is
refused with 400. A wildcard needs no refusing: escaped, it is one literal
segment, so a client sending `*` subscribes to that value and to nothing else.

Signal-scoped events can be mixed with user-addressed events and plain public
events on the same page. They can also coexist with non-signal subject fields:

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
- Signal tag names must match `[a-z][a-z0-9_.]*`.
- Two events must not share a subject.
- An event with subject fields occupies every subject below its own. No other
  event may declare one there. `"notify"` with one subject field rules out
  `"notify.user"`, since a page cannot tell the two apart on arrival.

The following is invalid because a subject field appears after a payload field:

```go
// EventInvalid is "invalid"
type EventInvalid struct {
	Message   string `json:"message"`
	Recipient datapages.SubjectUser // ERROR: subject field after payload field
}
```

A field named like a subject field but not typed as one is rejected, since it reads as routing metadata but would silently become
payload:

```go
// EventInvalid2 is "invalid2"
type EventInvalid2 struct {
	SubjectUser string // ERROR: not typed as a subject field
}
```

One dispatcher publishes one event type. A handler that publishes several
declares one parameter per type, and it must not declare two for the same type:

```go
typeA datapages.Dispatcher[EventTypeA],
typeB datapages.Dispatcher[EventTypeB],
typeC datapages.Dispatcher[EventTypeC],
```

The events go out in the order the handler dispatches them.
Nothing is atomic across them: a failed publish neither undoes the ones before it nor
stops the ones after, which is why joining the errors is usually what you want:

```go
return errors.Join(
	typeA.Dispatch(EventTypeA{}),
	typeB.Dispatch(EventTypeB{}),
)
```

---

<details>
<summary>Example</summary>

```go
// EventMessageSent is "chat.sent"
type EventMessageSent struct {
	Recipient datapages.SubjectUser `json:"recipient"`
	ChatRoom  datapages.Subject     `json:"chat_room"`

	Message string `json:"message"`
	Sender  string `json:"sender"`
}

// PageChat is /chat
type PageChat struct { App *App }

func (PageChat) POSTSendMessage(
	r *http.Request,
	session Session,
	signals datapages.Signals[struct {
		InputText string `json:"inputtext"`
		ChatRoom  string `json:"chatroom"`
	}],
	messageSent datapages.Dispatcher[EventMessageSent],
) error {
	if !isUserAllowedToSendMessages(session.UserID()) {
		return errors.New("unauthorized")
	}
	if signals.Values.InputText == "" {
		return nil // No-op.
	}
	for _, participant := range chatroom.ParticipantIDs {
		err := messageSent.Dispatch(EventMessageSent{
			Recipient: datapages.SubjectUser(participant),
			ChatRoom:  datapages.Subject(signals.Values.ChatRoom),
			Message:   signals.Values.InputText,
			Sender:    session.UserID(),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (PageChat) OnMessageSent(
	event EventMessageSent,
	sse datapages.SSE,
	session Session,
) error {
	// Use sse to patch the new message into view.
}
```

</details>

##### Event delivery

Delivery is at most once, without replay. An event reaches the streams
subscribed to its subject at the time of the publish.

A stream misses an event when:

- its tab is in the background, where the stream is closed by default, see
  [`enableBackgroundStreaming`](#get-return-value-enablebackgroundstreaming-datapagesenablebackgroundstreaming);
- its subscription buffer is full. The buffer holds `ChanBuffer` messages, 16 by
  default, and the broker drops what does not fit instead of blocking the
  publisher. A stream consumes events one at a time: a slow `OnXXX` handler
  fills the buffer.

A missed event is not reported to the page. The UI stays stale until the next render,
which by default follows the tab becoming visible again,
see [`disableRefreshAfterHidden`](#get-return-value-disablerefreshafterhidden-datapagesdisablerefreshafterhidden).
A render must therefore carry the full state, not a delta.

Applications built with Prometheus metrics export drops as
`datapages_event_broker_deliveries_dropped_total`.

#### Return Value: `body datapages.Component`

Specifies the [Templ](https://templ.guide/) template to use for the contents of the page.

#### Return Value: `head datapages.Head`

Specifies the [Templ](https://templ.guide/) template to use for `<head>` tag of the page.
`datapages.Head` is `datapages.Component` under another name, which is what
tells the head apart from the body.
Return values are recognized by their type, their names are up to the application.

#### Return Value: `redirect datapages.Redirect`

```go
redirect datapages.Redirect
```

Redirects the client to `redirect.URL` with the status code `redirect.Status`.
The zero value is a no-op, the client stays on the current page.

```go
return datapages.Redirect{URL: href.PageIndex()}, nil
```

`Status` defaults to `302 Found`, any code that isn't a redirect status is
replaced by it. Requests issued by Datastar actions (carrying the header
`Datastar-Request: true`) can't follow an HTTP redirect:
they navigate client-side by assigning `window.location` and ignore `Status`.

The type is defined in [datapages.go](datapages.go),
which documents each field and is the source of truth. It is also rendered on
[pkg.go.dev](https://pkg.go.dev/github.com/romshark/datapages#Redirect).

#### Return Value: `newSession datapages.NewSession[Data]`

```go
newSession datapages.NewSession[Data]
```

Signs a client in. Adds response headers to set a session cookie if
`newSession.UserID` is not empty, otherwise no-op. Datapages generates the
session token and stamps the issuance time, the handler supplies `UserID`, an
optional `ExpiresAt` and `Data`. See
[datapages.go](datapages.go) and
[pkg.go.dev](https://pkg.go.dev/github.com/romshark/datapages#NewSession).

#### Validating a User ID

```go
func ValidateUserID(userID string) error
```

A user ID may carry any byte: what cannot stand in a subject is escaped.
Two things are still refused, and `datapages.ValidateUserID` reports both
without allocating:

- `datapages.ErrUserIDEmpty`, an empty ID names nobody.
- `datapages.ErrUserIDTooLong`, an ID whose escaped form is longer than
  `datapages.MaxUserIDEncodedLen`. A subject travels on one broker line,
  which bounds what fits.

Call it before returning a `newSession`,
to answer the request rather than fail the sign-in:

```go
if err := datapages.ValidateUserID(id); err != nil {
	return fmt.Errorf("%w: %w", datapages.ErrBadRequest, err)
}
```

`newSession` is checked against the same rule,
and a sign-in carrying an ID that breaks it fails.

#### Return Value: `closeSession datapages.CloseSession`

```go
closeSession datapages.CloseSession
```

Closes the session and removes any session cookie if `true`, otherwise no-op.

#### Return Value `error` or `err error`

Regular error values that will be logged and followed by the error handling procedure
(500 Internal Server Error, or `RecoverError` if defined).

To return a specific HTTP status code instead of 500, return one of the sentinel
errors from `github.com/romshark/datapages`:

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

Available sentinels:
- `datapages.ErrBadRequest` - 400
- `datapages.ErrForbidden` - 403
- `datapages.ErrNotFound` - 404
- `datapages.ErrConflict` - 409

Don't wrap more than one sentinel into a single error. If you do, the first of
`ErrBadRequest`, `ErrForbidden`, `ErrNotFound`, `ErrConflict` decides the status.

Return a sentinel directly, or wrap into the original error. When `RecoverError` is
defined, all errors (including the datapages sentinels) are routed through it first. If
`RecoverError` is not defined or fails, the server responds with the appropriate HTTP
status code using the standard status text.

#### `GET` Return Value: `enableBackgroundStreaming datapages.EnableBackgroundStreaming`

Can only be used for `GET` methods.

```go
enableBackgroundStreaming datapages.EnableBackgroundStreaming
```

By default, `OnXXX` event handlers can't deliver updates to background tabs.
If `true`, the SSE stream is always kept open. This prevents missed updates when the tab
is inactive, but increases battery and resource usage, especially on mobile devices.

This is equivalent to datastar's [`openWhenHidden`](https://data-star.dev/reference/actions)).

Events published while the stream is closed are lost, they are not replayed when
it opens again. See [Event delivery](#event-delivery).

`enableBackgroundStreaming=true` will automatically disable the auto-refresh after
hidden. If you want to prevent this, you have to explicitly add
`disableRefreshAfterHidden` to the return values and set it to `false`.

#### `GET` Return Value: `disableRefreshAfterHidden datapages.DisableRefreshAfterHidden`

Can only be used for `GET` methods.

```go
disableRefreshAfterHidden datapages.DisableRefreshAfterHidden
```

By default, Datapages refreshes the page when it becomes active again after being in the
background (for example, when switching back from another tab).
This is useful when `enableBackgroundStreaming` is `false`, since SSE events may be missed
while the tab is inactive and the page state can become stale.
You can disable this behavior by returning `disableRefreshAfterHidden=true`.
Doing so leaves the page showing whatever it last rendered, since nothing else
tells it that it missed an event. See [Event delivery](#event-delivery).

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
- **Unverifiable action expression**: an expression in a Datastar action context
  that is not a plain `action.XXX()` call (e.g.
  `data-on:click={ buildAction() }`, `data-on:click={ fmt.Sprintf(...) }`).
  The linter cannot statically verify these.
- **Action call with a prefix or suffix**: an `action.XXX()` call concatenated
  with another string (e.g. `data-on:click={ "$busy = true; " + action.POSTPageIndexSave() }`).
  Reported separately from the generic unverifiable case, with the concatenated
  side named. Use `action.WithBefore(expr)` and `action.WithAfter(expr)`
  instead, which put the expression inside the generated action string.
- **Form action attribute**: using a `<form action=...>` attribute (constant or
  expression). Datapages does not support plain HTML form submissions — use
  `data-on:submit` with Datastar actions instead.
- **Action context**: using an `action.XXX()` call in an attribute that is not a
  Datastar action context.
  For example, `action.POSTPageIndexSubmit()` in an `href` attribute.
- **Href context**: using an `href.XXX()` call in a Datastar action context.
  Href functions return URL paths, not Datastar action strings,
  use `action.XXX()` instead.
- **Action on wrong page**: using an action that belongs to a different page
  (e.g. `action.POSTPageProfileSave()` in a template rendered by `PageSettings`).
  App-level actions are allowed on any page.

These attributes are the Datastar action contexts, and no others:

- `data-on:<event>`, any DOM event.
- `data-on-intersect`, `data-on-interval` and `data-on-signal-patch`,
  the plugin events the linter knows.
- `data-init`.

The plugin and `data-init` attributes may carry Datastar modifiers
(`data-on-intersect.once`, `data-on-interval__duration.500ms`).

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

- For now, an application that declares a session type cannot use plain HTML forms.
  CSRF protection is on for it and the CSRF token is auto-injected only for
  Datastar `fetch` requests (where the `Datastar-Request` header is `true`).
  You must use Datastar actions for any sort of server interactivity.

- The href linter cannot detect absolute links to your own domain
  (e.g. `href="https://mydomain.com/login"`). These bypass the linter because they
  have an explicit URL scheme, which the linter treats as external.
  Use the generated `href.PageXxx()` builders instead.
