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
  toxiproxy:
    presets:
      - name: "Fast 4G"
        toxics:
          latency:
            downstream:
              latency: 165ms
              jitter: 0s
          bandwidth:
            downstream:
              rate: 1013
            upstream:
              rate: 169
      # Slow 4G and 3G follow.
```

`cmd` identifies the main package until the project contains a `datapages.NewServer` call. `toxiproxy` configures the second address of the next section. Add `watch` settings for the proxy, TLS, compiler flags, or custom file watchers. Run `datapages watch --help` for command-line overrides.

## Test on a slow or failing network

`datapages watch` also serves the application through [Toxiproxy](https://github.com/Shopify/toxiproxy), which injects network faults, called toxics, into the connection of each request. The address of `--host` keeps serving without toxics. Watch mode prints the address of the proxy and of its settings page:

```
toxiproxy: http://localhost:7332 (settings: http://localhost:7333/)
```

```
browser -> watch.toxiproxy.host -> Toxiproxy -> --host (dev server) -> watch.app-host (application)
```

The scaffold writes the presets of Datapages and no toxics, which the settings page shows as the preset No throttling. The settings page sets the toxics of every endpoint and overrides them for 1 endpoint, such as a slow upload action or a stream that resets. A change applies to the next request and to open connections, such as an SSE stream, and lasts until watch mode stops. The save button writes the settings into the `watch.toxiproxy` section of `datapages.yaml` and leaves the rest of the file as it is. It drops comments inside the section, and it is disabled while the file holds the settings. The reload button discards the changes and reads `toxics`, `endpoints` and `presets` again. A change of `host` or `ui-host` applies when watch mode restarts.

`watch.toxiproxy` in `datapages.yaml` configures the proxy. Every key is optional:

```yaml
watch:
  toxiproxy:
    host: localhost:9090
    toxics:
      latency:
        downstream:
          latency: 300ms
          jitter: 100ms
      bandwidth:
        downstream:
          rate: 256
    endpoints:
      "POST /upload/{$}":
        bandwidth:
          upstream:
            rate: 16
        latency:
          downstream:
            disabled: true
      "POST /post/{slug}/like/{$}":
        http-error:
          downstream:
            status: 500
            toxicity: 0.5
