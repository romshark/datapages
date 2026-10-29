# Events

An event sends a change from its handler to every open page that shows the affected data. Subjects can limit delivery to a user, signal value or browser tab. A message broker carries events across server processes.

## Why events

An action that takes `datapages.SSE` patches only the tab that sent the request. To update other tabs, users or server processes, dispatch an event:

```text
tab A                         server                          broker
  POST /add/ ------------->   POSTAdd
                                Notes.Add(...)
                                Dispatch(EventNotesChanged) ---> "notes.changed"
                                                                    |
tab A stream <--- patch ----  OnNotesChanged <----------------------+
tab B stream <--- patch ----  OnNotesChanged <----------------------+
```

Every tab of a page that handles events holds 1 SSE connection at the page path plus `_$/`. Datapages opens the stream and subscribes it to the subjects handled by that page. An arriving event calls the page's `OnXXX` handler, which sends patches through the tab's stream.

## Declare an event

An event is a struct type whose name starts with `Event`. The first line of its doc comment names its subject in quotes:

```go title="app/app.go"
// EventNotesChanged is "notes.changed"
type EventNotesChanged struct{}
```

Datapages publishes the event to the broker under this subject. Fields carry a payload, which Datapages encodes as JSON, and each payload field needs a `json` tag. 2 event types must not use the same subject.

## Dispatch an event

Add a `datapages.Dispatcher[EventXXX]` parameter to the handler that changes the data, 1 per event type:

```go title="app/app.go"
// POSTAdd is /add
func (p PageIndex) POSTAdd(
	r *http.Request,
	signals datapages.Signals[struct {
		Text string `json:"text"`
	}],
	notesChanged datapages.Dispatcher[EventNotesChanged],
) error {
	text := strings.TrimSpace(signals.Values.Text)
	if text == "" {
		return datapages.ErrBadRequest
	}
	if err := p.App.Notes.Add(r.Context(), text); err != nil {
		return err
	}
	return notesChanged.Dispatch(EventNotesChanged{})
}
```

`Dispatch` publishes with the request context and returns the error of the broker. Actions, page `GET` methods and stream hooks may take dispatchers. Event handlers and `GETXXX` file actions may not.

A handler with several dispatchers publishes in call order. A failed publish neither undoes the earlier ones nor stops the later ones. Collect the errors with `errors.Join`. To publish after the handler returned, as from a goroutine, call `DispatchCtx` with a context of its own, such as `context.WithoutCancel(r.Context())` with a deadline.

## Handle an event

A page handles `EventXXX` with a method named `On` plus the event name without `Event`. It takes the event and a `datapages.SSE`:

```go title="app/app.go"
func (p PageIndex) OnNotesChanged(
	event EventNotesChanged, sse datapages.SSE,
) error {
	return sse.PatchElement(noteList(p.App.Notes.List()))
}
```

```templ title="app/app.templ"
templ noteList(list []string) {
	<ul id="notes">
		for _, n := range list {
			<li>{ n }</li>
		}
	</ul>
}
```

`PatchElement` morphs the rendered list into the element with the ID `notes` in every tab of the page, the tab that added the note included. Most changes follow the same 5 steps:

1. Validate the command.
2. Change the authoritative data.
3. Dispatch an event.
4. Re-read the data in each `OnXXX` handler.
5. Render a complete current fragment.

