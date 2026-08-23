# Offline Cache — Datapages Demo

A small ticketing app: browse shows with live search, view a show, buy a ticket,
and see your tickets — complete with a scannable QR code. Authentication uses
server-side sessions and all data lives in a simple in-memory store seeded with
mock data.

It demonstrates Datapages best practices:

- **Hypermedia-first, no custom JavaScript** — the UI is driven by Datastar
  attributes and server-rendered HTML fragments over SSE.
- **Live search** — the shows list filters as you type, patching only the
  results container so the input keeps focus. The query is reflected into the
  URL (`?q=`) so searches are deep-linkable.
- **Session-based auth** — pages that require a login redirect guests to the
  sign-in page and back again (`?next=`).
- **Type-safe URLs and actions** — every link and action uses the generated
  `app/datapagesgen/href` and `app/datapagesgen/action` packages.
- **Automatic light/dark mode** via `prefers-color-scheme`.
- **Offline support** via the `modules/offline` service-worker module (see below).

## Pages

| Route                          | Page          | Description                                    |
| ------------------------------ | ------------- | ---------------------------------------------- |
| `/`                            | redirect      | Redirects to `/shows`.                         |
| `/shows`                       | Shows         | Browse all shows with live search.             |
| `/shows/{nameslug}`            | Show          | View a single show and start a purchase.       |
| `/shows/{nameslug}/purchase`   | Purchase      | Confirm and "pay" for a ticket (auth).         |
| `/shows/{nameslug}/ticket`     | Ticket        | Your ticket for the show, with a QR code (auth).|
| `/tickets`                     | My Tickets    | All tickets you have bought (auth).            |
| `/login`                       | Login         | Sign in with a demo account.                   |

## Demo accounts

| Username    | Password   |
| ----------- | ---------- |
| `moviebuff` | `demopass` |
| `jazzfan`   | `demopass` |

`moviebuff` starts with two pre-purchased tickets.

## Running

The demo runs entirely in-memory — no NATS, database, or other services are
required.

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

- `app/domain` — the thread-safe, in-memory data store (shows, users, tickets).
- `app/*.go` — one page struct per route with `GET`/`POST…` handlers.
- `app/app.templ` — all Templ templates (navbar, cards, ticket, forms, …).
- `cmd/server` — wires up the in-memory session manager and message broker and
  seeds the mock data (`testdata.go`).

QR codes are generated server-side (`github.com/skip2/go-qrcode`) and embedded
as `data:` URIs, so no external requests are needed to render a ticket.

## Offline support

Offline support is provided by the reusable `github.com/romshark/datapages/modules/offline`
module and wired in with a single server option in `cmd/server/main.go`:

```go
datapagesgen.WithOffline(app.OfflineConfig())
```

`WithOffline` is generated because the app declares `PageOffline`: it supplies
that page's route to the module, so the route stays declared only on the page
type.

The middleware serves a generated **service worker** at `/service-worker.js` and
automatically injects its registration into every page — the application's
`<head>` and templates stay untouched (Datapages still owns the head).

What it does:

- **Precaches the app shell** (CSS, JS, icons) and the offline fallback page on
  install. All assets are self-hosted (`app/static/`) so the app works with no
  network at all.
- **Serves an offline fallback** for uncached pages: going to the shows search
  while offline shows *"You're currently offline, come back when you're back
  online."* (the `/offline` page, `PageOffline`).
- **Keeps bought tickets viewable offline.** `PageTicket.GET` receives a
  `pageCache datapages.PageCacheWriter` handle and calls
  `pageCache.Set(href.PageTicket(slug), view, ver)` to store the ticket's
  offline snapshot (versioned by the ticket code, so it is only re-cached when
  it changes).

`OfflineConfig` lives in `app/offline.go`. Any GET or action handler can control
its page's offline copy through the injected `pageCache
datapages.PageCacheWriter` parameter: `Version()`, `Set(url, body, version)`,
`SetShim(...)`, `Clear(url)`, `ClearAll()`.
Actions receive `sse datapages.SSE`, which hides the datastar runtime.

Try it: load the app, buy (or open) a ticket, then stop the server and reload —
the search page shows the offline message while your ticket still opens.
