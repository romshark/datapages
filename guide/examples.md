# Examples

The [`example/`](../example/) directory contains 12 applications in 11 modules.

## Run an example

Each example is a separate Go module. Its `go.mod` replaces `github.com/romshark/datapages` with the surrounding checkout at `../../`. Clone the repository and run commands from the example directory:

```sh
git clone https://github.com/romshark/datapages
cd datapages/example/todolist
go run ./cmd/server
```

The generated code and the frontend bundles are committed, and `go run` needs only Go. The examples that need more say so below.

`datapages watch` regenerates the code with the CLI on the `PATH`. Install the CLI from the same checkout, in the repository root:

```sh
go install ./cmd/datapages
```

A release of the CLI generates the code of its release, which does not compile against the checkout, with errors such as `not enough arguments in call to s.streams.Handle`.

> [!NOTE]
> Fast shim, Offline cache and Docs install a service worker for their server address. The browser keeps the worker after the server stops. It can answer requests from the next example at that address with cached files. Before running another example there, unregister the worker under Application and Service workers in the browser developer tools.

Each example also has the agent instructions that `datapages init` writes. See [AI agents](ai-agents.md).

## Counter

A counter that every open tab updates, built 2 times in 1 module. `app/simple` is the smallest Datapages application, and the [Introduction](README.md) walks through it. `app/fancy` is the same model with animated digits.

Features:

- A page and an action with a query parameter. See [Pages and routes](pages.md) and [Actions](actions.md).
- An event, its dispatcher and an `On` handler, over the in-memory broker. See [Events](events.md).
- 2 app packages, 2 generated packages and 2 entry points, with event subjects that carry the name of the application. See [Multiple applications](multiple-applications.md).

Run:

```sh
go run ./cmd/simple   # http://localhost:8080/
go run ./cmd/fancy    # http://localhost:8081/
datapages watch --app app/simple
```

The code is in [`example/counter`](../example/counter/).

## Todo list

