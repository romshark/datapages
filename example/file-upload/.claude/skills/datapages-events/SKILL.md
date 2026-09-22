---
name: datapages-events
description: >-
  Datapages real-time events: event types and subjects, dispatchers, On
  handlers, per-user and signal-bound subject fields,
  and the StreamOpen and StreamClose hooks. Activate when adding events,
  subscriptions or stream hooks to a Datapages app.
---

# Events

Read `datapages` first for the build loop, hard rules and naming conventions. Read `datapages-state` when the page uses per-tab server state.

A handler dispatches an event type. Datapages delivers it over SSE to subscribed pages. Declare the type in the app package or an imported package. Types in imported packages can be shared by multiple apps.

```go
// EventMessageSent is "messaging.sent"
type EventMessageSent struct {
	Message string `json:"message"`
}
```

## Dispatch

Add a `datapages.Dispatcher[EventX]` parameter, one per event type.

```go
func (PageChat) POSTSend(
	r *http.Request,
	messageSent datapages.Dispatcher[EventMessageSent],
) error {
	return messageSent.Dispatch(EventMessageSent{Message: "hello"})
}
```

`Dispatch` uses the handler's context. Use `DispatchCtx(ctx, ev)` when the publish can outlive the handler or needs a separate deadline. An action can use `r.Context()`. An `On` handler has no request and can use `sse.Context()`. Publishes through multiple dispatchers are not atomic. Combine their errors with `errors.Join`.

## Handle

`On` + the event name, on a page or on an embedded type.

```go
func (PageChat) OnMessageSent(
	event EventMessageSent,
	sse datapages.SSE,
	streamID datapages.StreamID, // optional
	session Session,             // optional
) error {
	return sse.PatchElement(messageComponent(event.Message))
}
```

The event and `sse` are required. `On` handlers cannot accept signals. Put required data on the event, or use `datapages.State[T]` for per-tab data.

## Subjects

A subject field must have the exact type `datapages.Subject` or `datapages.SubjectUser`. The generator rejects a defined type such as `type Recipient datapages.SubjectUser`. Subject fields must be exported and must precede payload fields. Their values extend the base subject in field order, separated by periods. `EventDirectMessage{Recipient: "u1"}` publishes to `messaging.direct.u1`. A subject field must not be tagged `json:"-"`: the payload carries its value, the subject only routes the event.

```go
// EventDirectMessage is "messaging.direct"
type EventDirectMessage struct {
	Recipient datapages.SubjectUser `json:"recipient"`

	Content string `json:"content"`
}
```

- One dispatch targets one subject. To notify several recipients, dispatch once per recipient. Each publish can fail independently.
- Two events must not share a subject. An event with subject fields also claims every subject below its base subject. For example, `"chat"` with fields conflicts with `"chat.msg"`.
- A subject value may contain any byte. Datapages escapes bytes that are not valid in a subject, so an email address works. An empty value fails the dispatch and publishes nothing.
- `SubjectUser` sends only to the client authenticated as that user and requires a session type. Validate an ID with `datapages.ValidateUserID` before returning it in a new session.
- A `signal:"name"` tag on a `datapages.Subject` field gets the value from the client signal. An empty value returns 400. Datapages escapes `*` as a literal segment, not a wildcard. The tag is a period-separated signal path. Each segment must match `[A-Za-z_][A-Za-z0-9_]*` and must not contain `__`. Two fields must not use the same signal. A `SubjectUser` field cannot have this tag because it is already bound to the authenticated user.

## Delivery

Delivery is at most once and has no replay. Only streams subscribed at publish time receive an event. A hidden tab closes its stream by default and misses events. The default broker subscription buffer holds 16 messages. When it is full, the broker drops new messages without blocking the publisher or telling the page. Render full current state after a refresh so a missed append or prepend event cannot leave the page incomplete.

## Stream hooks

`StreamOpen` and `StreamClose` run when a page's SSE stream opens and closes. Both require `r *http.Request` and at least one of `streamID datapages.StreamID` or `state datapages.State[T]`. They may return `error` or no value.

```go
func (PageIndex) StreamOpen(
	r *http.Request,
	streamID datapages.StreamID,
	sse datapages.SSE,                            // optional
	session Session,                              // optional
	signals datapages.Signals[struct{ /*...*/ }], // optional
	ping datapages.Dispatcher[EventPing],         // optional
) error
```

`StreamClose` may also take `session`, dispatchers and `stateID string` with `State[T]`. It cannot take `sse` or `signals`. A `StreamOpen` error stops setup, closes the stream and calls `RecoverError`. Datapages only logs a `StreamClose` error.

Use `State[T]` for filters, sort order and other per-tab values. Use stream hooks to acquire and release resources for a stream. `streamID` is internal and must not be sent to a client.

Datapages serves the stream at the page route plus `_$/`. A page with both public and user-addressed events has a second stream at `_$/anon/`.

Share a handler across pages by embedding: see `datapages-pages`.
