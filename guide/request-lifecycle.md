# Request lifecycle

A loaded page can open an SSE stream, send actions and receive events from other requests. The generated server also handles reconnects, deployments, closed sessions and shutdown. The sequence below names each path, header and status code.

## Overview

The examples on this page use this page type:

```go title="app/app.go"
// EventCounterUpdated is "counter.updated"
type EventCounterUpdated struct{}

type StateIndex struct{ Visits int }

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(
	r *http.Request, session Session,
) (body datapages.Component, err error) {
	return pageIndex(session.UserID(), p.App.counter.Load()), nil
}

func (p PageIndex) StreamOpen(
	r *http.Request, state datapages.State[StateIndex],
) error {
	state.Values.Visits++
	return nil
}

// POSTAdd is /add
func (p PageIndex) POSTAdd(
	r *http.Request,
	counterUpdated datapages.Dispatcher[EventCounterUpdated],
) error {
	p.App.counter.Add(1)
	return counterUpdated.Dispatch(EventCounterUpdated{})
}

func (p PageIndex) OnCounterUpdated(
	event EventCounterUpdated, sse datapages.SSE, session Session,
) error {
	return sse.PatchElement(counter(p.App.counter.Load()))
}
```

`App` holds `counter atomic.Int64`, `Session` is `datapages.Session[struct{}]`, and `pageIndex` and `counter` are templ components. 2 tabs load the page, and tab A clicks the button that calls `POSTAdd`:

```text
Tab A       Tab B                Server            Broker
  |           |                     |                 |
  |-- GET / ----------------------->|                 |  PageIndex.GET
  |<- 200 text/html ----------------|                 |
  |-- GET /_$/ -------------------->|                 |
  |           |                     |-- subscribe --->|
  |<= event-stream =================|                 |  StreamOpen
  |           |-- GET / ----------->|                 |  PageIndex.GET
  |           |<- 200 text/html ----|                 |
  |           |-- GET /_$/ -------->|                 |
  |           |                     |-- subscribe --->|
  |           |<= event-stream =====|                 |  StreamOpen
  |-- POST /add/ ------------------>|                 |  PageIndex.POSTAdd
  |           |                     |-- publish ----->|
  |<- 200 --------------------------|                 |
  |           |                     |<- deliver ------|
  |           |                     |<- deliver ------|
  |<= patch ========================|                 |  OnCounterUpdated
  |           |<= patch ============|                 |  OnCounterUpdated
```

`=` marks the long-lived SSE response of a page stream. The right column names the application code that runs on the server.

## Before any handler

Every request passes the same chain before the generated handler of its route runs:

