---
name: datapages-server
description: >-
  Configure the Datapages server entry point: NewServer type arguments,
  the message broker, server options, static assets, TLS and Prometheus metrics.
---

# Server entry point

Read `datapages` first for the build loop, hard rules and naming conventions.

`datapages gen` writes the `main.go` of the server command on the first run. After that the file is yours and is never regenerated.

## NewServer

The type arguments are configuration: `datapages gen` reads them to find the app package and where to generate into.

```go
s, err := datapages.NewServer[
	app.App,                     // App
	app.SessionData,             // or datapages.DisableSessions
	datapages.DisablePrometheus, // or datapages.EnablePrometheus
	datapagesgen.Server,         // the datapagesgen of that app package
](&a, broker, opts...) // the app is passed by pointer
```

Keep the call inside the module and import `datapages` under a qualifier: the scan matches the call by its qualifier and rejects a dot import. Generated code always lands in `datapagesgen` directly under its app package, so one module can build several apps (`datapages watch --app frontend` runs one of them).

`datapages.EnablePrometheus` requires `WithPrometheus`, `DisablePrometheus` rejects it.

## Broker

Always required: it carries events between instances and fans out SSE. Use `modules/messaging/natscore`. `modules/messaging/inmem` is for a single instance only. The scaffolded server uses NATS; start it with `make up` before running the server.

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

`WithBodySizeLimit` caps the request body of an action, which is what limits the signals a page may send. Its default is 1 MiB; an over-limit request returns 400 while reading signals. `WithLogSampling` throttles the framework's own warnings, not the application's. `WithHTTPServer` keeps every field but `Addr` and `Handler`. Keep `WriteTimeout` at zero: a nonzero value ends long-lived SSE streams. `WithPrometheus` starts a second HTTP server on the configured host for `/metrics`. `WithShutdownTimeout` caps how long `ListenAndServe` waits after context cancellation for in-flight requests, open SSE streams and `StreamClose` hooks; on expiry it logs the shutdown error and returns. The session cookie carries `Secure`: set `DisableSecureCookie` only for a deployment that is plain HTTP end to end, where the browser would drop it. `datapages.IsDevMode()` reports the dev server; `DATAPAGES_DEV_MODE` and `TEMPL_DEV_MODE` enable dev behavior, which is a reason to log at `slog.LevelDebug`.

If `WithMiddleware` adds a `Content-Security-Policy`, stateful pages require `script-src 'unsafe-inline'` for the generated instance-ID script.

```go
s.ListenAndServe(ctx, "localhost:8080")
s.ListenAndServeTLS(ctx, "localhost:8443", certPath, keyPath)
```

## Static assets

An `embed.FS` in the app package turns file serving on. Its doc comment names the URL prefix, its directive names the directory.

```go
// StaticFS is /static/
//go:embed static/*
var StaticFS embed.FS
```

One such variable per app package, no more. The URL prefix has to start and end with `/` and cannot be `/` alone. The directive has to name exactly one directory inside the app package.

`datapages.WithAssets(app.StaticFS, false)` carries the filesystem and whether directory browsing is allowed. The generated code supplies the prefix, the subdirectory and the dev-mode disk path. `WithAssetsFS` accepts an `http.FileSystem` instead of an `embed.FS`. In dev mode the files come from disk with caching off, so no rebuild is needed. An app package that declares no assets rejects the option.

Datapages sends neither `Cache-Control` nor `ETag` for assets unless `WithAssetsCache` is configured. Its zero config sends `Cache-Control: public, max-age=0` plus an ETag, so the browser revalidates every request and an unchanged file answers 304 with no body. Set `MaxAge` only when the asset URL changes with its content, such as a name carrying a build hash; otherwise the browser keeps stale content until the age expires. `Immutable` stops reloads from revalidating a fresh response and needs a positive `MaxAge`. `CacheControl` sets the header value directly and excludes `MaxAge` and `Immutable`. `DisableETag` omits the ETag, `Disabled` omits both headers.

The server computes an ETag on a file's first request and keeps it for the process lifetime, which matches the immutable `embed.FS` behind `WithAssets`. Set `DisableETag` for a `WithAssetsFS` file system whose files change while the server runs. Dev mode ignores the option and keeps answering `Cache-Control: no-store`.

Reference files with `assets.Path("style.css")` from the generated `assets` package, or `href.Asset("style.css")` inside an `<a href>`. A hardcoded path is a lint error.

## datapages.yaml

`cmd` names the command scaffolded when there is no `NewServer` call; it defaults to `cmd/server`. `watch` configures `datapages watch`:

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
