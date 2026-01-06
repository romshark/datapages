# Datapages (Proof of Concept)

A [Templ](https://templ.guide) + Go + [Datastar](https://data-star.dev) server rendered
web app framework prototype (currently referred to as "Datapages" and "dp" as CLI tool.) 
that is supposed to work as a code generator and code linter.

- Run `dp init` which creates an application template in the current folder and
  prompts for preset configs (such as whether to use
  [TailwindCSS](https://tailwindcss.com/), etc.).
- Then you run `dp dev` which starts a development mode that begins listening for file
  changes and automatically regenerates the generated app bundle while also
  reloading the browser tabs similar to [Templiér](https://github.com/romshark/templier).
- You may also use `dp gen` to report whether there's any logical errors in the code
  or use this during CI/CD to lint the code and check for whether checked in generated
  code was regenerated prior to committing.
- You write the business logic in the `app package` in the form expected by datapages,
  the rest (routing, auth, and other boilerplate) is generated in a neighboring package.

Being primarily a code generator, Datapages allows your application code to take
full advantage of Go's strong static typing and achieve a higher level of efficiency
and performance.

This repository presents a demo application resembling an online classifieds marketplace.

🚧👷‍♂️ Due to several design iterations and frequent changes,
so far I've been playing the role of the code generator and the contents
of `_gen.go` files are hand-written according to the logic
the code generator would follow.

## Source Package

Generator requires a path to an application source package
that must contain an `App` type and the `type PageIndex struct`.

### App

The `App` type may optionally provide a method for custom global HTML `<head>` tags:

```go
func (*App) Head(
    r *http.Request,
    session SessionJWT,
) (templ.Component, error) {
    return globalHeadTags(session.UserID), error
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

Page types `Page500` and `Page404` are optional special error pages for the response
codes `500` and `404` respectively. Otherwise datapages will use its own defaults.

Handler method parameters and their order are defined and enforced by datapages.
Using unsupported parameter names and types will result in generator errors.

The `GET` method parameter lists must always start with `r *http.Request`,
followed by other parameters:

```go
func (PageIndex) GET(
    r *http.Request,
    session SessionJWT, // Optional
    path struct{...}, // Required only when path variables are used in the URL
    query struct{...}, // Optional
    signals struct {...}, // Optional
    setSessionJWT func(userID string, expire time.Time, claims map[string]any), // Optional
    dispatch(
        EventSomethingHappened,
        EventSomethingElseHappened,
        //...
    ) error // Optional
    err error,
) (
    body templ.Component,
    head templ.Component, // Optional
    redirect Redirect, // Optional
    err error
) {
    // ...
} 
```

The action handlers `POSTXXX`, `DELETEXXX` and `PUTXXX` method parameter lists must
always start with `r *http.Request`, followed by other parameters:

```go
func (PageIndex) POSTActionName(
    r *http.Request,
    sse *datastar.ServerSentEventGenerator, // Optional
    session SessionJWT, // Optional
    path struct{...}, // Required only when path variables are used in the URL
    query struct{...}, // Optional
    signals struct {...}, // Optional
    setSessionJWT func(userID string, expire time.Time, claims map[string]any), // Optional
    dispatch(
        EventSomethingHappened,
        EventSomethingElseHappened,
        //...
    ) error // Optional
    err error,
) error {
    // ...
}
```

All `OnXXX` method parameter lists must always start with
`sse *datastar.ServerSentEventGenerator`, followed by other parameters and must end
with the `event EventXXX` parameter specifying the event to be handled.
The `XXX` placeholder must always match the event name after the type's `Event` prefix.

```go
func (PageIndex) OnSomethingHappened(
	sse *datastar.ServerSentEventGenerator,
	event EventSomethingHappened,
	session SessionJWT, // Optional
) error {
	// ...
}
```

#### Abstract Page Types

Abstract page types can be embedded in page types to share functionality across pages:

```go
type Base struct{ App *App }

func (Base) OnSomethingHappened(
	sse *datastar.ServerSentEventGenerator,
	session SessionJWT,
	event EventSomethingHappened,
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
    session SessionJWT,
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
    session SessionJWT,
    dispatch(EventSomethingHappened) error,
) error {
    // Update everyone that something happened.
    return dispatch(EventSomethingHappened{WhoCausedIt: session.UserID})
}

func (p PageExample) OnSomethingHappened(
    sse *datastar.ServerSentEventGenerator,
	event EventSomethingHappened,
	session SessionJWT,
) error {
    // When something happens, patch the page.
    return sse.PatchElementTempl(updateTemplate())
}
```

</details>

#### 🧩 Parameter `signals struct {...}`

```go
signals struct {
    Foo string `json:"foo"`
    Bar int    `json:"bar"`
}
```

Provides the captured [Datastar signals](https://data-star.dev/guide/reactive_signals)
from the page.
Any named or anonymous struct is accepted,
but every field must have a json struct field tag.

#### 🧩 Parameter `path struct {...}`

```go
path struct {
	ID string `path:"id"`
}
```

Provides URL path parameters. These parameters must be defined in the URL comment.

#### 🧩 Parameter `query struct {...}`

```go
query struct {
	Filter string `query:"f"`
    Limit  int    `query:"l"`
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

#### 🧩 Parameter `session SessionJWT`

```go
session SessionJWT
```

Provides [JWT](https://www.jwt.io/)-based authentication information from cookies.

If used, must be defined at the source package level as:

```go
type SessionJWT struct {
    UserID     string    `json:"sub"` // Required.
    IssuedAt   time.Time `json:"iat"` // Optional.
    Expiration time.Time `json:"exp"` // Optional.
}
```

#### 🧩 Parameter `setSessionJWT func(...)`

```go
setSessionJWT func(userID string, expire time.Time, claims map[string]any)
```

Provides a function for setting a JWT-based session cookie.

#### 🧩 Parameter `sse *datastar.ServerSentEventGenerator`

```go
sse *datastar.ServerSentEventGenerator
```

This parameter is allowed only on `POSTXXX` page methods handling
`POST` [action requests](https://data-star.dev/reference/actions) and
`OnXXX` event handler page methods.
This gives you a handle to patch page elements, execute scripts, etc.

#### 🧩 Parameter `dispatch func(...) error`

```go
dispatch func(EventType) error
```

This parameter provides a function for dispatching system wide events and
only accepts `EventXXX` types as parameters. These events can be handled
by `OnXXX` page methods.

May provide multiple event types which are dispatched in the order of definition:

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
    session SessionJWT,
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
        Message:       signals.InputText,
        Sender:        session.UserID,
    })
}

func (PageChat) OnMessageSent(
    sse *datastar.ServerSentEventGenerator,
    session SessionJWT,
    e EventMessageSent,
) error {
    // Use sse to patch the new message into view.
}
```

</details>

#### 🧩 Return Value `body templ.Component`

Specifies the [Templ](https://templ.guide/) template to use for the contents of the page.

#### 🧩 Return Value `head templ.Component`

Specifies the [Templ](https://templ.guide/) template to use for `<head>` tag of the page.

#### 🧩 Return Value `redirect Redirect`

Allows for redirecting to different URLs.

The `Redirect` type must be defined on the source package level as:

```go
package app

type Redirect struct {
	Target string
	Status int
}
```

#### 🧩 Return Value `error` or `err error`

Regular error values that will be logged and followed by the error handling procedure.
