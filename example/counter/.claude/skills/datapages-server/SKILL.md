---
name: datapages-server
description: >-
  Configure the Datapages server entry point: NewServer type arguments,
  the message broker, server options, static assets, TLS and Prometheus metrics.
---

# Server entry point

Read `datapages` first for the build loop, hard rules and naming conventions.

On its first run, `datapages gen` writes the server command's `main.go`. It does not regenerate that file on later runs.

## NewServer

`datapages gen` reads the type arguments to find the app package and output directory.

```go
s, err := datapages.NewServer[
	app.App,                     // App
	app.SessionData,             // or datapages.DisableSessions
	datapages.DisablePrometheus, // or datapages.EnablePrometheus
	datapagesgen.Server,         // the datapagesgen of that app package
](&a, broker, opts...) // the app is passed by pointer
```

Keep the call inside the module. Import `datapages` with a package name because the scanner rejects dot imports. Generated code is always in `datapagesgen` directly below its app package. One module can contain several apps. Run one with `datapages watch --app frontend`.

`datapages.EnablePrometheus` requires `WithPrometheus`, `DisablePrometheus` rejects it.

## Broker

A broker is always required. It sends events between server instances and to SSE subscribers. Use `modules/messaging/natscore` for multiple instances. Use `modules/messaging/inmem` only for one instance. The generated server uses NATS. Start it with `make up` before starting the server.

## Options

```go
var opts []datapages.ServerOption
opts = append(opts,
	datapages.WithLogger(slog.Default()),
	datapages.WithMiddleware(mw),
	datapages.WithSessions(datapages.SessionsConfig{}),
	datapages.WithSessionManager[app.SessionData](mgr),
	datapages.WithAssets(app.StaticFS, false),
	datapages.WithHTTPServer(&http.Server{ReadHeaderTimeout: 10 * time.Second}),
	datapages.WithDatastarJS("https://cdn.example.com/datastar.js"),
	datapages.WithShutdownTimeout(30*time.Second),
	datapages.WithAssetsCache(datapages.AssetsCacheConfig{}),
	datapages.WithBodySizeLimit(1<<20),
	datapages.WithLogSampling(datapages.LogSamplingConfig{Interval: time.Minute}),
	datapages.WithPrometheus(datapages.PrometheusConfig{Host: ":9091"}),
)
```

`WithBodySizeLimit` limits an action request body, including its signals. The default is 1 MiB. An over-limit request returns 400 while reading signals. `WithLogSampling` limits repeated framework warnings, not application logs. `WithHTTPServer` uses every supplied field except `Addr` and `Handler`. Keep `WriteTimeout` at zero because a nonzero value ends long-lived SSE streams.

`WithPrometheus` starts a second HTTP server that serves `/metrics` on the configured host. `WithShutdownTimeout` limits how long `ListenAndServe` waits after context cancellation for requests, SSE streams and `StreamClose` hooks. When the timeout expires, Datapages logs the shutdown error and returns.

The session cookie uses the `Secure` attribute. Set `DisableSecureCookie` only when the complete deployment uses plain HTTP, where the browser would reject the secure cookie. `datapages.IsDevMode()` reports whether the dev server is active. `DATAPAGES_DEV_MODE` and `TEMPL_DEV_MODE` enable development behavior. Log at `slog.LevelDebug` when you need to inspect that behavior.

Declaring `PageOffline` generates `datapagesgen.WithOffline(offline.Config{...})`. The option serves the service worker. See `datapages-offline`.

If `WithMiddleware` adds a `Content-Security-Policy`, it must allow `script-src 'unsafe-inline' 'unsafe-eval'`. Datapages writes the CSRF script and the instance ID script inline. Datastar compiles every `data-*` expression at run time.

`WithCSPNonce(func(r *http.Request) string)` replaces both allowances with a nonce. The application mints the nonce and puts it in its own policy header. Datapages reads it back, writes it on the `html` element as `data-nonce` and on every script it writes. Mint the nonce in middleware and store it in the request context: Datapages calls the function several times per response and every call with the same request must return the same value. `data-nonce` turns on Datastar's CSP mode, which compiles expressions through a nonced script element instead of `Function`. It needs Datastar 1.0.3 or later. The nonce must differ per response.

`WithOffline` passes the nonce to the offline module regardless of option order. The nonce cannot reach a page served from the service worker's cache; see `datapages-offline`.

```go
s.ListenAndServe(ctx, "localhost:8080")
s.ListenAndServeTLS(ctx, "localhost:8443", certPath, keyPath)
```

## Static assets

An `embed.FS` in the app package enables file serving. Its doc comment defines the URL prefix. Its directive defines the source directory.

```go
// StaticFS is /static/
//go:embed static/*
var StaticFS embed.FS
```

Declare at most one such variable in an app package. The URL prefix must start and end with `/` and must not be `/`. The directive must name exactly one directory inside the app package.

`datapages.WithAssets(app.StaticFS, false)` supplies the filesystem and the directory-browsing setting. Generated code supplies the URL prefix, source directory and development disk path. In development mode, Datapages reads files from disk and disables caching. Asset changes then need no rebuild. The server rejects `WithAssets` when the app declares no assets.

`WithAssetsFS` accepts an `http.FileSystem` and overrides `WithAssets`. Datapages uses that filesystem as-is: it does not extract the declared source directory or replace it in development mode. The app's declared URL prefix still determines where files are served. Without an asset declaration, the option is accepted but no asset route is registered.

Without `WithAssetsCache`, Datapages sends neither `Cache-Control` nor `ETag` for assets. A zero `AssetsCacheConfig` sends `Cache-Control: public, max-age=0` and an ETag. The browser revalidates every request. An unchanged file returns 304 with an empty body.

Set `MaxAge` only when an asset URL changes with its content, such as when the file name contains a build hash. Otherwise a browser can use stale content until the age expires. `Immutable` prevents reloads from revalidating a fresh response and requires a positive `MaxAge`. `CacheControl` sets the header directly and cannot be combined with `MaxAge` or `Immutable`. `DisableETag` omits the ETag. `Disabled` omits both headers.

The server computes an ETag on a file's first request and keeps it until the process exits. This is correct for the immutable `embed.FS` used by `WithAssets`. Set `DisableETag` for a `WithAssetsFS` file system whose files can change while the server runs. Development mode ignores this option and always sends `Cache-Control: no-store`.

Use `assets.Path("style.css")` from the generated `assets` package to reference a file. Use `href.Asset("style.css")` inside an `<a href>`. The linter checks app-internal URLs in `<a href>` and Datastar action attributes. It does not check asset URLs in `<link>` or `<script>` elements.

## datapages.yaml

`cmd` names the command to create when there is no `NewServer` call. It defaults to `cmd/server`. `watch` configures `datapages watch`:

| key under `watch` | use |
| ----------------- | --- |
| `app-host`, `proxy-timeout`, `debounce` | dev server URL and timing |
| `format`, `lint` | format or lint during rebuilds |
| `exclude`, `watcher-ignore` | paths excluded from source scanning or file watching |
| `flags`, `dir-work` | command flags and working directory |
| `log.level`, `log.clear-on`, `log.print-js-debug-logs` | watcher output |
| `tls.cert`, `tls.key` | certificate and key for dev-server TLS |
| `compiler.tags`, `compiler.env` | Go build tags and environment; `compiler` also accepts Go compiler flags |
| `custom-watchers` | extra watchers with include/exclude patterns, command and rebuild action |

See `datapages watch --help` for CLI flags.

<!-- written by datapages sha256:09d9723416d70b71 -->
