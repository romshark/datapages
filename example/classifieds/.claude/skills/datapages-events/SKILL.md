---
name: datapages-events
description: >-
  Datapages real-time events: event types and subjects, dispatchers, On
  handlers, per-user and signal-bound subject fields,
  and the StreamOpen and StreamClose hooks.
---

# Events

An event type in the app package, dispatched from a handler, delivered over SSE
to every subscribed page.

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

`Dispatch` publishes with the handler's context. Use `DispatchCtx(ctx, ev)`
when the publish outlives the handler or needs its own deadline. Nothing is
atomic across dispatchers, so `errors.Join` their results.

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

The event and `sse` are required. `On` handlers accept **no** signals: put what
the handler needs on the event type and fill it in the action that dispatches.
Use `streamID` to read per-tab state registered in `StreamOpen`.

## Subjects

A field typed `datapages.Subject` or `datapages.SubjectUser` extends the base
subject. Subject fields are exported, come before any payload field, and are
appended in field order, separated by dots. `EventNotify` below with
`Recipient: "u1"` publishes to `notify.u1`.

```go
// EventDirectMessage is "messaging.direct"
type EventDirectMessage struct {
	Recipient datapages.SubjectUser `json:"recipient"`

	Content string `json:"content"`
}
```

- One dispatch reaches one subject. Loop and dispatch per recipient: there is
  no fan-out, and every publish fails on its own.
- No two events may share a subject, and an event with subject fields claims
  every subject below its own: `"chat"` with fields rules out `"chat.msg"`.
- A value may carry any byte, what cannot stand in a subject is escaped, so an
  email address works. An empty value fails the dispatch and publishes nothing.
- `SubjectUser` delivers only to the client authenticated as that user and
  requires a session type. Check an ID with `datapages.ValidateUserID` before
  returning it as a new session.
- A `signal:"name"` tag on a `datapages.Subject` field makes the client supply
  the value from that signal. An empty one is rejected with 400. `*` is escaped
  into one literal segment, not a wildcard. The tag value is
  `[a-z][a-z0-9_.]*`, no two fields may share one, and a `SubjectUser` field
  may not carry one: it is bound to the authenticated user already.

## Stream hooks

`StreamOpen` and `StreamClose` run when a page's SSE stream opens and closes.
Both require `r *http.Request` and `streamID datapages.StreamID`, and return
`error` or nothing at all.

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

`StreamClose` takes `session` and dispatchers only: no `sse`, no `signals`.
An error from `StreamOpen` stops setup, closes the stream and goes through
`RecoverError`; one from `StreamClose` is logged.

Use them for per-tab server state keyed by `streamID` (filters, sort order, the
item in view) that event handlers read back, for per-stream resources acquired
and released in pairs, and for an HMAC of the `streamID` patched to the client
as a signal and verified in actions. The `streamID` itself is internal
bookkeeping and must never reach a client.

The stream is served at the page route plus `_$/`, and a page mixing public
with user-addressed events serves a second one at `_$/anon/`.

Share a handler across pages by embedding: see `datapages-pages`.
