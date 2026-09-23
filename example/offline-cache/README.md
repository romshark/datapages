# Offline Cache

A ticketing example with live search, purchases and QR-code tickets. It uses
server-side sessions and an in-memory data store. A process restart removes all
data.

What it demonstrates:

- Datastar attributes and server-rendered HTML fragments, with no application
  JavaScript.
- Live search that updates the results without moving input focus. The `?q=`
  parameter preserves a search in the URL.
- Session authentication. Protected pages send guests to sign-in and use
  `?next=` to return afterward.
- Generated URLs and actions. Every link and action uses the
  `app/datapagesgen/href` and `app/datapagesgen/action` packages.
- Light and dark mode via `prefers-color-scheme`.
- Offline pages through `modules/offline`.

https://github.com/user-attachments/assets/325a5f88-a9c5-45f1-b2fc-bbaca92bb246

## Pages

| Route                        | Page           | Description                                         |
| ---------------------------- | -------------- | --------------------------------------------------- |
| `/`                          | `PageIndex`    | Browse all shows with live search.                  |
| `/shows/{nameslug}`          | `PageShow`     | View a single show and start a purchase.            |
| `/shows/{nameslug}/purchase` | `PagePurchase` | Confirm and "pay" for a ticket (auth).              |
| `/shows/{nameslug}/ticket`   | `PageTicket`   | Your ticket for the show, with a QR code (auth).    |
| `/tickets`                   | `PageTickets`  | All tickets you have bought (auth).                 |
| `/login`                     | `PageLogin`    | Sign in with a demo account.                        |
| `/offline`                   | `PageOffline`  | Served for a URL with no cached copy while offline. |
| `/not-found`                 | `PageError404` | Unknown URL.                                        |
| `/whoops`                    | `PageError500` | Failed request.                                     |

## Demo accounts

| Username    | Password   |
| ----------- | ---------- |
| `moviebuff` | `demopass` |
| `jazzfan`   | `demopass` |

`moviebuff` starts with two pre-purchased tickets.

## Running

The demo runs entirely in memory. No NATS, database or other service is required.

```sh
templ generate          # compile .templ files (after any .templ change)
datapages gen           # generate the Datapages server code
go run ./cmd/server     # serve on http://localhost:8080
```

Or, for live reload during development:

```sh
datapages watch
```

## How it works

- `app/domain` is the thread-safe, in-memory data store: shows, users, tickets.
- `app/*.go` holds one page struct per route with its `GET` and `POST` handlers.
- `app/app.templ` holds every templ template: navbar, cards, ticket, forms.
- `cmd/server` wires up the in-memory session manager and message broker and
  seeds the mock data (`testdata.go`).

QR codes are rendered server-side (`github.com/skip2/go-qrcode`) and embedded as
`data:` URIs. Rendering a ticket makes no external request.

## Offline support

Offline support comes from `github.com/romshark/datapages/modules/offline` and
is configured with one server option in `cmd/server/main.go`:

```go
datapagesgen.WithOffline(app.OfflineConfig())
```

Declaring `PageOffline` generates `WithOffline`. The option reads the route from
the page declaration.

The middleware serves `/service-worker.js` and adds registration to HTML
responses. No template registers the worker.

What it does:

- Caches the CSS, JavaScript, icons and offline fallback during installation.
  Every asset is in `app/static/`.
- Serves the offline fallback for an uncached page. Opening a show that was never visited
  online shows "You're currently offline, come back when you're back online.",
  which is the `/offline` page, `PageOffline`. `/` has its own cached snapshot and
  states instead that search needs a connection.
- Keeps purchased tickets available offline. `PageTicket.GET` receives a
  `pageCache datapages.PageCacheWriter` handle and calls
  `pageCache.Set(href.PageTicket(slug), view, ver)` to store the ticket's
  offline snapshot. The version covers the session and the purchase time,
  which updates the snapshot only when the ticket or signed-in user changes.

`OfflineConfig` lives in `app/offline.go`. Any GET or action handler can control
an offline copy through the `pageCache
datapages.PageCacheWriter` parameter: `Version()`, `Set(url, body, version)`,
`SetShim(...)`, `Clear(url)`, `ClearAll()`.

To test it, load the app, buy or open a ticket, stop the server, and reload.
The shows page reports that search is unavailable, a show you never opened falls
back to `/offline`, and your ticket still opens.