1. The server appends `/` to a path that does not end in one, except below the asset prefix. `/post/hello` and `/post/hello/` reach the same route.
2. The Prometheus middleware, when the server counts metrics, and then response compression.
3. The middleware passed to `datapages.WithMiddleware`, the first one outermost.
4. The build check. A Datastar request whose `Datapages-Build` header differs from the build ID of the server receives `205 Reset Content`. See [A new deployment](#a-new-deployment).
5. The `net/http.ServeMux` with the routes of the app package. `PageIndex` registers `GET /`, which also catches every path no other route matches. It answers such a path with `404 Not Found`, rendered by `PageError404` when the app package declares it.

## The page load

The browser requests the page URL, `GET /`, without Datapages headers. The generated `GET` handler:

1. Reads the session when the page needs it. It looks up the session cookie, `sessiontoken` by default, in the session store. A cookie the store does not know is deleted, an expired session is closed and its cookie deleted, and a store that fails answers `503 Service Unavailable`.
2. Parses the path and query parameters. A value that does not parse answers `400 Bad Request`.
3. For a page with [per-tab state](state.md), mints a `Datapages-Instance` ID of 16 random bytes and sets it as a response header with `Cache-Control: no-store`, which keeps a `no-transform` that a middleware set. It allocates no state yet.
4. Calls `GET`. An error with status 404 or 500 renders `PageError404` or `PageError500` when the app package declares it. Any other error answers with its status and the status text.
5. Writes the session cookie for a returned `newSession` or deletes it for `closeSession`.
6. Answers a returned `datapages.Redirect` with `302 Found`, or its `Status`, and `Location`. Otherwise renders the document with status 200.

The document for the page above, with line breaks added and script bodies shortened:

```html
<!DOCTYPE html><html><head><meta charset="UTF-8"/>
<meta name="datapages-build" content="build-1"/>
<script>(() => { ... h.set("Datapages-Build",b) ... })()</script>
<script>(() => { let __dpInstance="UDSVhIHvd6z8rC7ExQpGXw" ... })()</script>
<script type="module" src="https://cdn.jsdelivr.net/gh/starfederation/datastar@1.0.4/bundles/datastar.js"></script>
<script type="module"> ... h.set("X-CSRF-Token",'...') ... </script>
</head><body data-on:visibilitychange__window="if (!document.hidden) window.location.reload()">
<main><p id="user">u1</p><p id="counter">0</p><div id="toasts"></div></main>
<template data-init="@get('/_$/',{retry:'always',retryMaxCount:Infinity})"></template>
</body></html>
```

In order, Datapages writes:

- The build meta element and the build script. The script adds `Datapages-Build` to every same-origin Datastar request and reloads the page when the answer is `205 Reset Content`.
- The instance script, on a page with per-tab state. It adds `Datapages-Instance` to every same-origin Datastar request, then removes its own element from the DOM. A `409 Conflict` with `Datapages-Retry: reconnect` reloads the page once.
- The Datastar bundle, whose URL `datapages.WithDatastarJS` sets.
- The tags of `App.Head`, then the head the handler returned.
- The CSRF script, in a document rendered for a session cookie. It adds `X-CSRF-Token` to every same-origin Datastar request other than `GET`, `HEAD`, `OPTIONS` and `QUERY`.
- On `<body>`, the attribute that reloads the page when its tab becomes visible again, on a page with a stream unless its `GET` turns it off, and a `data-signals` attribute for each query field with a `reflectsignal` tag.
- The body the handler returned.
- On a page with a stream, a `template` element whose `data-init` opens the stream once Datastar has loaded.

With `datapages.WithCSPNonce`, every script Datapages writes carries the nonce, and `<html>` carries it as `data-nonce`. See [Content Security Policy](../SPECIFICATION.md#content-security-policy).

## The page stream

A page with an event handler, a stream hook or per-tab state serves its stream at its route followed by `_$/`, as `/_$/` for `PageIndex` and `/post/hello/_$/` for `/post/{slug}`. Datastar requests it with `GET`, the headers `Datastar-Request: true` and `Datapages-Build`, the session cookie, and the signals of the page as JSON in the `datastar` query parameter. A page with state also sends `Datapages-Instance`.

After the build check, the generated stream handler checks the request:

| Check | Answer |
| --- | --- |
| `Datastar-Request: true` is missing | `406 Not Acceptable` |
| `Datapages-Instance` is not 22 base64url characters, on a page with state | `409 Conflict` with `Datapages-Retry: reconnect` |
| The server has reached its limit of live tab states, 10000 by default or the limit set with `datapages.WithStateConfig` | `503 Service Unavailable` with `Retry-After: 5` |
| A signal that scopes a subscription is empty | `400 Bad Request` |

Then it opens the stream:

1. It allocates a zeroed `StateIndex` for the instance ID, on a page with state.
2. It subscribes to the subjects of the events the page handles, before it answers. A dispatch that follows the response head therefore reaches the stream.
3. It answers `200` with `Content-Type: text/event-stream` and `Cache-Control: no-cache`, and keeps the response open.
4. It runs `StreamOpen`. An error goes to `App.RecoverError` when the app declares it, and the stream ends with a reconnect delay of 5s.
5. It handles the events of the subscription, 1 at a time, until the stream ends.

The stream ends when the browser closes it, when the server shuts down, or when the session it was opened with closes or expires. `StreamClose` then runs, and the tab's state is released. A page that handles events addressed to a user and also public events or state serves signed-out visitors from a second stream at `_$/anon/`. See [Events](events.md).

## An action

A button with `data-on:click={ action.PageIndex.Add.POST() }` sends `POST /add/` with `Datastar-Request: true`, `Datapages-Build`, `Datapages-Instance`, `X-CSRF-Token` for a signed-in visitor, and the signals of the page as a JSON body. The generated handler checks the request before it calls the action:

| Check | Answer | Applies to |
| --- | --- | --- |
| `Datastar-Request: true` is missing | `406 Not Acceptable` | actions that take `datapages.Signals`, `datapages.SSE` or `datapages.PageCacheWriter` |
| `Sec-Fetch-Site` is `same-site` or `cross-site`, or `Origin` differs from `Host` | `403 Forbidden` | the other actions, which a plain HTML form can reach |
| `Datapages-Instance` is missing or has no state on this server | `409 Conflict` with `Datapages-Retry: reconnect` | actions that take `datapages.State[T]` |
| `X-CSRF-Token` is missing or wrong | `403 Forbidden` | `POST`, `PUT`, `PATCH` and `DELETE` requests that carry a session cookie |
| The signals body exceeds 1 MiB | `400 Bad Request` | actions other than `DELETE` that take `datapages.Signals`, see `datapages.WithBodySizeLimit` |
| A signal, path or query value does not parse | `400 Bad Request` | all actions |

An action that takes a session reads it from the store and treats a closed or expired session as a guest. An action without a session parameter checks the CSRF token against the cookie alone, without reading the store. See [Sessions and auth](sessions.md).

The handler's signature decides the response:

| The action | Response |
| --- | --- |
| returns only `error` | `200` with an empty body |
| takes `sse datapages.SSE` | `200` with `Content-Type: text/event-stream` and the patches the action sends, closed when it returns |
| returns `datapages.Redirect` | for a Datastar request, `200` with `Content-Type: text/javascript` and `window.location = "/";`, otherwise `302 Found` with `Location`. An action with `sse` sends the redirect as a script on its stream. |
| returns `body` | a complete HTML document, as a page load writes |
| returns `datapages.File` | the file, served by `http.ServeContent` |
| returns `newSession` or `closeSession` | the same response with a `Set-Cookie` header that sets or deletes the session cookie |

An error that the action returns or a panic in it takes the error path. For a Datastar request, the server calls `App.RecoverError` with an open SSE stream, and the response keeps status 200. This `RecoverError` appends a message to the element `#toasts`:

```go title="app/app.go"
func (*App) RecoverError(err error, sse datapages.SSE) error {
	return sse.PatchElementAt(
		errorToast("Could not save. Try again."), "#toasts", datapages.PatchModeAppend,
	)
}
```

An action that fails answers:

```text
event: datastar-patch-elements
data: selector #toasts
data: mode append
data: elements <p class="toast">Could not save. Try again.</p>
```

Without `RecoverError`, the response carries the status of the error, `400`, `403`, `404` or `409` for the [sentinels](../SPECIFICATION.md#return-value-error-or-err-error) and `500` for any other error. A request from a plain HTML form receives `PageError404` or `PageError500` for status 404 or 500 when the app package declares them. See [Actions](actions.md).

## An event

`counterUpdated.Dispatch(EventCounterUpdated{})` reaches every tab that has the page open:

1. The generated dispatcher encodes the event as JSON and builds its subject, the subject of the event comment followed by the values of its subject fields. `EventCounterUpdated` has none, so the subject is `counter.updated`.
2. It publishes the message to the broker and returns. The action does not wait for the streams.
3. The broker delivers the message to every subscription of that subject, which is 1 per open stream of every page that handles the event. With `modules/messaging/natscore` that includes the streams of every server process connected to the same NATS server. `modules/messaging/inmem` reaches the streams of 1 process.
4. Each stream calls `OnCounterUpdated` with the event, its own SSE stream and the other values the handler declares, such as the session of its tab. The patches go to that tab only.
5. Datastar morphs each patched element into the element with the same ID.

The patch of step 4 on the wire:

```text
event: datastar-patch-elements
data: elements <p id="counter">1</p>
```

The acting tab receives the event on its own stream like any other tab. Delivery is at most once. A stream that is closed when the event is published, such as the stream of a hidden tab, misses it, and a stream whose 16-message buffer is full drops it. Render from the current data in `OnXXX`, not from the event alone. An error that `OnXXX` returns is logged, and a panic goes to `App.RecoverError`. See [Event delivery](../SPECIFICATION.md#event-delivery).

## Reconnects

The stream request that the document writes uses `retry:'always'` and `retryMaxCount:Infinity`. Datastar reopens the stream after every end, whether the server closed it, a proxy answered `502` or the network dropped. It waits 1s, then doubles the wait up to 30s, without a limit. After a failed `StreamOpen` or a session end, the server sets the wait to 5s.

A reconnect opens a new stream on whatever server receives it:

| Survives | Is lost |
| --- | --- |
| the document, its DOM and its signals | the state of the tab. The server allocates a zeroed `T` and runs `StreamOpen` again. |
| the `Datapages-Instance` ID, which the page keeps | the events published while no stream of the tab was subscribed |
| the session cookie | the broker subscription, which the new stream makes again |

Rebuild per-tab state in `StreamOpen` from the signals it receives. A stateful action that arrives before the new stream receives `409 Conflict` with `Datapages-Retry: reconnect`, and the instance script reloads the page once.

By default Datastar closes the stream of a hidden tab, and the server releases its state. The page reloads when the tab becomes visible again, which renders what it missed. The `GET` return values [`enableBackgroundStreaming`](../SPECIFICATION.md#get-return-value-enablebackgroundstreaming-datapagesenablebackgroundstreaming) and [`disableRefreshAfterHidden`](../SPECIFICATION.md#get-return-value-disablerefreshafterhidden-datapagesdisablerefreshafterhidden) change this.

## A new deployment

Each server has a build ID, by default the first 16 bytes of the SHA-256 hash of its executable in hex. `datapages.WithBuildID` sets it. Every document carries the ID of the server that rendered it in `<meta name="datapages-build">`, and the build script sends it in `Datapages-Build` with every same-origin Datastar request.

A server with another build ID answers such a request with `205 Reset Content`, its own ID in `Datapages-Build` and `Cache-Control: no-store`. It does not run the handler, because the routes, element IDs and signals of the old document may not match the new code. Middleware added with `WithMiddleware` sees the request. The build script then reloads the page:

- An action reloads at once. The action is not applied.
- A stream reconnect reloads after a random delay of up to 2s, which spreads the page loads of tabs that reconnect together.
- Repeated reloads without a successful Datastar request wait 1s, 2s, 4s and up to 30s.

A request without `Datapages-Build`, such as one from `curl` or from a page rendered before the build script existed, passes. See [Deployments](../SPECIFICATION.md#deployments) and [Deployment](deployment.md).

## A closed or expired session

A stream is opened with a session when the page handles an event addressed to a user or when one of its stream handlers takes the session, as `OnCounterUpdated` above does. The server ends such a stream when the session closes, as when the visitor signs out in any tab or a `newSession` replaces it, and when the session reaches its `ExpiresAt`. The last event of the stream reloads the page after 1s:

```text
event: datastar-patch-elements
retry: 5000
data: selector body
data: mode append
data: elements <script data-effect="el.remove()">setTimeout(() => { try { history.replaceState(history.state, "", location.href) } catch {} location.reload() }, 1000)</script>
```

The delay lets the tab that signed out follow its own redirect first. The reload renders the page for the session the browser holds now, if any. `retry: 5000` sets the reconnect delay of the closed stream.

## Graceful shutdown

The server shuts down when the context passed to `ListenAndServe` is canceled. The `cmd/server/main.go` of the scaffold cancels it on `SIGINT` and `SIGTERM`. Then:

- The server stops accepting connections.
- Every open stream ends at once. It runs `StreamClose`, releases the state of its tab, and the server resets its connection.
- The server waits for the other requests in flight, for at most 10s. `datapages.WithShutdownTimeout` changes the limit.
- `ListenAndServe` returns.

The pages reconnect to the next server process. A process of the same build resumes the stream, and one of another build answers 205, which reloads the page. Events published while no process serves a stream are lost. The reset is a panic with `http.ErrAbortHandler`, and middleware that recovers panics must panic again with that value.

## See also

- [Application model](application-model.md): how the generated server is built from the app package.
- [Pages](../SPECIFICATION.md#pages): the exact rules for page, action and stream handlers.
- [Deployments](../SPECIFICATION.md#deployments): the build ID and the reload rules.
- [Per-tab state](state.md): the lifecycle of `datapages.State[T]`.
- [Events](events.md): subjects, dispatchers and delivery.
- [Troubleshooting](troubleshooting.md): reloads, 406, 409 and other answers this page names.