Treat an event as a notification, not as the source of truth. A tab can miss events, as [Delivery](#delivery) describes. A handler that renders the whole list from the store corrects the page with the next event that reaches it. A handler that appends the text of the event to the page leaves a gap for every event it missed. [Performance](performance.md#re-render-whole-fragments) explains why a whole fragment costs little on a compressed stream.

An event handler may also take `session` for the visitor of the tab, `datapages.StreamID` and `datapages.State[T]`. It cannot take signals, because a tab sends its signals once, when its stream connects. Put what the handler needs in the event, in the store, or in [per-tab state](state.md).

Datapages logs the error an `OnXXX` handler returns and keeps the stream open. It does not pass that error to `RecoverError`, so send the feedback with `sse` before returning. A panic reaches `RecoverError` as a `datapages.PanicError` and ends the stream, which the page then reconnects.

## Address events with subjects

An event without subject fields reaches every stream of every page that handles it. Subject fields narrow the delivery. Their values follow the subject of the event in field order, separated by periods. `EventMessageReceived{Recipient: "u1"}` with the subject `"messages.received"` publishes to `messages.received.u1`. What a page subscribes to depends on the type of the field:

| Field | A page that handles the event subscribes to |
| ----- | ------------------------------------------- |
| `datapages.SubjectUser` | the user ID of the session the stream belongs to |
| `datapages.Subject` with a `signal:"name"` tag | the value of the signal `name` when the stream connects |
| `datapages.Subject` without a tag | every value |
| `datapages.SubjectStateID` | the tab's state ID, see [Per-tab state](state.md) |

Declare subject fields before the payload fields, export them, use exactly these types and do not tag them `json:"-"`. 1 dispatch publishes to 1 subject. To reach several values, dispatch once per value. Datapages escapes `.`, `*`, `>`, `%` and whitespace in a value into 1 literal segment, which lets an email address serve as a user ID and makes a `*` that a client sends match only the value `*`. An empty value fails the dispatch. An event with subject fields also claims every subject below its own, so `"notes"` with a subject field conflicts with `"notes.changed"`.

### Address a user

A `datapages.SubjectUser` field delivers the event to the streams of 1 signed-in user:

```go title="app/app.go"
// EventMessageReceived is "messages.received"
type EventMessageReceived struct {
	Recipient datapages.SubjectUser `json:"recipient"`

	From string `json:"from"`
}

// POSTSend is /inbox/send
func (p PageInbox) POSTSend(
	r *http.Request,
	session Session,
	signals datapages.Signals[struct {
		To   []string `json:"to"`
		Text string   `json:"text"`
	}],
	received datapages.Dispatcher[EventMessageReceived],
) error {
	if session.IsGuest() {
		return datapages.ErrForbidden
	}
	var errs []error
	for _, to := range signals.Values.To {
		err := p.App.Inbox.Deliver(r.Context(), session.UserID(), to, signals.Values.Text)
		if err == nil {
			err = received.Dispatch(EventMessageReceived{
				Recipient: datapages.SubjectUser(to),
				From:      session.UserID(),
			})
		}
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (p PageInbox) OnMessageReceived(
	event EventMessageReceived, sse datapages.SSE, session Session,
) error {
	return sse.PatchElement(inbox(p.App.Inbox.For(session.UserID())))
}
```

The application needs a session type. See [Sessions and auth](sessions.md). An event has at most 1 `SubjectUser` field, and the field takes no `signal` tag. A page whose events are all user-addressed and which holds no per-tab state opens its stream only for a signed-in visitor. A page that also handles public events or holds per-tab state serves signed-out visitors a second stream at `_$/anon/`, which carries no user-addressed events.

### Scope by a signal

A `signal` tag binds a `datapages.Subject` field to a Datastar signal. Each tab subscribes to the value it holds when its stream connects:

```go title="app/app.go"
// EventRoomMessage is "rooms.message"
type EventRoomMessage struct {
	Room datapages.Subject `json:"room" signal:"room"`

	Text string `json:"text"`
}
```

```templ title="app/app.templ"
templ pageRoom(room string, msgs []string) {
	<main data-signals:room={ templ.JSONString(room) }>
		<h1>{ room }</h1>
		@roomMessages(msgs)
	</main>
}
```

A dispatch with `Room: "red"` reaches the tabs of room `red` and no other. A stream that connects without the signal, or with an empty value, receives 400. The tag names a signal path, such as `signal:"chat.room"` for a nested signal. [Signal-scoped subject fields](../SPECIFICATION.md#signal-scoped-subject-fields) lists the restrictions.

## Delivery

Delivery is at most once and has no replay. Only the streams subscribed when the event is published receive it. A tab misses events in these cases:

- Datastar closes the stream of a hidden tab. Datapages reloads the page when the tab becomes visible again, which renders the data it missed. A `GET` that returns `datapages.EnableBackgroundStreaming(true)` keeps the stream open instead.
- Each subscription of the in-memory broker buffers 16 messages, `messaging.DefaultBrokerChanBuffer`, and each of the NATS broker up to 32. When a page handles events slower than they arrive, the broker drops what does not fit. It does not block the publisher and does not tell the page. With Prometheus, `datapages_event_broker_deliveries_dropped_total` counts the drops.
- A stream that reconnects after a network failure or a deployment misses what was published in between.

## Stream hooks

`StreamOpen` runs when a tab's stream opens and `StreamClose` when it closes. Use them to acquire and release what a stream holds. This stream embed counts the open tabs and tells every tab the count:

```go title="app/app.go"
// EventPresenceChanged is "presence.changed"
type EventPresenceChanged struct{}

// Presence is a stream embed. A page that embeds it shows how many tabs
// have a page of this process open.
type Presence struct{ App *App }

func (p Presence) StreamOpen(
	r *http.Request,
	streamID datapages.StreamID,
	presenceChanged datapages.Dispatcher[EventPresenceChanged],
) error {
	p.App.presence.add(streamID)
	if err := presenceChanged.Dispatch(EventPresenceChanged{}); err != nil {
		// StreamClose does not run after a failed StreamOpen.
		p.App.presence.remove(streamID)
		return err
	}
	return nil
}

func (p Presence) StreamClose(
	r *http.Request,
	streamID datapages.StreamID,
	presenceChanged datapages.Dispatcher[EventPresenceChanged],
) error {
	p.App.presence.remove(streamID)
	return presenceChanged.Dispatch(EventPresenceChanged{})
}

func (p Presence) OnPresenceChanged(
	event EventPresenceChanged, sse datapages.SSE,
) error {
	return sse.PatchElement(presenceCount(p.App.presence.count()))
}
```

- Both hooks take `r *http.Request` and a `datapages.StreamID`, a `datapages.State[T]` or both. The stream ID identifies the stream within the process and pairs the 2 calls. Keep it on the server.
- `StreamOpen` may also take `sse`, `session`, `signals` and dispatchers. `StreamClose` may take `session` and dispatchers.
- `StreamOpen` runs before any event handler of the stream. If it returns an error or panics, `RecoverError` receives the error when the app defines it, the stream ends, the page reconnects after 5 seconds, and `StreamClose` does not run. Release what it acquired before returning the error.
- `StreamClose` runs after the last event handler of the stream. Datapages logs its error. Its dispatchers publish with the request context without its cancelation, which lets them publish after the client is gone.

## Share handlers between pages

A struct in the app package whose name does not start with `Page` and which declares `App *App` is a stream embed. A page that embeds it gets its event handlers and stream hooks:

```go title="app/app.go"
// PageIndex is /
type PageIndex struct {
	App *App
	Presence
}
```

A handler that the page declares itself replaces the embedded one. A stream embed cannot declare `GET` or actions, and 2 types that a page embeds must not handle the same event. [Stream Embeds](../SPECIFICATION.md#stream-embeds) has the exact rules.

## Events across server processes

The broker passed to `datapages.NewServer` carries the events. The in-memory broker of `modules/messaging/inmem` connects the streams of 1 process. The broker of `modules/messaging/natscore` publishes through NATS, so an event dispatched on 1 process reaches the streams of every process connected to the same NATS server. The `cmd/server/main.go` that `datapages init` writes creates it with `natscore.New(conn, natscore.Config{})`. [Deployment](deployment.md#run-several-processes) covers running several processes.

## Events declared outside the app package

An event type may live in a package that the app package imports, directly or not. Several applications of 1 module then share it. Each dispatches and handles the same type under the same subject:

```go title="events/events.go"
// Package events provides the event types that the applications of this module share.
package events

// EventAnnouncement is "announcement"
type EventAnnouncement struct {
	Text string `json:"text"`
}
```

```go title="app/app.go"
func (PageNews) OnAnnouncement(
	event events.EventAnnouncement, sse datapages.SSE,
) error {
	return sse.PatchElement(announcement(event.Text))
}
```

Applications that share a user-addressed event must use the same user IDs. [Multiple applications](multiple-applications.md) shows the module layout.

## See also

- [`Dispatcher[EventXXX]`](../SPECIFICATION.md#parameter-datapagesdispatchereventxxx): declaring events, dispatchers and subject fields.
- [Event delivery](../SPECIFICATION.md#event-delivery): when a stream misses events.
- [Pages](../SPECIFICATION.md#pages): the signatures of `OnXXX`, `StreamOpen` and `StreamClose`.
- [Per-tab state](state.md): values an event handler reads for 1 tab, and events addressed to 1 tab.
- [When the result arrives through an event](templates.md#when-the-result-arrives-through-an-event): keep a loading indicator on until the event handler patches the tab.
- [Sessions and auth](sessions.md): the session behind user-addressed events.
- [`example/todolist`](../example/todolist/): a list that every tab re-renders on each change.