A collaborative todo list. A change in 1 tab appears in every open tab, and each tab keeps its own filter, sort order and search. It follows [the Tao of Datastar](https://data-star.dev/guide/the_tao_of_datastar). Actions send commands, and the server answers over SSE with the whole list rendered again. The data lives in memory.

Features:

- Per-tab state in `datapages.State[T]`, set in `StreamOpen` and read by the event handlers. See [Per-tab state](state.md).
- `EventTodoUpdated`, which every open tab handles. See [Events](events.md).
- Query fields with `reflectsignal` tags, which keep the filter, sort order and search in the URL. See [Pages and routes](pages.md).
- `POST`, `PUT` and `DELETE` actions on pages and on `App`. See [Actions](actions.md).
- A custom 404 page and static files. See [Static assets](assets.md).

Run:

```sh
go run ./cmd/server   # http://localhost:8080/
datapages watch       # http://localhost:7331/
```

The code is in [`example/todolist`](../example/todolist/).

## Calculator

A calculator that evaluates expressions on the server with [`shopspring/decimal`](https://github.com/shopspring/decimal). Each button press sends 3 signals to a `QUERY` action, which answers with an SSE patch of the calculator and its signals. The server keeps no state between requests, and the page has no stream. The same app package runs as an HTTP server and as a [Wails v3](https://v3.wails.io/) desktop app, as the [FAQ](../FAQ.md) describes.

Features:

- `PageIndex.QUERYInput` with `datapages.Signals`, `datapages.Query` and `datapages.SSE`. See [Actions](actions.md).
- Static files. See [Static assets](assets.md).
- In the desktop app, `datapages.WithNoSSECompression`, `datapages.WithNoResponseCompression` and `datapages.WithNoAssetsCompression`, because its only client is a webview on the loopback interface. See [Performance](performance.md).
- Tests with `net/http/httptest`. See [Testing](testing.md).

Run with [Mage](https://magefile.org/):

```sh
mage runServer        # http://localhost:8080/
mage runDesktop       # the desktop app
mage dev              # datapages watch, http://localhost:7331/
mage testUIWorkflows  # UI tests with Maestro
```

`mage runServer` and `mage runDesktop` run `templ generate` and `datapages gen` first. `go run ./cmd/server` starts the HTTP server without Mage. The desktop app uses cgo and needs a C compiler. `mage testUIWorkflows` needs [Maestro](https://maestro.dev/).

The code is in [`example/calculator`](../example/calculator/).

## Classifieds

A classifieds marketplace with sign-in, posts, search, user pages, chat with typing indicators and unread counts, and a settings page that ends sessions. It runs on NATS and comes with Prometheus metrics, Grafana dashboards and [k6](https://k6.io/) load tests.

Features:

- Sessions in the NATS key-value store, `NewSession` at sign-in, `CloseSession` at sign-out, and closing the other sessions of a user. See [Sessions and auth](sessions.md).
- Events addressed to 1 user with `datapages.SubjectUser`, and a stream embed, `Base`, whose `On` handlers every page except `PageLogin` and `PageError500` shares. See [Events](events.md).
- Per-tab state for the chat a tab has open, seeded from a signal in `StreamOpen`. See [Per-tab state](state.md).
- Search through a `QUERY` action, with `reflectsignal` query fields that keep the search in the URL. See [Pages and routes](pages.md).
- `PageError404`, `PageError500` and `RecoverError`. See [Actions](actions.md).
- The `natscore` broker, Prometheus metrics with collectors of the application, and TLS. See [Deployment](deployment.md).
- A self-hosted Datastar bundle passed to `datapages.WithDatastarJS`. See [Static assets](assets.md).

Run, with Docker:

```sh
make dev    # http://localhost:7331/
make down
```

`make dev` starts NATS, Prometheus and Grafana in containers and then `datapages watch`. The app itself listens on port 52000, as `.env.dev` sets. Grafana is at <http://localhost:3000/> and Prometheus at <http://localhost:9091/>. `make load` runs the k6 load test against the running app. `make stage` builds the server and runs it with TLS through `sudo`. It needs [mkcert](https://github.com/FiloSottile/mkcert) and the `HOST` of `.env.stage` in `/etc/hosts`.

The code is in [`example/classifieds`](../example/classifieds/).

## Tailwind CSS

A static page styled with [Tailwind CSS](https://tailwindcss.com) v4. A custom watcher of `datapages watch` rebuilds `app/static/output.css` when a `.templ` file or `input.tw.css` changes, and a second one reloads the browser when a static file changes.

Features:

- Static files served with `datapages.WithAssets`. See [Static assets](assets.md).
- Custom watchers in `datapages.yaml`. See [datapages.yaml](configuration.md).

Run, with Node.js and npm:

```sh
make dev              # npm install, the CSS build and datapages watch, http://localhost:7331/
go run ./cmd/server   # with the committed output.css, http://localhost:8080/
```

The code is in [`example/tailwindcss`](../example/tailwindcss/).

## Web Components

A landing page with 2 Web Components in TypeScript, `<wc-counter>` without dependencies and `<wc-particles>` built with [Lit](https://lit.dev). A custom watcher bundles them with [esbuild](https://esbuild.github.io) into `app/static/bundle.js` on every change and reloads the browser.

Features:

- Web Components in templ templates. See [Templates and forms](templates.md).
- The bundle served as a static file through `assets.Path("bundle.js")`. See [Static assets](assets.md).
- A custom watcher in `datapages.yaml`. See [datapages.yaml](configuration.md).

Run, with Node.js:

```sh
make npm-install
make dev              # http://localhost:7331/
```

The code is in [`example/webcomponents`](../example/webcomponents/).

## SQLite sessions

Sign-in and registration with sessions that a custom `sessions.Manager` stores in SQLite through [sqinn-go](https://github.com/cvilsmeier/sqinn-go), without cgo. The rest of the application is the smallest one that uses the store.

Features:

- A session manager with the 5 methods of `sessions.Manager`, and `DeleteExpired` scheduled by `cmd/server/main.go`. See [Sessions and auth](sessions.md).
- `NewSession` at sign-in and registration, and `CloseSession` at sign-out.
- `EventSessionClosed`, addressed with `datapages.SubjectUser`, which redirects the other tabs of the closed session. See [Events](events.md).
- `QUERY` actions that validate the forms while the visitor types. See [Actions](actions.md).

Run:

```sh
make dev              # datapages watch, http://localhost:7331/
```

The first start creates 3 users, `alice@example.com`, `bob@example.com` and `carol@example.com`, all with the password `password123`. The database is `sqlitesessions.db` in the working directory, and `SESSION_DB_PATH` changes it. sqinn-go ships a prebuilt `sqinn` binary, and `SQINN_PATH` selects another.

The code is in [`example/sqlitesessions`](../example/sqlitesessions/).

## Fast shim

Pages that take 900 ms to render. `/` and `/subpage` show a cached shim at once and the live page when it arrives. `/noshim` and `/noshim2` wait for the live page on every visit.

Features:

- `pageCache.SetShim` with a version that changes with the markup of the shim. See [Offline and shims](offline.md).
- The offline module installed with `offline.WithServiceWorker`, for an app without `PageOffline`.

Run:

```sh
go run ./cmd/server   # http://localhost:8080/
```

Reload a page with a shim to see the shim before the live page. The worker runs only on HTTPS and on localhost.

The code is in [`example/fast-shim`](../example/fast-shim/).

## Offline cache

A ticketing app with live search, purchases and QR-code tickets, which stay available offline. The data lives in memory. The demo accounts are `moviebuff` and `jazzfan`, with the password `demopass`.

Features:

- Snapshots written with `pageCache.Set`, `ClearAll` at sign-in and sign-out, `PageOffline` and the generated `WithOffline`, and a self-hosted Datastar bundle. See [Offline and shims](offline.md).
- Sessions with the in-memory session manager, and a redirect to sign-in with `?next=` from protected pages. See [Sessions and auth](sessions.md).
- Live search through a `QUERY` action, with the search kept in `?q=`. See [Actions](actions.md) and [Pages and routes](pages.md).
- `PageError404`, `PageError500` and tests with `net/http/httptest`. See [Testing](testing.md).

Run:

```sh
go run ./cmd/server   # http://localhost:8080/
```

To see the offline pages, sign in, buy or open a ticket, stop the server and reload. The ticket still opens, and a show you never opened falls back to `/offline`.

The code is in [`example/offline-cache`](../example/offline-cache/).

## File upload

Uploads in chunks to disk, which can be paused, resumed, taken over by another tab, and continued after a reload or a restart of the server. Completed files can be downloaded. Upload and download rates have limits that the server enforces and every tab shows.

Features:

- Per-tab state, an event addressed to 1 tab with `datapages.SubjectStateID`, and `StreamClose`, which releases the transfers of a closed tab. See [Per-tab state](state.md) and [Events](events.md).
- `App.GETFile`, a GET action that returns `datapages.File`, and its `href` builder. See [Actions](actions.md).
- `PUTChunk`, an action that reads the request body and that JavaScript calls instead of Datastar.
- `enableBackgroundStreaming`, which keeps the stream open while the tab is hidden. See [Pages and routes](pages.md).
- Tests with `net/http/httptest`. See [Testing](testing.md).

Run:

```sh
go run ./cmd/server   # http://localhost:8080/
datapages watch       # http://localhost:7331/
```

The files go to `uploads` in the working directory, and `-dir` selects another directory.

The code is in [`example/file-upload`](../example/file-upload/).

## Docs

A collaborative rich text editor. Any number of tabs edit the same document and see the changes, cursors and avatars of the others, and a tab keeps its changes across a reload or a lost connection. The editor is [Quill](https://quilljs.com) in a Web Component. The server orders the changes with operational transformation of Quill deltas, ported to Go, and stores each document as a snapshot and a change log.

Features:

- `EventDocChanged`, whose `datapages.Subject` field is bound to the signal `docId`, which reaches only the tabs of 1 document. See [Events](events.md).
- Per-tab state that records what a tab received, with `StreamOpen` and `StreamClose`. See [Per-tab state](state.md).
- `sse.ExecuteScript`, which hands the changes to the editor. See [Actions](actions.md).
- `App.GETImage`, a GET action that serves uploaded images with `datapages.File`.
- `PageOffline`, `WithOffline` and a self-hosted Datastar bundle. See [Offline and shims](offline.md).
- A Web Component bundled by esbuild. See [Templates and forms](templates.md).
- Go tests, and a JavaScript test that runs 3 editors against a model of the server. See [Testing](testing.md).

Run, with Node.js 24 or later for the editor bundle and its tests:

```sh
go run ./cmd/server   # http://localhost:8080/
make npm-install
make dev              # http://localhost:7331/
make test
```

The documents go to `documents` in the working directory, and `-dir` selects another directory. `-host 0.0.0.0:8080` lets other devices on the network edit the documents.

The code is in [`example/docs`](../example/docs/).

## See also

- [Installation](installation.md): the CLI and templ, which `datapages watch` needs
- [Tutorial](tutorial/README.md): a project built step by step
- [Application model](application-model.md): what the examples declare and what is generated for them
- [Multiple applications](multiple-applications.md): the module layout of the counter
- [FAQ](../FAQ.md): desktop apps, templ and NATS
