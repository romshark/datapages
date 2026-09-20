---
name: datapages-state
description: >-
  Add per-tab Datapages State[T], initialize and use it in handlers, dispatch
  state-scoped events, and configure the live instance limit. Activate when a
  Datapages page needs server-side values that belong to one browser tab.
---

# Per-tab state

Read `datapages` first for the build loop, hard rules and naming conventions.

Declare `T` as an exported struct at package level in the app package. `State[T]` rejects pointers, anonymous structs and types from other packages. A page becomes stateful when an action, `On` handler, `StreamOpen` or `StreamClose` takes `datapages.State[T]`.

Every stateful handler on a page must use the same `T`. An app-level action may take `State[T]` only when the calling page uses that `T`; a mismatch returns 409 with `Datapages-Retry: reconnect`. Keep actions callable from every page stateless.

```go
type StateIndex struct{ Filter string }

func (PageIndex) StreamOpen(
	r *http.Request, state datapages.State[StateIndex],
) error {
	state.Values.Filter = "all"
	return nil
}
```

`GET` cannot take state: the server allocates it when the tab connects its SSE stream. A stateful page gets a stream even without stream hooks or event handlers. The server serializes handlers of the same tab that take state, so they can read and write `state.Values` without another mutex. Disconnect releases the instance. Reconnect starts with a zeroed value; initialize it in `StreamOpen` from the URL or signals if needed.

`GET` may return `datapages.EnableBackgroundStreaming(true)` to keep the stream and state alive while the tab is hidden; this also disables the default refresh when the tab becomes visible. `datapages.DisableRefreshAfterHidden(true)` suppresses that refresh without preserving the stream or state.

Do not retain `state.Values` past the handler, including in a goroutine or a component that renders later. Copy the fields needed after the handler returns. State lives in one server process. Route a tab's stream and actions to the same server using `Datapages-Instance` as the routing key. If middleware sets `Content-Security-Policy`, it must allow `script-src 'unsafe-inline'` for the instance-ID script; no nonce hook exists.

## Events for one tab

A stateful handler may take `stateID string` alongside `State[T]`. Put it in an event's `datapages.SubjectStateID` field to address that tab:

```go
// EventFilterChanged is "filter.changed"
type EventFilterChanged struct {
	Tab datapages.SubjectStateID
}
```

It must be the event's only subject field. It cannot carry a `signal` tag. A page handling the event must use state and cannot also handle a user-addressed or signal-scoped event. `stateID` addresses events; it does not grant access to the state value.

## Limit

The default cap is `datapages.DefaultMaxConcurrentInstances` live instances per server. Set `datapages.WithStateConfig(datapages.StateConfig{MaxConcurrentInstances: n})` on the server to change it. At the cap, a new stream receives 503 with `Retry-After`.
