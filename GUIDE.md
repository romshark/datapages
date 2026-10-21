# Getting Started

This tutorial creates a Datapages project, adds an action, and introduces the features most applications need next. Use [SPECIFICATION.md](SPECIFICATION.md) for exact declaration rules and runtime behavior.

## Requirements

Install the latest [Go toolchain](https://go.dev/dl/) and [templ](https://templ.guide) `v0.3.1070`. The generated project also uses [Docker](https://www.docker.com/) for NATS. You can replace NATS with the in-memory modules for a single-process application.

Install the CLI and templ:

```sh
go install github.com/romshark/datapages/cmd/datapages@latest
go install github.com/a-h/templ/cmd/templ@v0.3.1070
```

## Create and run a project

Create the project:

```sh
datapages init
```

The initializer asks for the project settings and generates the server. Accept its prompt to start the application, or run:

```sh
make dev
```

`make dev` starts NATS and `datapages watch`. Open the URL it prints.

Use `datapages init --prometheus=false` when the first version of the project does not need metrics. Use `--no-ai-skills` to omit agent instructions.

## Read the scaffold

The project starts with these files:

```text
.
├── app/                 application model and templ components
│   ├── app.go
│   ├── app.templ
│   └── datapagesgen/    generated server and typed helpers
├── cmd/
│   └── server/
│       └── main.go      dependencies, server options, and process startup
└── datapages.yaml       development tooling configuration
```

Work in `app/` and `cmd/server/`. Generation replaces `datapagesgen/` and every file with a `DO NOT EDIT` header.

`app/app.go` defines the application model. `app/app.templ` renders the first page. `cmd/server/main.go` creates the dependencies and server.

## Use the development loop

Keep `datapages watch` running while you work. It regenerates code, rebuilds the server, and reports failures in the browser.

Run the steps separately when diagnosing a build:

```sh
templ generate
datapages gen
datapages lint
go build ./...
```

`templ generate` compiles templates. `datapages gen` generates the server from the application model. `datapages lint` checks that model without writing files. Fix generator errors in the application package, then regenerate.

## Understand the application model

Datapages reads the types, methods, and comments in the application package. You do not maintain a separate route table.

Put long-lived dependencies, such as repositories and API clients, on `App`. Pages render documents, actions handle browser commands, events update open pages, and stream hooks manage resources for an open page.

Start with `PageIndex` and add 1 feature at a time. The [source package specification](SPECIFICATION.md#source-package) lists every recognized declaration. Use [stream embeds](SPECIFICATION.md#stream-embeds) when several pages share event handlers or stream hooks.

## Add a page action

First, add a fragment that the action can render. Replace the generated page body in `app/app.templ`:

```templ
templ pageIndex() {
	<main>
		@status("Ready")
	</main>
}

templ status(text string) {
	<p id="status">{ text }</p>
}
```

Generate the template, then add the action to `app/app.go`:

```go
// POSTUpdateStatus is /status
func (PageIndex) POSTUpdateStatus(
	r *http.Request,
	sse datapages.SSE,
) error {
	return sse.PatchElement(status("Updated"))
}
```

Run `datapages gen`. Import the generated `action` package in `app/app.templ`, then add this button inside `main`:

```templ
<button
	type="button"
	data-on:click={ action.PageIndex.UpdateStatus.POST() }
>
	Update status
</button>
```

Regenerate the template or let the watcher do it. Clicking the button calls the action and replaces the `status` element. See [Pages](SPECIFICATION.md#pages) for action signatures and [`SSE`](SPECIFICATION.md#parameter-sse-datapagessse) for other browser updates.

## Read request data

Choose a parameter by where the input belongs:

- Use `datapages.Path` for route values.
- Use `datapages.Query` for URL state that should be bookmarkable or shareable.
- Use `datapages.Signals` for Datastar state sent with an action.
- Use `*http.Request` for headers, context, and other HTTP data.

See [`Path`](SPECIFICATION.md#parameter-datapagespathstruct-), [`Query`](SPECIFICATION.md#parameter-datapagesquerystruct-), and [`Signals`](SPECIFICATION.md#parameter-datapagessignalsstruct-) for their field types and tags.

## Update more than the acting tab

An action with `datapages.SSE` updates only the page that sent the request. Use an event to update other pages or server processes.

A common flow is:

1. Validate the command.
2. Change authoritative application data.
3. Dispatch an event.
4. Re-read that data in each `OnXXX` handler.
5. Render a complete current fragment.

Treat an event as a notification, not as the source of truth. Re-reading the data keeps the page correct after a reconnect or missed event. See [`Dispatcher`](SPECIFICATION.md#parameter-datapagesdispatchereventxxx) for declarations and [event delivery](SPECIFICATION.md#event-delivery) for delivery guarantees.

## Keep temporary state per tab

Use `datapages.State[T]` for temporary values owned by 1 browser tab, such as an unfinished wizard or a selection used by an event handler. Keep durable data in a repository, and do not use per-tab state as a session store.

Initialize stream resources in `StreamOpen` and release them in `StreamClose`. See [`State[T]`](SPECIFICATION.md#parameter-datapagesstatet) for its lifecycle and multi-server requirements.

## Add sessions and authorization

Add a session manager in `cmd/server/main.go`, then accept the application's session type in handlers that need identity. Keep authorization in the application. A valid session identifies the caller but does not authorize an operation.

See [`Session`](SPECIFICATION.md#parameter-session-datapagessessiondata), [`NewSession`](SPECIFICATION.md#return-value-newsession-datapagesnewsessiondata), and [`CloseSession`](SPECIFICATION.md#return-value-closesession-datapagesclosesession). Read [Plain Forms and CSRF](SPECIFICATION.md#plain-forms-and-csrf) before adding authenticated forms.

## Handle failures

Return ordinary Go errors from handlers. Return a Datapages sentinel for a specific HTTP status. Use `App.RecoverError` to render failed Datastar requests and page error handlers for failed page loads. See [`App`](SPECIFICATION.md#app) and [`error`](SPECIFICATION.md#return-value-error-or-err-error) for recovery behavior.

## Write templates with generated helpers

Use the generated `href` package for links and the generated `action` package for Datastar actions:

```templ
<a href={ href.PagePost(post.Slug) }>{ post.Title }</a>
<button data-on:click={ action.PageIndex.UpdateStatus.POST() }>
	Update status
</button>
```

The generated helpers keep templates synchronized with the Go model, and `datapages lint` checks their use. Keep application decisions in Go. Use templates for markup and Datastar attributes. See [Linting](SPECIFICATION.md#linting) for the exact rules.

## Configure the server

Create repositories and clients in `cmd/server/main.go`, put long-lived dependencies on `App`, then configure the broker, session manager, and `datapages.NewServer` options.

The generated server implements `http.Handler`, so standard Go middleware can wrap it. See [`ServerOption`](https://pkg.go.dev/github.com/romshark/datapages#ServerOption) for available options and [Content Security Policy](SPECIFICATION.md#content-security-policy) before setting a CSP.

## Tune SSE compression

Datapages enables balanced SSE compression by default. Choose another preset when memory, bandwidth, or large fragments matter more:

- `datapages.SSECompressionLowMemory()` uses smaller windows for many open streams with fragments below about 100 KB.
- `datapages.SSECompressionLowBandwidth()` prefers brotli for slow or metered clients.
- `datapages.SSECompressionLargeFragments()` uses 4 MiB windows for fragments above about 1 MB.
- `datapages.SSECompressionBalanced()` returns the defaults.

Apply a preset to every stream:

```go
datapages.WithSSECompression(datapages.SSECompressionLowMemory()),
```

Override it for 1 page with `StreamConfig`:

```go
func (PageReport) StreamConfig() datapages.StreamConfig {
	c := datapages.SSECompressionLargeFragments()
	return datapages.StreamConfig{Compression: &c}
}
```

Override 1 action by appending its method name:

```go
func (PageEditor) StreamConfigPOSTSave() datapages.StreamConfig {
	return datapages.StreamConfig{
		Compression: &datapages.SSECompressionConfig{Disabled: true},
	}
}
```

The method can also choose settings per request:

```go
func (PageFeed) StreamConfig(r *http.Request) (datapages.StreamConfig, error) {
	if !strings.Contains(r.UserAgent(), "Mobi") {
		return datapages.StreamConfig{}, nil
	}
	c := datapages.SSECompressionLowBandwidth()
	return datapages.StreamConfig{Compression: &c}, nil
}
```

A method replaces the server setting. Zero fields use package defaults. See [Choose where compression runs](#choose-where-compression-runs) before disabling compression. See [`SSECompressionConfig`](https://pkg.go.dev/github.com/romshark/datapages#SSECompressionConfig) for the presets and [`StreamConfig`](SPECIFICATION.md#streamconfig) for valid method signatures.

## Tune response compression

Datapages compresses page and action bodies of at least 256 bytes. The default preset balances CPU, memory and size. `datapages.ResponseCompressionLowCPU()` reduces CPU and memory. `datapages.ResponseCompressionLowBandwidth()` prefers brotli for slow or metered clients.

```go
datapages.WithResponseCompression(datapages.ResponseCompressionLowCPU()),
```

`datapages.File` responses remain uncompressed. Static assets use separate compression; see [Tune asset compression](#tune-asset-compression). To send precompressed content, serve an archive such as `export.csv.gz` with type `application/gzip`, or negotiate `Accept-Encoding` and set `Content-Encoding` in `Header`. Give each representation its own `ETag` and send `Vary: Accept-Encoding`. See [Choose where compression runs](#choose-where-compression-runs) for loopback clients and proxies. Set `Cache-Control: private, no-transform` on pages that combine secrets with attacker-controlled input, and on their `_$/` streams, or give such a page a `StreamConfig` that returns `Compression: &datapages.SSECompressionConfig{Disabled: true}`. See [Response Compression](SPECIFICATION.md#response-compression) for the rules and [`ResponseCompressionConfig`](https://pkg.go.dev/github.com/romshark/datapages#ResponseCompressionConfig) for measurements.

## Choose where compression runs

Disable it for loopback-only clients:

```go
datapages.WithNoResponseCompression(),
datapages.WithNoSSECompression(),
datapages.WithNoAssetsCompression(),
```

Behind a reverse proxy, keep it on unless a body filter such as nginx `sub_filter` must read the HTML. Then disable it, or set `proxy_set_header Accept-Encoding "";`. Caches must honor `Vary: Accept-Encoding`.

## Choose modules for the deployment

The scaffold uses NATS for messaging and sessions. A single-process deployment can use the in-memory modules:

| Need | Single process | Multiple processes |
| ---- | -------------- | ------------------ |
| Events | `modules/messaging/inmem` | `modules/messaging/natscore` |
| Sessions | `modules/sessions/inmem` | `modules/sessions/natskv` |

Restarting the in-memory session manager removes its sessions. Use durable sessions in production. With more than 1 application server, also follow the routing rules for [`State[T]`](SPECIFICATION.md#parameter-datapagesstatet).

## Deploy a new version

Open tabs keep the document from the previous deployment. Each Datastar request carries its build ID. A server with another ID returns `205 Reset Content` without running the handler, and the page reloads.

The default build ID is a hash of the executable. If you set `datapages.WithBuildID`, change it whenever element IDs, signals, routes, or other browser-server contracts change. Give every replica of 1 release the same ID, and route each client to 1 build during a rolling deployment. See [Deployments](SPECIFICATION.md#deployments).

## Serve static assets

Declare an `embed.FS` in the app package and put the URL path in its doc comment, as the example in `app/app.go` shows:

```go
// StaticFS is /static/
//
//go:embed static/*
var StaticFS embed.FS
```

Pass it to the server in `cmd/server/main.go`:

```go
opts = append(opts, datapages.WithAssets(app.StaticFS, false))
```

`false` answers 404 instead of listing a directory. The browser asks before it reuses a file and gets a 304 without a body while the file is unchanged. Set a `MaxAge` with `datapages.WithAssetsCache` only for files whose name changes with their content, such as `datastar-1.0.4.js`. Otherwise browsers keep old files after a deploy.

Run `datapages gen`, then write asset URLs with the generated `href` or `assets` helper:

```templ
<link rel="stylesheet" href={ href.Asset("style.css") }/>
```

Spell each path exactly like its file. `/static/App.js` gets 404 for `app.js` on every system.

Keep assets unchanged while the server runs, and serve files that change through a GET action that returns `datapages.File`. See [Changing Assets](SPECIFICATION.md#changing-assets), [Dev Mode](SPECIFICATION.md#dev-mode) and [`WithAssets`](https://pkg.go.dev/github.com/romshark/datapages#WithAssets).

## Tune asset compression

Datapages compresses eligible assets on their first request and keeps the copies in memory. To compress them before the first request, create the compressor and precompress the embedded files:

```go
assets, err := assetcompress.New(assetcompress.Config{})
if err != nil {
	return err
}
opts = append(opts, datapages.WithAssetsCompression(assets))
go func() {
	if err := assets.Precompress(ctx, app.StaticFS); err != nil {
		slog.Error("precompressing assets", slog.Any("err", err))
	}
}()
```

Pass the whole `embed.FS`. Copies are keyed by content, which lets them serve the subdirectory the server uses. Precompression stops at the first error, and requests compress the files it did not reach. Drop `go` to finish before serving, which delays startup.

For copies 6% to 11% smaller, use brotli 11 and drop `go` before `Precompress`. A request that finds no copy otherwise waits for its compression, up to 215 ms per 200 KB:

```go
assets, err := assetcompress.New(assetcompress.Config{
	Levels: assetcompress.Levels{BrotliQuality: 11},
})
```

Without a store, the copies stay in memory until the server exits, at about 1/4 of the size of the compressible files per coding. Persist them in a directory when they take too much memory, or when brotli 11 has megabytes to compress again after every restart, at 1 to 1.5 MB per second:

```go
store, err := disk.New("/var/cache/myapp/assets", disk.Config{})
if err != nil {
	return err
}
assets, err := assetcompress.New(assetcompress.Config{Store: store})
```

The store serves each copy from its file instead of from memory. Skip it for a few assets at the default levels, which recompress in a few milliseconds after a restart. Give the store a directory that only the server can write to. For a Kubernetes `emptyDir` volume, pass a subdirectory of its mount point, which `disk.New` creates. The store deletes copies that no server used for 7 days. Set `disk.Config.MaxUnused` to keep them longer.

To keep the copies elsewhere, such as in object storage that all servers share, implement [`assetcompress.Store`](https://pkg.go.dev/github.com/romshark/datapages/modules/assetcompress#Store) and pass it as `Store`. Every request for a stored copy calls `Open`, so cache remote copies locally. Keys contain the SHA-256 of the file, which lets the servers of every build share the store. The compressor never deletes copies. Expire old ones in the storage, for example with a lifecycle rule.

See [Asset Compression](SPECIFICATION.md#asset-compression) for the rules and [`assetcompress.Config`](https://pkg.go.dev/github.com/romshark/datapages/modules/assetcompress#Config) for measurements.

## Add offline page caching

The offline module installs a service worker and lets handlers cache page snapshots. Start with the fallback for a URL that has no snapshot:

```go
// PageOffline is /offline
type PageOffline struct{ App *App }

func (PageOffline) GET(
	r *http.Request,
) (body datapages.Component, err error) {
	return pageOffline(), nil
}
```

`PageOffline` renders with a zero session and is shared by every visitor. Keep its content independent of the signed-in user.

Next, configure the worker. List every asset a snapshot needs before its first online load, including a self-hosted Datastar bundle:

```go
const offlineWorkerVersion = 1

func OfflineConfig() offline.Config {
	return offline.Config{
		WorkerVersion: offlineWorkerVersion,
		Assets: []string{
			assets.Path("style.css"),
			assets.Path("datastar-1.0.4.js"),
		},
	}
}
```

Put the library version in the file name, such as `datastar-1.0.4.js`. Browsers and the offline worker cache files by URL. If an upgrade keeps the name, they can keep using the old file from their cache. A new name is a new URL, which they download.

Declaring `PageOffline` generates its server option. Add it with the bundle URL:

```go
datapages.WithDatastarJS(assets.Path("datastar-1.0.4.js")),
datapagesgen.WithOffline(app.OfflineConfig()),
```

The worker runs only on HTTPS and localhost.

Add `pageCache datapages.PageCacheWriter` to a `GET` or action. Store an offline body with `Set`, using a URL from the generated `href` package:

```go
func (p PageItem) GET(
	r *http.Request,
	pageCache datapages.PageCacheWriter,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
) (body datapages.Component, err error) {
	item, err := p.App.Item(r.Context(), path.Values.ID)
	if err != nil {
		return nil, err
	}
	if pageCache.Version() != item.Revision {
		pageCache.Set(href.PageItem(item.ID), itemOffline(item), item.Revision)
	}
	return itemPage(item), nil
}
```

`Version()` returns the cached version of the current URL, or 0 when it has no entry. `Clear` removes 1 entry, and `ClearAll` removes all page entries. Cache writes take effect together after the handler succeeds.

Keep snapshots safe and current:

- Pass a page body, not a complete document.
- Include every value that changes the snapshot in its version. Use `!=` for hashes and unordered version keys.
- Do not cache secrets or data that must disappear at sign-out. Call `ClearAll` during sign-in and sign-out.
- Update or clear a URL when an action changes its data.
- Increment `WorkerVersion` after a Datapages upgrade or a change to the worker configuration. Give a changed cross-origin asset a new URL or increment the version.

Same-origin assets enter the cache on their first online load unless excluded. `Config.Assets` precaches required files. Use the `is-offline` class on `<html>` to disable controls that need the network.

Response compression runs after `WithOffline`. Put a custom compressing middleware before it. With a nonce-based CSP, `WithCSPNonce` must return the same nonce for every call with 1 request.

See [`pageCache`](SPECIFICATION.md#parameter-pagecache-datapagespagecachewriter), [Service Worker](SPECIFICATION.md#service-worker), and the complete [`example/offline-cache`](example/offline-cache/).

## Show a cached shim while a page loads

Use `SetShim` when a live page is slow. The worker shows the cached body while it fetches the live page, then replaces the document head and body:

```go
const shimVersion = 1

func (p PageIndex) GET(
	r *http.Request,
	pageCache datapages.PageCacheWriter,
) (body datapages.Component, err error) {
	rows, err := p.App.Rows(r.Context())
	if err != nil {
		return nil, err
	}
	if pageCache.Version() != shimVersion {
		pageCache.SetShim(href.PageIndex(), indexShim(len(rows)), shimVersion)
	}
	return indexPage(rows), nil
}
```

The first visit stores the shim. Later visits can show it before the live response arrives. Increment `shimVersion` when its markup or data changes. This version is independent of `offline.Config.WorkerVersion`.

A shim can remain visible after a failed fetch. Use neutral placeholder text. It has no session or CSRF script and must contain no Datastar attributes. Keep actions and visitor-specific content in the live body.

If the application has no custom `PageOffline`, install the module directly:

```go
offline.WithServiceWorker("", offline.Config{
	WorkerVersion: 1,
})
```

See [`example/fast-shim`](example/fast-shim/) for a complete comparison.

## Build multiple applications

1 Go module can contain several application packages and entry points. `datapages gen` processes them all. Select 1 for development:

```sh
datapages watch --app app/frontend
```

`--app` is the application directory relative to the module root. Put shared event types in a shared package. See [shared events](SPECIFICATION.md#events-declared-outside-the-application-package) and [`example/counter`](example/counter/).

## Configure development tooling

The scaffold configures generation and watch mode in `datapages.yaml`:

```yaml
cmd: cmd/server
watch:
  exclude:
    - ".git/**"
    - ".*"
    - "*~"
```

`cmd` identifies the main package until the project contains a `datapages.NewServer` call. Add `watch` settings for the proxy, TLS, compiler flags, or custom file watchers. Run `datapages watch --help` for command-line overrides.

## Use the generated agent instructions

`datapages init` writes `AGENTS.md`, tool-specific pointers, and Datapages skills. Update them from the installed CLI with:

```sh
datapages init -n
```

The command backs up a changed file before replacing it. If the project uses both skill directories, edit both copies.

## Test through HTTP

Use `httptest` against the generated `http.Handler` to test page loads, actions, and streams. Test domain logic separately without HTTP. The [acceptance-test guide](internal/acceptance/README.md) shows how to construct a generated server and assert HTML and SSE responses.

## Continue

- [SPECIFICATION.md](SPECIFICATION.md) defines the source model and runtime.
- [example/](example/) contains complete applications.
- [SECURITY.md](SECURITY.md) lists application and deployment responsibilities.
- [FAQ.md](FAQ.md) explains design decisions.
