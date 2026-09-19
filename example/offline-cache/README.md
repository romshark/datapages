# Offline Cache

A ticketing app: browse shows with live search, view a show, buy a ticket and see
your tickets, each with a scannable QR code. Authentication uses server-side sessions.
All data lives in an in-memory store seeded with mock data and is lost on restart.

What it demonstrates:

- Hypermedia-first with no custom JavaScript. The UI is driven by Datastar
  attributes and server-rendered HTML fragments over SSE.
- Live search. The shows list filters as you type, patching only the results container,
  which keeps the focus in the input. The query is reflected into the
  URL (`?q=`), which makes a search deep-linkable.
- Session-based auth. A page that requires a login redirects guests to the
  sign-in page and back again (`?next=`).
- Type-safe URLs and actions. Every link and action uses the generated
  `app/datapagesgen/href` and `app/datapagesgen/action` packages.
- Light and dark mode via `prefers-color-scheme`.
- Offline support via the `modules/offline` service-worker module, see below.

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
is wired in with one server option in `cmd/server/main.go`:

```go
datapagesgen.WithOffline(app.OfflineConfig())
```

`WithOffline` is generated because the app declares `PageOffline`. It supplies
that page's route to the module: the route stays declared only on the page type.

The middleware serves a generated service worker at `/service-worker.js` and
injects its registration into every page. The application's `<head>` and its
templates stay untouched, Datapages still owns the head.

What it does:

- Precaches the app shell (CSS, JS, icons) and the offline fallback page on install.
  Every asset is self-hosted in `app/static/`. The app needs no network at all.
- Serves the offline fallback for an uncached page. Opening a show that was never visited
  online shows "You're currently offline, come back when you're back online.",
  which is the `/offline` page, `PageOffline`. `/` has its own cached snapshot and
  states instead that search needs a connection.
- Keeps bought tickets viewable offline. `PageTicket.GET` receives a
  `pageCache datapages.PageCacheWriter` handle and calls
  `pageCache.Set(href.PageTicket(slug), view, ver)` to store the ticket's
  offline snapshot. The version covers the session and the purchase time,
  which rewrites the snapshot only when the ticket or the signed-in user changes.

`OfflineConfig` lives in `app/offline.go`. Any GET or action handler can control
its page's offline copy through the injected `pageCache
datapages.PageCacheWriter` parameter: `Version()`, `Set(url, body, version)`,
`SetShim(...)`, `Clear(url)`, `ClearAll()`.
Actions receive `sse datapages.SSE`, which hides the Datastar runtime.

Try it: load the app, buy (or open) a ticket, then stop the server and reload.
The shows page reports that search is unavailable, a show you never opened falls
back to `/offline`, and your ticket still opens.
