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

The SSE action handlers `POSTXXX`, `DELETEXXX` and `PUTXXX` method parameter lists must
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
	sessionToken string, // Optional
	session Session, // Optional
	signals struct {...}, // Optional
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
- **Hardcoded action URLs**: using `action="/path"` instead of the generated `action`
  package (e.g. `action={ action.POSTPageProfileSave() }`).
- **Action context**: using an `action.XXX()` call in an attribute that is not a Datastar
  action context (`data-on:<event>`, `data-on-<plugin>`, `data-init`). For example,
  `action.POSTPageIndexSubmit()` in an `href` or plain HTML `action` attribute.
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
It suppresses href/action attribute errors only — it does **not** suppress
cross-page action ownership errors.

## Technical Limitations

- For now, with CSRF protection enabled, you will not be able to use plain HTML forms,
  since the CSRF token is auto-injected for Datastar `fetch` requests
  (where `Datastar-Request` header is `true`).
  You must use Datastar actions for any sort of server interactivity.

## Modules

Datapages ships pluggable modules with swappable implementations:

- [`SessionManager[S]`](modules/sessmanager/sessmanager.go)
  - [`natskv`](https://pkg.go.dev/github.com/romshark/datapages/modules/sessmanager/natskv) - NATS KV store with AES-128-GCM encrypted cookies
  - [`inmem`](https://pkg.go.dev/github.com/romshark/datapages/modules/sessmanager/inmem) - In-memory sessions (lost on restart; single-instance only)
- [`MessageBroker`](modules/msgbroker/msgbroker.go)
  - [`natsjs`](https://pkg.go.dev/github.com/romshark/datapages/modules/msgbroker/natsjs) - NATS JetStream backed message broker
  - [`inmem`](https://pkg.go.dev/github.com/romshark/datapages/modules/msgbroker/inmem) - In-memory fan-out message broker (single-instance only)
- [`TokenManager`](modules/csrf/csrf.go)
  - [`hmac`](https://pkg.go.dev/github.com/romshark/datapages/modules/csrf/hmac) - HMAC-SHA256 with BREACH-resistant masking
- [`TokenGenerator`](modules/sessmanager/sessmanager.go)
  - [`sesstokgen`](https://pkg.go.dev/github.com/romshark/datapages/modules/sesstokgen) - Cryptographically random session tokens (256-bit)
