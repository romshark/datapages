# Todo List

A collaborative real-time todo list demonstrating per-tab server-side state handling.
Changes made in one tab are immediately reflected in all other open tabs across all
connected clients. For simplicity reasons,
all data is stored in memory and lost on restart.

This example implements the official Datastar design recommendtations following
[The Tao of Datastar](https://data-star.dev/guide/the_tao_of_datastar).

https://github.com/user-attachments/assets/bac07de1-bce6-43fe-af32-41a10c2e0add

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

- Following the [CQRS](https://data-star.dev/guide/the_tao_of_datastar#cqrs)
  architecture, actions (commands) transmit user inputs to the server while UI
  updates are received via SSE. To reduce code complexity, the server sends the
  whole page template rerendered with new data
  (["fat morph"](https://data-star.dev/guide/the_tao_of_datastar#in-morph-we-trust)),
  which isn't a problem thanks to
  [Brotli compression](https://andersmurphy.com/2025/04/15/why-you-should-use-brotli-sse.html).
- HMAC-signed identifiers securily identify each individual browser tab
  (or rather the SSE stream that tab opens). Every action handler verifies this
  signature, preventing one tab from impersonating another.
- All application state is managed by the server and stored on the server
  (see [State in the Right Place](https://data-star.dev/guide/the_tao_of_datastar#state-in-the-right-place)).
- For simplicity reasons, an in-memory message broker is used since this example
  doesn't require a multi-instance setup.
- Filter and sort state is synced to the URL via `reflectsignal` query parameters,
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
    SS->>SS: Store tab state<br>(streamID -> filter/sort)
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
