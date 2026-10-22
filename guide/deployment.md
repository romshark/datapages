# Deployment

A deployment runs 1 or more copies of the server binary. Configure the binary in `cmd/server/main.go`, terminate TLS in the server or a reverse proxy, and collect its metrics and logs.

## Build the binary

Build the Go server binary from `cmd/server`. It contains the compiled templates, the generated `datapagesgen` package and the app's embedded files. At run time, the scaffold also needs its environment variables and a NATS server.

Generate the code, then build:

```sh
templ generate
datapages gen
go build -o server ./cmd/server
```

The scaffold's `.github/workflows/ci.yml` runs both generators and fails when their output differs from the committed code. `make build` runs the last step and writes `cmd/server/server`. The scaffold's dependencies need no cgo. For a static Linux binary, run `CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server`.

Do not set `DATAPAGES_DEV_MODE` or `TEMPL_DEV_MODE` in production. Either variable turns on dev mode, in which the server reads static assets from the source directory and sends pages and assets uncompressed. The server logs `dev mode is on` at startup when either is set. See [Dev mode](assets.md#dev-mode).

## Configure the server

`datapages init` writes `cmd/server/main.go` once and never regenerates it. Connect the broker and the session manager there, create the repositories and clients that `App` holds, and pass options to `datapages.NewServer`.

### Environment variables

The scaffold's `main.go` reads these variables:

| Variable | Default | Use |
| -------- | ------- | --- |
| `HOST` | `localhost` | Address to listen on |
| `PORT` | `8080` | Port to listen on |
| `NATS_URL` | required | NATS server of the broker and the session manager |
| `SESSION_ENCRYPTION_KEY` | required | 16-byte key in hex that encrypts session cookies |
| `HOST_METRICS` | `localhost` | Address of the Prometheus metrics server |
| `PORT_METRICS` | `9090` | Port of the metrics server |
| `PATH_TLS_CERT`, `PATH_TLS_KEY` | empty | Certificate and key files, see [Serve TLS](#serve-tls) |

- `localhost` accepts connections from the same machine only. Set `HOST=0.0.0.0` in a container.
- `main` first reads `.env` from the working directory and sets each variable that the environment leaves empty. `datapages init` writes `.env` with `NATS_URL` and a random `SESSION_ENCRYPTION_KEY` and adds the file to `.gitignore`. In a deployment, set the variables in the environment.
- The process exits with status 2 when `NATS_URL` or `SESSION_ENCRYPTION_KEY` is unset, and with status 1 when another setup step fails. A listen error, such as a port in use, is logged and the process exits with status 0. A supervisor that restarts only failed processes does not restart it. [The complete example](#complete-example) exits with status 1 instead.
- A `main.go` written for an app without a session type reads no `SESSION_ENCRYPTION_KEY`. `datapages init --prometheus=false` writes a `main.go` without the metrics variables.

### Server options

`datapages.NewServer` applies its [`ServerOption`](https://pkg.go.dev/github.com/romshark/datapages#ServerOption) values in order. The scaffold passes the access logger, `WithSessions`, `WithPrometheus` and the session manager, and `WithAssets` when the app declares static files. Append others to `opts` before the `NewServer` call.

Options of the process:

| Option | Default | Sets |
| ------ | ------- | ---- |
| [`WithHTTPServer`](https://pkg.go.dev/github.com/romshark/datapages#WithHTTPServer) | see [HTTP server](#http-server) | The `net/http` server with its timeouts and `TLSConfig` |
| [`WithShutdownTimeout`](https://pkg.go.dev/github.com/romshark/datapages#WithShutdownTimeout) | 10s | How long shutdown waits for requests and `StreamClose` hooks |
| [`WithBuildID`](https://pkg.go.dev/github.com/romshark/datapages#WithBuildID) | hash of the executable | The ID that makes pages of another build reload |
| [`WithLogger`](https://pkg.go.dev/github.com/romshark/datapages#WithLogger) | JSON on stderr, level INFO | The logger of the server |
| [`WithLogSampling`](https://pkg.go.dev/github.com/romshark/datapages#WithLogSampling) | 1 of each warning per minute | How often a repeated framework warning is logged |
| [`WithMiddleware`](https://pkg.go.dev/github.com/romshark/datapages#WithMiddleware) | none | Handlers that wrap the router, in the order given |
| [`WithPrometheus`](https://pkg.go.dev/github.com/romshark/datapages#WithPrometheus) | none | The metrics server, required by `datapages.EnablePrometheus` |

Options for requests and responses:

| Option | Default | Sets |
| ------ | ------- | ---- |
| [`WithBodySizeLimit`](https://pkg.go.dev/github.com/romshark/datapages#WithBodySizeLimit) | 1 MiB | The body limit of actions that take `Signals` |
| [`WithFileSendTimeout`](https://pkg.go.dev/github.com/romshark/datapages#WithFileSendTimeout) | 1 minute | How long a write of a `datapages.File` or an asset may block |
| [`WithResponseCompression`](https://pkg.go.dev/github.com/romshark/datapages#WithResponseCompression) | zstd, brotli, gzip from 256 bytes | Compression of page, action and error bodies |
| [`WithNoResponseCompression`](https://pkg.go.dev/github.com/romshark/datapages#WithNoResponseCompression) | | Turns that compression off |
| [`WithSSECompression`](https://pkg.go.dev/github.com/romshark/datapages#WithSSECompression) | zstd, brotli, gzip with 1 MiB windows | Compression of streams |
| [`WithNoSSECompression`](https://pkg.go.dev/github.com/romshark/datapages#WithNoSSECompression) | | Turns stream compression off |

Options for static assets:

| Option | Default | Sets |
| ------ | ------- | ---- |
| [`WithAssets`](https://pkg.go.dev/github.com/romshark/datapages#WithAssets) | none | The `embed.FS` of the app's static files |
| [`WithAssetsFS`](https://pkg.go.dev/github.com/romshark/datapages#WithAssetsFS) | none | An `http.FileSystem` that replaces it |
| [`WithAssetsCache`](https://pkg.go.dev/github.com/romshark/datapages#WithAssetsCache) | `Cache-Control: public, max-age=0`, no ETag | The cache headers of assets |
| [`WithAssetsCompression`](https://pkg.go.dev/github.com/romshark/datapages#WithAssetsCompression) | brotli 6 or gzip 9, kept in memory | The asset compressor |
| [`WithNoAssetsCompression`](https://pkg.go.dev/github.com/romshark/datapages#WithNoAssetsCompression) | | Turns asset compression off |

Options for sessions and security:

| Option | Default | Sets |
| ------ | ------- | ---- |
| [`WithSessionManager`](https://pkg.go.dev/github.com/romshark/datapages#WithSessionManager) | required with a session type | The store of sessions |
| [`WithSessions`](https://pkg.go.dev/github.com/romshark/datapages#WithSessions) | cookie `sessiontoken`, `Secure`, `HttpOnly` | The session cookie and the token generator |
| [`WithCSRFProtection`](https://pkg.go.dev/github.com/romshark/datapages#WithCSRFProtection) | on with sessions | Other CSRF tokens, or protection off |
| [`WithCSPNonce`](https://pkg.go.dev/github.com/romshark/datapages#WithCSPNonce) | none | The Content-Security-Policy nonce of a request |

Options for pages:

| Option | Default | Sets |
| ------ | ------- | ---- |
| [`WithDatastarJS`](https://pkg.go.dev/github.com/romshark/datapages#WithDatastarJS) | Datastar 1.0.4 from `cdn.jsdelivr.net` | The URL of the Datastar bundle |
| [`WithStateConfig`](https://pkg.go.dev/github.com/romshark/datapages#WithStateConfig) | 10,000 instances | The per-tab state limit of the server |

The default level of `WithLogger` is DEBUG in dev mode. Declaring `PageOffline` generates a `datapagesgen.WithOffline` option, see [Offline and shims](offline.md).

### HTTP server

Without `WithHTTPServer`, the server uses these values:

| Field | Value |
| ----- | ----- |
| `ReadTimeout` | 30s |
| `ReadHeaderTimeout` | 5s |
| `WriteTimeout` | 0, no limit |
| `IdleTimeout` | 60s |
| `MaxHeaderBytes` | 1 MiB |

`ReadTimeout` limits reading a request and its body. It does not end a stream that stays open longer. A request body that takes longer to arrive fails, which [`example/file-upload`](../example/file-upload/) avoids by moving the read deadline while the bytes of an upload arrive.

`WithHTTPServer` replaces this server, values included. Datapages sets `Addr`, `Handler` and `BaseContext`, sets `ErrorLog` when it is nil and uses every other field as given. `net/http` reads a zero timeout as no limit. Copy the values to keep from the generated package:

```go title="cmd/server/main.go"
opts = append(opts, datapages.WithHTTPServer(&http.Server{
	ReadTimeout:       datapagesgen.DefaultHTTPReadTimeout,
	ReadHeaderTimeout: datapagesgen.DefaultHTTPReadHeaderTimeout,
	IdleTimeout:       datapagesgen.DefaultHTTPIdleTimeout,
	MaxHeaderBytes:    datapagesgen.DefaultHTTPMaxHeaderBytes,
	TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS13},
}))
```

Keep `WriteTimeout` at 0. The write deadline counts from the request, not from the last event, and a stream fails at the first event it writes after `WriteTimeout`. The server logs an `i/o timeout` error and the page reconnects.

## Choose modules

The broker and the session manager decide whether the app can run as more than 1 process:

| Need | 1 process | Several processes |
| ---- | --------- | ----------------- |
| Events | `modules/messaging/inmem` | `modules/messaging/natscore` |
| Sessions | `modules/sessions/inmem` | `modules/sessions/natskv` |

The scaffold uses the NATS modules. A restart of the server process loses:

- `messaging/inmem` and `messaging/natscore`: nothing, because neither stores events. An event dispatched while a page has no open stream, as during a restart, never reaches that page. See [Event delivery](../SPECIFICATION.md#event-delivery).
- `sessions/inmem`: every session. The cookies then name no session, and the server clears them and treats every visitor as a guest.
- `sessions/natskv`: nothing. Sessions live in the JetStream key-value bucket `SESSIONS` of the NATS server, which needs JetStream enabled. The scaffold's `compose.yaml` starts NATS with `-js`.
- Per-tab state, with any modules: every instance. Each tab reconnects with zeroed state, and an action sent before the reconnect receives 409 and reloads the page. See [Per-tab state](state.md).

For 1 process without NATS, replace the `connectNATS()` call of the scaffold with the in-memory modules:

```go title="cmd/server/main.go"
messageBroker := inmem.New(0)
sessionManager := sessinmem.New[struct{}](
	sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
)
```

The imports are `github.com/romshark/datapages/modules/messaging/inmem` and `sessinmem "github.com/romshark/datapages/modules/sessions/inmem"`. `inmem.New(0)` buffers 16 events per stream. Use in-memory sessions only where signing every visitor out at each restart is acceptable. [`example/sqlitesessions`](../example/sqlitesessions/) implements a `sessions.Manager` on SQLite, which keeps the sessions of 1 process across restarts.

Datapages deletes an expired session when a handler reads it. Call `DeleteExpired` of the session manager on a ticker for the sessions that nobody sends again. See [Sessions and auth](sessions.md).

## Run several processes

Behind a load balancer:

- Connect every process to the same NATS server with `natscore` and `natskv`. An event dispatched in 1 process then reaches the streams of every process, and every process reads the same sessions.
- Give every process the same `SESSION_ENCRYPTION_KEY`. `natskv` encrypts the cookie with it, and a process with another key clears the cookie as stale, which signs the visitor out. CSRF tokens derive from the session token and need no shared secret.
- Run the same binary in every process, or pass the same [`WithBuildID`](#deploy-a-new-version). A process answers the requests of a page from another build with `205 Reset Content`, which reloads the page.
- Send the stream and the actions of 1 tab to 1 process when a page uses per-tab state. The state lives in the process that serves the tab's stream. Route by a hash of the `Datapages-Instance` request header, by session or by an affinity cookie. Page loads carry no `Datapages-Instance` and may reach any process. An action that reaches another process receives `409 Conflict` with `Datapages-Retry: reconnect`, and the page reloads.

Each process enforces its own `WithStateConfig` limit and serves its own metrics.

## Deploy a new version

An open page keeps the document of the build that rendered it. Each document carries the build ID of its server in `<meta name="datapages-build">`, and an inline script sends it in the `Datapages-Build` header with every same-origin Datastar request. A server of another build answers `205 Reset Content` without running the handler, and the script reloads the page:

- after a random delay of up to 2s when the request was a stream reconnect,
- at once when it was an action, which is not applied,
- after 1s, 2s, 4s and up to 30s when reloads repeat without a successful request.

A reload discards signals, form input and focus.

The build ID defaults to the first 16 bytes of the SHA-256 hash of the executable, in hex. Any change to the binary changes it. The server logs it as `build` when it starts:

```json
{"time":"2026-10-22T14:40:42.586217+02:00","level":"INFO","msg":"listening HTTP","addr":"127.0.0.1:18810","build":"4bbaa20b45ae57a327126137317d02bd"}
```

Set `WithBuildID` when the processes of 1 release run different binaries, such as builds for amd64 and arm64. Give every release its own ID, such as the commit:

```go title="cmd/server/main.go"
// buildID is set with -ldflags "-X main.buildID=<release>".
// Empty keeps the default, a hash of the executable.
var buildID string
```

```go title="cmd/server/main.go"
if buildID != "" {
	opts = append(opts, datapages.WithBuildID(buildID))
}
```

```sh
go build -ldflags "-X main.buildID=$(git rev-parse --short HEAD)" -o server ./cmd/server
```

An ID has 1 to 128 printable ASCII characters without spaces. Every document contains it, which makes it public.

During a rolling deployment, old and new processes serve at the same time. Route each client to 1 build until the old processes stop, for example with session affinity. A tab whose requests alternate between builds reloads at every switch.

Shutdown resets every open stream. Datastar reconnects after 1s and doubles the delay up to 30s, without a limit, also after the `502` of a proxy while no process runs. A stream that reaches the same build continues, and a stream that reaches another build reloads its page. See [Deployments](../SPECIFICATION.md#deployments).

## Shut down gracefully

The scaffold cancels the context of `ListenAndServe` on SIGINT and SIGTERM:

```go title="cmd/server/main.go"
ctx, cancel := signal.NotifyContext(context.Background(),
	os.Interrupt, syscall.SIGTERM)
defer cancel()
```

Docker, Kubernetes and systemd stop a process with SIGTERM. Without the signal context, SIGTERM ends the process without a shutdown. When the context ends, the server:

1. logs `server shutdown initiated` and stops accepting connections, also on the metrics server,
2. runs the `StreamClose` hook of each open stream and resets its connection, which makes the page reconnect,
3. waits for the requests in progress,
4. returns nil from `ListenAndServe`.

The wait ends after 10s by default. Set another limit:

```go title="cmd/server/main.go"
opts = append(opts, datapages.WithShutdownTimeout(20*time.Second))
```

When the limit passes, the server logs `shutting down` with the error `context deadline exceeded` and `ListenAndServe` returns. Requests still running end when the process exits. Keep the limit below the time the platform waits before it kills the process, such as the 30s default of Kubernetes `terminationGracePeriodSeconds` and the 10s default of `docker stop`.

Middleware added with `WithMiddleware` that recovers panics must re-panic `http.ErrAbortHandler`. Shutdown resets streams with that panic.

## Serve TLS

Terminate TLS in the server process or in front of it.

In the process, set `PATH_TLS_CERT` and `PATH_TLS_KEY`. The scaffold then calls `ListenAndServeTLS(ctx, addr, certFile, keyFile)`, which loads the files once at startup. To renew certificates without a restart, set `TLSConfig.GetCertificate` through [`WithHTTPServer`](#http-server) and pass empty file names. `ListenAndServeTLS` then takes the certificates from the `TLSConfig`. Set the minimum TLS version and the other TLS settings in the same `TLSConfig`.

In front of the process, a reverse proxy or a load balancer terminates TLS and forwards plain HTTP. The session cookie keeps its `Secure` attribute, which is correct because the browser talks HTTPS. Set `SessionsConfig.DisableSecureCookie` only when the whole deployment uses plain HTTP.

Serve browsers over HTTP/2. Over HTTP/1.1, a browser opens at most 6 connections to 1 host, and each open stream holds 1 of them. Over HTTP/2, the streams and requests of a browser share 1 connection. `ListenAndServeTLS` negotiates HTTP/2. Behind a proxy, enable HTTP/2 between the browser and the proxy.

The offline service worker runs only on HTTPS or `localhost`. See [Use TLS](security.md#use-tls) for HSTS and the cookie attributes.

## Run behind a reverse proxy

Each open page with a stream holds 1 request open, a `GET` of the page path followed by `_$/`, answered with `text/event-stream`. The server writes each event when it happens and nothing between events. The proxy must:

- Pass each event on at once. nginx buffers proxied responses unless `proxy_buffering off` applies or the response carries `X-Accel-Buffering: no`. The middleware below adds the header to streams.
- Keep quiet streams open. A proxy that closes a connection after a time without data, as nginx does after `proxy_read_timeout` (60s by default), ends every stream that has no event for that long. The page reconnects after 1s, but it misses the events dispatched in between and its per-tab state starts zeroed. Set the timeout above the longest quiet time of a page, such as 1 hour.
- Forward the `Host` header of the browser. For browsers that send no `Sec-Fetch-Site`, the cross-origin check of form actions compares `Origin` with `Host` and answers 403 when they differ.
- Forward the headers that Datapages reads and writes, `Datastar-Request`, `Datapages-Build`, `Datapages-Instance` and `X-CSRF-Token` in requests and `Datapages-Build`, `Datapages-Instance` and `Datapages-Retry` in responses.
- Send the stream and the actions of 1 tab to 1 process when pages use per-tab state, see [Run several processes](#run-several-processes).

```go title="cmd/server/main.go"
// streamsUnbuffered asks nginx to pass the events of each stream at once.
func streamsUnbuffered(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/_$/") {
			w.Header().Set("X-Accel-Buffering", "no")
		}
		next.ServeHTTP(w, r)
	})
}
```

Add it with `datapages.WithMiddleware(streamsUnbuffered)`.

Datapages compresses pages, action responses, streams and assets itself and sends `Vary: Accept-Encoding`. A compressing proxy passes these responses on unchanged. A proxy that edits bodies, such as nginx with `sub_filter`, needs them uncompressed. See [Choose where compression runs](performance.md#choose-where-compression-runs).

## Collect Prometheus metrics

The third type argument of `NewServer` decides whether the server counts metrics. `datapages.EnablePrometheus`, which the scaffold uses, requires `WithPrometheus`, and `datapages.DisablePrometheus` rejects it. `datapages gen` reads the type argument, and a change takes effect at the next `datapages gen`. `datapages init --prometheus=false` writes `DisablePrometheus`.

`WithPrometheus` starts a second HTTP server, which serves `/metrics` and answers 404 to other paths. It starts and stops with `ListenAndServe`. The scaffold binds it to `HOST_METRICS` and `PORT_METRICS`, `localhost:9090` by default. Keep it out of reach of the public network.

`PrometheusConfig.Collectors` registers metrics of the application:

```go title="cmd/server/main.go"
var ordersPlaced = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "shop_orders_placed_total",
	Help: "Orders placed.",
})
```

```go title="cmd/server/main.go"
datapages.WithPrometheus(datapages.PrometheusConfig{
	Host:       "localhost:9090",
	Collectors: []prometheus.Collector{ordersPlaced},
})
```

`Registerer` and `Gatherer` default to the default registry of `github.com/prometheus/client_golang`, which also exports Go runtime and process metrics such as `go_goroutines`. `Handler` replaces the handler of `/metrics`. Datapages exports:

| Metric | Type | Labels | Counts |
| ------ | ---- | ------ | ------ |
| `datapages_http_requests_total` | counter | `method`, `path`, `status` | Requests, streams included |
| `datapages_http_request_duration_seconds` | histogram | `method`, `path` | Request duration, streams excluded |
| `datapages_http_in_flight_requests` | gauge | | Requests in progress, streams excluded |
| `datapages_http_sse_connections` | gauge | | Open streams |
| `datapages_sse_connection_duration_seconds` | histogram | | Stream lifetime |
| `datapages_sse_disconnects_total` | counter | `reason` | Stream ends by `close`, `expired`, `client` or `shutdown` |
| `datapages_event_broker_publishes_by_kind_total` | counter | `kind` | Dispatched events |
| `datapages_event_broker_deliveries_dropped_total` | counter | | Events dropped for a full stream buffer |
| `datapages_internal_errors_recovered_total` | counter | | Errors that `RecoverError` answered |
| `datapages_internal_errors_not_recovered_total` | counter | | Errors answered with an error response |
| `datapages_action_options_dropped_total` | counter | `option` | Action options dropped for an invalid value |
| `datapages_session_creations_total` | counter | `result` | Sessions created, by `success` or `error` |
| `datapages_session_closures_total` | counter | `result` | Sessions closed, by `success` or `error` |
| `datapages_session_reads_total` | counter | `result` | Cookie reads, by `valid`, `none`, `stale`, `expired` or `error` |
| `datapages_state_instances` | gauge | | Live per-tab state instances |

`path` holds the route pattern, such as `GET /account/{$}`, not the URL, which keeps the number of series bounded. A URL that no other route matches counts under `GET /` of `PageIndex`. A request that matches no route at all counts as `<unmatched>`, and a method outside the standard ones as `<other>`. `kind` is the subject that the event type declares, such as `messaging.sent`. The `reason` values `close` and `expired` count the streams that the close or the expiry of their session ended.

## Log requests

`WithLogger` sets the logger of the server. The default writes JSON to stderr at level INFO, and at DEBUG in dev mode. At INFO, the server logs its start with `addr` and `build`, the start of a shutdown and errors. At DEBUG, it adds the cause of each 400 response and the requests of pages from another build.

The scaffold's `withAccessLogger` adds a middleware that logs the method and path of each request at INFO before the handler runs. To log the status and the duration, wrap the `http.ResponseWriter` and give the wrapper an `Unwrap` method. Streams flush through `http.ResponseController`, which reaches the writer underneath only through `Unwrap`. Without it every stream fails.

```go title="cmd/server/main.go"
// statusRecorder records the status code of a response.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach the Flush of the writer
// underneath, which every stream calls.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func withAccessLogger(opts *[]datapages.ServerOption) {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	*opts = append(*opts, datapages.WithMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			if rec.status == 0 {
				rec.status = http.StatusOK
			}
			logger.Info("access",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Duration("duration", time.Since(start)))
		})
	}))
}
```

The line of a stream appears when the stream ends, with its lifetime as the duration.

`WithLogSampling` throttles the warnings about action options dropped for an invalid value, URLs that `href.External` cannot use and assets that the compressor cannot read or compress. Datapages writes them on every render or request while their cause persists. By default it logs 1 of each per minute and adds the number held back as `suppressed`. It does not touch the logs of the application or the other logs of the server.

## Complete example

This `cmd/server/main.go` combines the settings above for an app in the module `example.com/shop` with `struct{}` session data. It leaves out the `.env` loading and the access logger of the scaffold, accepts previous session keys for [key rotation](security.md#manage-session-keys) and exits with status 1 when the server fails.

```go title="cmd/server/main.go"
package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/romshark/datapages"
	"github.com/romshark/datapages/modules/messaging/natscore"
	"github.com/romshark/datapages/modules/sessions"
	"github.com/romshark/datapages/modules/sessions/natskv"

	"example.com/shop/app"
	"example.com/shop/app/datapagesgen"
)

// buildID is set with -ldflags "-X main.buildID=<release>".
// Empty keeps the default, a hash of the executable.
var buildID string

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, logger); err != nil {
		logger.Error("running server", slog.Any("err", err))
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		return errors.New("NATS_URL not set")
	}
	conn, err := nats.Connect(natsURL)
	if err != nil {
		return fmt.Errorf("connecting to NATS: %w", err)
	}
	defer conn.Close()

	sessionManager, err := newSessionManager(conn)
	if err != nil {
		return err
	}

	opts := []datapages.ServerOption{
		datapages.WithLogger(logger),
		datapages.WithSessionManager[struct{}](sessionManager),
		datapages.WithShutdownTimeout(20 * time.Second),
		datapages.WithPrometheus(datapages.PrometheusConfig{
			Host: net.JoinHostPort(
				envOr("HOST_METRICS", "localhost"), envOr("PORT_METRICS", "9090"),
			),
		}),
	}
	if buildID != "" {
		opts = append(opts, datapages.WithBuildID(buildID))
	}

	s, err := datapages.NewServer[
		app.App,
		struct{},
		datapages.EnablePrometheus,
		datapagesgen.Server,
	](&app.App{}, natscore.New(conn, natscore.Config{}), opts...)
	if err != nil {
		return fmt.Errorf("creating server: %w", err)
	}

	addr := net.JoinHostPort(envOr("HOST", "localhost"), envOr("PORT", "8080"))
	cert, key := os.Getenv("PATH_TLS_CERT"), os.Getenv("PATH_TLS_KEY")
	if cert == "" && key == "" {
		return s.ListenAndServe(ctx, addr)
	}
	return s.ListenAndServeTLS(ctx, addr, cert, key)
}

// newSessionManager encrypts new session cookies with SESSION_ENCRYPTION_KEY.
// It also decrypts cookies under the keys in SESSION_PREVIOUS_ENCRYPTION_KEYS,
// separated by commas. Each key is 16 bytes in hex.
func newSessionManager(conn *nats.Conn) (*natskv.SessionManager[struct{}], error) {
	key, err := hex.DecodeString(os.Getenv("SESSION_ENCRYPTION_KEY"))
	if err != nil {
		return nil, fmt.Errorf("decoding SESSION_ENCRYPTION_KEY: %w", err)
	}
	var previous [][]byte
	for _, h := range strings.Split(os.Getenv("SESSION_PREVIOUS_ENCRYPTION_KEYS"), ",") {
		if h == "" {
			continue
		}
		k, err := hex.DecodeString(h)
		if err != nil {
			return nil, fmt.Errorf("decoding SESSION_PREVIOUS_ENCRYPTION_KEYS: %w", err)
		}
		previous = append(previous, k)
	}
	return natskv.New[struct{}](
		conn,
		sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
		natskv.Config{EncryptionKey: key, PreviousEncryptionKeys: previous},
	)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

## See also

- [Deployments](../SPECIFICATION.md#deployments): the build ID, the reload rules and stream reconnects.
- [App](../SPECIFICATION.md#app): graceful shutdown and error recovery.
- [Per-tab state](state.md): what lives in 1 process and how to route it.
- [Security](security.md): TLS, session keys, the Content-Security-Policy and the limits to put in front of the server.
- [Performance](performance.md): compression settings and their costs.
- [`ServerOption`](https://pkg.go.dev/github.com/romshark/datapages#ServerOption): the documentation of every option.
