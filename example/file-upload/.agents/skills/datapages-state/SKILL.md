---
name: datapages-state
description: >-
  Add per-tab Datapages State[T], initialize and use it in handlers, dispatch
  state-scoped events, and configure the live instance limit. Activate when a
  Datapages page needs server-side values that belong to one browser tab.
---

# Per-tab state

Read `datapages` first for the build loop, hard rules and naming conventions.

Declare `T` as an exported, package-level struct in the app package. `State[T]` rejects pointers, anonymous structs and types from other packages. A page is stateful when one of its actions, `On` handlers or stream hooks takes `datapages.State[T]`.

Every stateful handler on a page must use the same `T`. An app-level action may take `State[T]` only when the calling page uses that type. A mismatch returns 409 with `Datapages-Retry: reconnect`. Keep actions used by pages with different state types stateless.

```go
type StateIndex struct{ Filter string }

func (PageIndex) StreamOpen(
	r *http.Request, state datapages.State[StateIndex],
) error {
	state.Values.Filter = "all"
	return nil
}
```

`GET` cannot take state. The server allocates state when the tab connects its SSE stream. A stateful page gets a stream even when it has no stream hooks or event handlers. The server runs stateful handlers for one tab serially. They can read and write `state.Values` without another mutex.

A disconnect deletes the instance. A reconnect starts with a zero value. Initialize it in `StreamOpen` from the URL or signals when needed.

`GET` may return `datapages.EnableBackgroundStreaming(true)` to keep the stream and state active while the tab is hidden. This also disables the default refresh when the tab becomes visible. `datapages.DisableRefreshAfterHidden(true)` disables that refresh without preserving the stream or state. Only pages with a stream refresh after being hidden. Returning either value from a page without a stream is an error.

Do not retain `state.Values` after the handler returns. Do not capture it in a goroutine or a component that renders later. Copy the required fields instead.

State exists in one server process. Route a tab's stream and actions to the same server with `Datapages-Instance` as the routing key. The instance ID script is inline: a `Content-Security-Policy` must allow `script-src 'unsafe-inline'` or set a nonce through `WithCSPNonce`. See `datapages-server`.

## Events for one tab

A stateful handler may take `stateID string` with `State[T]`. Put the value in an event's `datapages.SubjectStateID` field to address that tab:

```go
// EventFilterChanged is "filter.changed"
type EventFilterChanged struct {
	Tab datapages.SubjectStateID
}
```

It must be the event's only subject field and cannot have a `signal` tag. A page that handles the event must use state. That page cannot also handle a user-addressed or signal-scoped event. `stateID` selects the event recipient. It does not provide access to the state value.

## Limit

Each server allows `datapages.DefaultMaxConcurrentInstances` live instances by default. Change the limit with `datapages.WithStateConfig(datapages.StateConfig{MaxConcurrentInstances: n})`. When the server reaches the limit, a new stream receives 503 with `Retry-After`.
