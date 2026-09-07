---
name: datapages-server
description: >-
  Configure the Datapages server entry point: NewServer type arguments,
  the message broker, server options, static assets, TLS and Prometheus metrics.
---

# Server entry point

`datapages gen` writes the `main.go` of the server command on the first run.
After that the file is yours and is never regenerated.

## NewServer

The type arguments are configuration: `datapages gen` reads them to find the
app package and where to generate into.

```go
s, err := datapages.NewServer[
	app.App,                     // App
	app.SessionData,             // or datapages.DisableSessions
	datapages.DisablePrometheus, // or datapages.EnablePrometheus
	datapagesgen.Server,         // the datapagesgen of that app package
](&a, broker, opts...) // the app is passed by pointer
```

Keep the call inside the module and import `datapages` under a qualifier: the
scan matches the call by its qualifier and rejects a dot import. Generated code
always lands in `datapagesgen` directly under its app package, so one module
can build several apps (`datapages watch --app frontend` runs one of them).

`datapages.EnablePrometheus` requires `WithPrometheus`, `DisablePrometheus`
rejects it.

## Broker

Always required: it carries events between instances and fans out SSE. Use
`modules/messaging/natscore`. `modules/messaging/inmem` is for a single
instance only.

## Options

```go
var opts []datapages.ServerOption
opts = append(opts,
	datapages.WithLogger(slog.Default()),
	datapages.WithMiddleware(mw),
	datapages.WithSessions(datapages.SessionsConfig{}),
	datapages.WithSessionManager[app.SessionData](mgr),
	datapages.WithAssets(app.StaticFS),
	datapages.WithHTTPServer(&http.Server{ReadHeaderTimeout: 10 * time.Second}),
	datapages.WithDatastarJS("https://cdn.example.com/datastar.js"),
	datapages.WithBodySizeLimit(1<<20),
	datapages.WithLogSampling(datapages.LogSamplingConfig{Interval: time.Minute}),
	datapages.WithPrometheus(datapages.PrometheusConfig{Host: ":9091"}),
)
```

`WithBodySizeLimit` caps the request body of an action, which is what limits
the signals a page may send. `WithLogSampling` throttles the framework's own
warnings, not the application's. `WithHTTPServer` keeps every field but `Addr`
and `Handler`. The session cookie
carries `Secure`: set `DisableSecureCookie` only for a deployment that is plain
HTTP end to end, where the browser would drop it. `datapages.IsDevMode()`
reports the dev server, which is a reason to log at `slog.LevelDebug`.

```go
s.ListenAndServe(ctx, "localhost:8080")
s.ListenAndServeTLS(ctx, "localhost:8443", certPath, keyPath)
```

## Static assets

An `embed.FS` in the app package turns file serving on. Its doc comment names
the URL prefix, its directive names the directory.

```go
// StaticFS is /static/
//go:embed static/*
var StaticFS embed.FS
```

One such variable per app package, no more. The URL prefix has to start and
end with `/` and cannot be `/` alone. The directive has to name exactly one
directory inside the app package.

`datapages.WithAssets(app.StaticFS)` carries only the filesystem; the generated
code supplies the prefix, the subdirectory and the dev-mode disk path. In dev
mode the files come from disk with caching off, so no rebuild is needed. An app
package that declares no assets rejects the option.

Reference files with `assets.Path("style.css")` from the generated `assets`
package, or `href.Asset("style.css")` inside an `<a href>`. A hardcoded path is
a lint error.