```

| Key         | Value                                                                                                                                                                                                               |
| ----------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `disabled`  | `true` turns the proxy off.                                                                                                                                                                                         |
| `host`      | The address the proxy listens on. Default: the host of `--host` with the next port, `localhost:7332` for the default `--host`.                                                                                     |
| `ui-host`   | The address the settings page listens on. Default: the host of `host` with the next port, `localhost:7333` for the default `host`.                                                                                  |
| `toxics`    | The toxics of every endpoint.                                                                                                                                                                                       |
| `endpoints` | The toxics of 1 endpoint, keyed by its route pattern. A toxic of an endpoint replaces the global toxic of the same type and stream for that endpoint. The other global toxics apply to it.                          |
| `presets`   | Named sets of global toxics and endpoint overrides, each a `name`, its `toxics` and its `endpoints`, which the settings page applies. None without the key. No throttling and Offline are built in, which no preset can be named. |

Each toxic type takes a `downstream` toxic, which acts on the data the server sends, and an `upstream` toxic, which acts on the data the browser sends:

| Type         | Fields                                                     | Effect                                                                                                         |
| ------------ | ---------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| `latency`    | `latency`, `jitter`: durations in whole milliseconds       | Delays the data by `latency`, plus or minus up to `jitter`.                                                    |
| `bandwidth`  | `rate`: KB/s, at least 1                                   | Limits the rate of the data, which the requests of 1 browser share.                                            |
| `slow-close` | `delay`: duration in whole milliseconds                    | Delays closing the connection.                                                                                 |
| `timeout`    | `timeout`: duration in whole milliseconds                  | Stops all data and closes the connection after `timeout`. `0s` holds the connection until the browser gives up. |
| `reset-peer` | `timeout`: duration in whole milliseconds                  | Resets the connection with a TCP RST once data arrives, after `timeout`.                                       |
| `slicer`     | `average-size`, `size-variation`: bytes; `delay`: duration in whole microseconds | Splits the data into chunks of `average-size` bytes, plus or minus up to `size-variation`, and delays each chunk by `delay`. |
| `limit-data` | `bytes`: bytes                                             | Closes the connection after `bytes` bytes.                                                                     |
| `http-error` | `status`: HTTP status from 400 to 599                      | Answers with `status`. Upstream, the app never gets the request. Downstream, the app answers it and the browser gets `status` instead. |

Every toxic also takes:

- `toxicity`: the probability, from 0 to 1, that the toxic applies to a connection. Default 1. Each request opens a connection of its own, which makes it the share of requests the toxic applies to. The settings page shows it as "Requests affected", in percent.
- `disabled`: `true` turns the toxic off and keeps its other settings, which it may leave out. On an endpoint it turns off the global toxic of the same type and stream.

The proxy applies `http-error` itself, not Toxiproxy. It answers with the status and its standard text, the answer of the generated server to an error that no `PageError500`, `PageError404` or `RecoverError` handles. The toxic therefore stands for a failure in front of the app, such as a gateway or a crashed process. To test the error handling of the app, return an error from the handler. Upstream, the proxy answers without passing the request on, which skips the other toxics. Downstream, the request passes through the other toxics, and the proxy reads the response of the app to its end before it answers, which lets an action finish. The stream of a page does not end, so the proxy answers it once its head arrives, which ends the stream. With both set, a request the upstream toxic answers never reaches the downstream one.

The proxy also applies `bandwidth` itself. The requests of 1 browser share the rate, as the requests of 1 device share its network, and 2 downloads at a time take 2 times as long as 1. The proxy tells browsers apart by the cookie `datapages_toxiproxy`, which it sets on the first response and removes from the requests it passes to the app. The tabs of a browser share their cookies, and a private window has its own, which makes 2 windows 2 users on 2 networks. A client without cookies, such as curl, gets the rate per request. The requests of an endpoint that overrides `bandwidth` share the rate of the override among themselves and leave the global rate to the other requests. Cookies do not tell ports apart, so the dev server and the app at `--host` receive the cookie too.

A preset is a named set of global toxics and endpoint overrides. The preset combobox in the header of the settings page applies one, which makes the global toxics and the overrides those of the preset. It lists the built-in presets No throttling first and Offline last, and the presets of `datapages.yaml` between them. It shows the preset that gives every endpoint the toxics it gets, else "Custom". An override that changes nothing counts as none. Its last option opens a dialog that creates a preset from the current toxics and overrides or from a copy of a preset and applies it. The dialog copies the presets of `datapages.yaml`, which it marks with the name of the application, and the presets of Datapages. The button in the option of a preset of `datapages.yaml` deletes the preset after a confirmation. All of them are the network presets of Chrome DevTools, and `datapages init` writes the presets of Datapages into `presets`:

| Preset        | Toxics                                                                                   |
| ------------- | ---------------------------------------------------------------------------------------- |
| No throttling | Built in. None.                                                                          |
| Fast 4G       | Datapages. `latency` 165ms downstream, `bandwidth` 1013 KB/s downstream and 169 KB/s upstream. |
| Slow 4G       | Datapages. `latency` 563ms downstream, `bandwidth` 180 KB/s downstream and 84 KB/s upstream. |
| 3G            | Datapages. `latency` 2000ms downstream, `bandwidth` 50 KB/s downstream and upstream.     |
| Offline       | Built in. `reset-peer` with a `timeout` of 0s downstream and upstream. A request ends before the app gets it, an open stream at its next event. |

The values are those of Chrome rounded to whole milliseconds and KB/s. Chrome shares the bandwidth among the requests of a tab, the proxy among the requests of a browser. A preset of your own lists its toxics as `toxics` does and its overrides as `endpoints` does:

```yaml
watch:
  toxiproxy:
    presets:
      - name: Train
        toxics:
          latency:
            downstream:
              latency: 900ms
              jitter: 300ms
        endpoints:
          "POST /upload/{$}":
            bandwidth:
              upstream:
                rate: 16
```

An endpoint key is the [`net/http.ServeMux`](https://pkg.go.dev/net/http#ServeMux) pattern the generated server registers, method included:

| Endpoint                   | Pattern                      |
| -------------------------- | ---------------------------- |
| Index page                 | `GET /`                      |
| Page `/post/{slug}/`       | `GET /post/{slug}/{$}`       |
| Its stream                 | `GET /post/{slug}/_$/{$}`    |
| Its action `POSTLike`      | `POST /post/{slug}/like/{$}` |
| Assets under `/static/`    | `GET /static/`               |

The settings page lists the patterns of the application. The proxy finds the endpoint of a request the way the generated server does, and reads the endpoints again after every `datapages gen`. A request that matches no endpoint gets the global toxics. `datapages lint` reports an endpoint key of `endpoints` or of a preset that names no route of any application of the module, unless `disabled` is set, and `datapages watch` warns about one.

The proxy passes each request to the development server on a connection of its own, which applies `toxicity` and the toxics that end a connection per request. It keeps the `Host` header of the browser. A toxic that ends the connection to the development server ends the connection to the browser without a response. `/__templier/events`, which reloads the page after a rebuild, passes without toxics. With `watch.tls`, the proxy serves HTTPS with the same certificate.

The proxy connects to `--host`, which needs a fixed port. `host` and `ui-host` need ports other than those of `--host` and `watch.app-host`. With a `watch.toxiproxy` section, a problem that keeps the proxy from starting, such as a port in use, fails `datapages watch`. Without the section, `datapages watch` prints a warning and runs without the proxy. A `--host` with port 0 turns the proxy off, with a warning when `datapages.yaml` configures it.

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
