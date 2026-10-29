# Examples

The [`example/`](../example/) directory contains 13 applications in 12 modules.

## Run an example

Each example is a Go module that replaces `github.com/romshark/datapages` with the surrounding checkout. Clone the repository and run it from its directory:

```sh
git clone https://github.com/romshark/datapages
cd datapages/example/todolist
go run ./cmd/server
```

Generated code and frontend bundles are committed, so `go run` needs only Go unless noted below. To use `datapages watch`, install the CLI from the checkout root:

```sh
go install ./cmd/datapages
```

A released CLI may generate code that does not compile against the checkout.

> [!NOTE]
> Fast shim, Offline cache and Docs install a service worker that remains after the server stops. Before using another example at the same address, unregister the worker under Application and Service workers in the browser developer tools.

## Counter

Two applications in 1 module implement a [counter](../example/counter/) that updates every open tab. `app/simple` is the application from the [Introduction](README.md), while `app/fancy` adds animated digits. Both use a query action, an [event](events.md) over the in-memory broker and separate generated packages and entry points. See [Multiple applications](multiple-applications.md).

```sh
go run ./cmd/simple   # http://localhost:8080/
go run ./cmd/fancy    # http://localhost:8081/
datapages watch --app app/simple
```

## Todo list

The [collaborative todo list](../example/todolist/) stores its data in memory. Changes appear in every tab, while [per-tab state](state.md) keeps each tab's filter, sort order and search. Reflected query fields keep those settings in the URL. Actions dispatch `EventTodoUpdated`, whose handler renders the whole list again. The app also has static files and a custom 404 page.

```sh
go run ./cmd/server   # http://localhost:8080/
datapages watch       # http://localhost:7331/
```

## Calculator

