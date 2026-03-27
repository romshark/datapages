# Todo List

A collaborative real-time todo list demonstrating per-tab server-side state handling,
HMAC-signed tab identifiers, and the
[CQRS](https://data-star.dev/guide/the_tao_of_datastar#cqrs) &
[fat morphs](https://data-star.dev/guide/the_tao_of_datastar#in-morph-we-trust)
architecture.

Changes made in one tab are immediately reflected in all other open tabs.
All data is stored in memory and lost on restart.

## Prerequisites

- [Go](https://go.dev/dl/) 1.26+
- [Datapages](https://github.com/romshark/datapages) CLI (for `datapages watch`)

## Run

```sh
HMAC_SECRET_KEY=my-secret go run ./cmd/server
```

Then open http://localhost:8080/.

The `HMAC_SECRET_KEY` defaults to a dev-only value if unset.

## Develop

```sh
datapages watch
```

Then open http://localhost:7331/.

## Architecture

This example uses a [CQRS](https://data-star.dev/guide/the_tao_of_datastar#cqrs)
approach with
[fat morphs](https://data-star.dev/guide/the_tao_of_datastar#in-morph-we-trust).
Actions (commands) modify state and dispatch `EventTodoUpdated`.
Event handlers (queries) re-render the relevant page fragment via SSE.

- Toggling done is supported on both pages.
- All state lives on the server. No JavaScript.

Tab identity is established by HMAC-signing the `streamID` in `StreamOpen`
and patching the result to the client as a `tab_id` signal. Every action handler
verifies this signature, preventing one tab from impersonating another.

Editing and toggling are handled by a single shared App-level action
`PUTEdit /{id}`. When called with `?toggle=true` (from PageIndex checkboxes),
it flips the done state server-side. When called without (from PageItem inputs),
it updates all fields from signals. Both paths dispatch `EventTodoUpdated`.

Filter and sort state is synced to the URL via `reflectsignal` query parameters,
so reloading the page preserves the current view.

### Interaction flow

```mermaid
sequenceDiagram
    participant B as Browser Tab
    participant S as Server
    participant N as Message Broker

    B->>S: GET /
    activate S
    S->>B: HTML page<br/>(signals: search, filter, sort)
    deactivate S

    B->>S: SSE connect<br/>(signals: search, filter, sort)
    activate S
    create participant SS as SSE goroutine
    S->>SS: StreamOpen
    SS->>SS: Store tab state<br>(streamID → filter/sort)
    SS->>SS: HMAC-sign streamID
    SS->>B: patch signal tab_id
    deactivate S
    activate SS
    Note over SS: kept alive<br/>until disconnect
    SS->>N: Subscribe to EventTodoUpdated

    loop Every toggle / edit
    B->>S: PUT /{id}?toggle=true<br/>(signal: tab_id)
    activate S
    S->>S: Verify tab_id HMAC
    S->>S: Toggle todo done state
    S->>N: Publish EventTodoUpdated
    S->>B: 200 OK
    deactivate S

    N->>SS: Deliver EventTodoUpdated
    activate SS
    SS->>SS: Look up tab state by streamID
    SS->>SS: Render filtered todo list
    SS->>B: SSE morph patch #todo-list
    deactivate SS
    end

    loop Every filter / sort / search change
    B->>S: POST /filter (SSE action)<br/>(signals: tab_id, filter, sort, search)
    activate S
    S->>S: Verify tab_id, extract streamID
    S->>S: Update tab state for streamID
    S->>S: Render filtered todo list
    S->>B: SSE morph patch #todo-list
    deactivate S
    end
```