The [calculator](../example/calculator/) evaluates expressions on the server with [`shopspring/decimal`](https://github.com/shopspring/decimal). A stateless `QUERY` action receives 3 signals and patches the result. The same app package runs as an HTTP server and a [Wails v3](https://wails.io/) desktop app. The desktop build disables compression because its webview connects over the loopback interface.

Use [Mage](https://magefile.org/):

```sh
mage runServer        # http://localhost:8080/
mage runDesktop       # the desktop app
mage dev              # datapages watch, http://localhost:7331/
mage testUIWorkflows  # UI tests with Maestro
```

The desktop app needs a C compiler. UI tests need [Maestro](https://maestro.dev/). `go run ./cmd/server` needs no Mage.

## Classifieds

The [classifieds marketplace](../example/classifieds/) has sign-in, posts, search, user pages and chat. It demonstrates NATS sessions and user-addressed events, shared stream handlers, per-tab chat state, reflected search queries, custom error handling, Prometheus metrics, TLS and a self-hosted Datastar bundle. Its [Toxiproxy](network.md) preset applies different faults to an action, a page stream and static files.

Requires Docker:

```sh
make dev    # http://localhost:7331/
make down
```

`make dev` starts NATS, Prometheus, Grafana and `datapages watch`. Ports are 7332 for the proxied app, 7333 for Toxiproxy settings, 3000 for Grafana and 9091 for Prometheus. `make load` runs the k6 test. `make stage` needs [mkcert](https://github.com/FiloSottile/mkcert) and the staged `HOST` in `/etc/hosts`.

## Tailwind CSS

The [Tailwind example](../example/tailwindcss/) is a static page styled with [Tailwind CSS](https://tailwindcss.com) v4. Custom watchers rebuild the stylesheet after template or CSS changes and reload the browser after static-file changes.

Requires Node.js and npm:

```sh
make dev              # npm install, the CSS build and datapages watch, http://localhost:7331/
go run ./cmd/server   # with the committed output.css, http://localhost:8080/
```

## Web Components

The [Web Components example](../example/webcomponents/) is a landing page with 2 TypeScript components. `<wc-counter>` has no dependency; `<wc-particles>` uses [Lit](https://lit.dev). A custom watcher bundles them with [esbuild](https://esbuild.github.io), serves the bundle as a static asset and reloads the browser.

Requires Node.js:

```sh
make npm-install
make dev              # http://localhost:7331/
```

## SQLite sessions

The small [sign-in application](../example/sqlitesessions/) stores sessions in SQLite through a custom `sessions.Manager` and [sqinn-go](https://github.com/cvilsmeier/sqinn-go), without cgo. It schedules `DeleteExpired`, validates forms with `QUERY` actions and redirects other tabs through a user-addressed `EventSessionClosed`.

```sh
make dev              # datapages watch, http://localhost:7331/
```

The first start creates `alice@example.com`, `bob@example.com` and `carol@example.com`, all with the password `password123`. `SESSION_DB_PATH` changes the database path, and `SQINN_PATH` selects another `sqinn` binary.

## Fast shim

The [fast-shim example](../example/fast-shim/) shows how shims affect pages that take 900 ms to render. `/` and `/subpage` use versioned `pageCache.SetShim` entries; `/noshim` and `/noshim2` wait for the live page. The app installs the offline worker without a `PageOffline`.

```sh
go run ./cmd/server   # http://localhost:8080/
```

Reload a shimmed page to see both stages. The worker requires HTTPS or localhost.

## Offline cache

The in-memory [ticketing app](../example/offline-cache/) has live search, purchases and QR-code tickets. It caches page snapshots and static assets, clears private snapshots at sign-in and sign-out, provides `PageOffline` and self-hosts Datastar. Protected pages preserve their destination in `?next=`. The demo users `moviebuff` and `jazzfan` use the password `demopass`.

```sh
go run ./cmd/server   # http://localhost:8080/
```

To test it, open a ticket, stop the server and reload. Opened tickets remain available; uncached pages fall back to `/offline`.

## File upload

The [file-upload example](../example/file-upload/) sends files in chunks. Transfers can pause, continue in another tab and resume after a reload or server restart. Per-tab state and events track each transfer. `StreamClose` releases a tab's transfers, `PUTChunk` receives chunks and `App.GETFile` returns completed files. The server limits transfer rates and keeps streams open in hidden tabs.

```sh
go run ./cmd/server   # http://localhost:8080/
datapages watch       # http://localhost:7331/
```

Files go to `uploads`; `-dir` selects another directory.

## Docs

The [collaborative rich-text editor](../example/docs/) uses [Quill](https://quilljs.com) in a Web Component. Open tabs show each other's changes, cursors and avatars and retain unsent changes across reloads and disconnections. The server applies operational transformation to Quill deltas and stores snapshots and change logs.

Each event is scoped to 1 document. Per-tab state tracks received changes, `sse.ExecuteScript` updates Quill and a file action serves images. The app works offline and tests 3 concurrent editors against a JavaScript server model.

Building and testing the editor requires Node.js 24 or later:

```sh
go run ./cmd/server   # http://localhost:8080/
make npm-install
make dev              # http://localhost:7331/
make test
```

Documents go to `documents`; `-dir` selects another directory. Use `-host 0.0.0.0:8080` to edit from other devices.

## Realtime charts

The [realtime charts example](../example/realtime-charts/) simulates 48 indexes updated every second. Its chart, market map and portfolio pages use [Lightweight Charts](https://github.com/tradingview/lightweight-charts), [Apache ECharts](https://echarts.apache.org/) and [uPlot](https://github.com/leeoniya/uPlot). A goroutine writes prices to an in-memory time-series database and publishes updates under 1 event subject per index.

A shared stream embed renders the whole view as HTML on every push, with the data of each chart in a JSON attribute, and stream compression sends little more than what changed. State-addressed events update 1 tab. Reflected query fields preserve the selection, chart type, time range and paused second. Datastar Rocket wraps the chart libraries in custom elements.

```sh
go run ./cmd/server   # http://localhost:8080/
datapages watch       # http://localhost:7331/
```

The chart libraries load from jsDelivr. `HISTORY` sets retention. Its default, `1h`, uses about 11 MB. `HOST` and `PORT` change the address. The example README describes the architecture and push sizes.
