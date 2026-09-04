package httpserve

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/sync/errgroup"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/runtime/prom"
)

const (
	DefaultHTTPReadTimeout       = 30 * time.Second
	DefaultHTTPReadHeaderTimeout = 5 * time.Second
	DefaultHTTPWriteTimeout      = 0 // SSE needs this disabled.
	DefaultHTTPIdleTimeout       = 60 * time.Second
	DefaultHTTPMaxHeaderBytes    = 1 << 20 // 1 MB

	// DefaultBodySizeLimit is how much of an action's request body is read
	// before the request is refused. Signals travel in that body.
	DefaultBodySizeLimit int64 = 1 << 20 // 1 MiB

	// DefaultDatastarJSSrc is the default URL for the Datastar JavaScript bundle.
	DefaultDatastarJSSrc = "https://cdn.jsdelivr.net/gh/starfederation/datastar@1.0.3/bundles/datastar.js"
)

// Core is the HTTP server a generated server is built on.
// It holds the parts that carry nothing of the application:
// the listener, the routes, the middleware chain, the logger and the shutdown.
//
// A generated server embeds it, registers its routes on
// [Core.Mux] and calls [Core.Build].
type Core struct {
	// assetsURLPrefix is the path static files are served under.
	// Empty when the application serves none.
	assetsURLPrefix string

	shutdownCh   chan struct{} // Closed when shutting down.
	shutdownOnce sync.Once
	runCancel    context.CancelFunc

	httpServer    *http.Server
	metricsServer *http.Server
	mux           *http.ServeMux
	handler       http.Handler
	logger        *slog.Logger
	middleware    []func(http.Handler) http.Handler
	outermost     func(http.Handler) http.Handler
	assetsFS      http.FileSystem
	datastarJSSrc string
	htmlPrefix    string
	bodySizeLimit int64

	// lockListen guards the fields [Core.listenAndServe] sets once it binds.
	lockListen sync.Mutex
	addr       string
	enabledTLS bool
}

// NewCore returns a core configured by cfg, serving static files under assetsURLPrefix.
// An empty prefix serves none.
func NewCore(cfg datapages.ServerConfig, assetsURLPrefix string) (*Core, error) {
	c := &Core{
		assetsURLPrefix: assetsURLPrefix,
		shutdownCh:      make(chan struct{}),
		mux:             http.NewServeMux(),
		middleware:      cfg.Middleware,
		outermost:       cfg.OutermostMiddleware,
		assetsFS:        cfg.AssetsFS,
		datastarJSSrc:   cfg.DatastarJS,
		logger:          cfg.Logger,
		httpServer:      cfg.HTTPServer,
		bodySizeLimit:   cfg.BodySizeLimit,
	}
	if c.bodySizeLimit <= 0 {
		c.bodySizeLimit = DefaultBodySizeLimit
	}
	if c.logger == nil {
		// Not in Build: the generated Init logs in between.
		opt := &slog.HandlerOptions{Level: slog.LevelInfo}
		if datapages.IsDevMode() {
			opt.Level = slog.LevelDebug
		}
		c.logger = slog.New(slog.NewJSONHandler(os.Stderr, opt))
	}
	if c.httpServer == nil {
		c.httpServer = &http.Server{
			// Time to read request headers + body
			ReadTimeout: DefaultHTTPReadTimeout,
			// Time to read just headers (helps prevent Slowloris attacks)
			ReadHeaderTimeout: DefaultHTTPReadHeaderTimeout,
			// Time to write response
			WriteTimeout: DefaultHTTPWriteTimeout,
			// Time to wait for next request when using keep-alive
			IdleTimeout:    DefaultHTTPIdleTimeout,
			MaxHeaderBytes: DefaultHTTPMaxHeaderBytes,
		}
	}
	if p := cfg.Prometheus; p != nil {
		// The registering happens here rather than in [datapages.WithPrometheus]
		// so a rejected configuration leaves the registerer as it found it.
		if err := prom.Register(p.Registerer, p.Collectors...); err != nil {
			return nil, fmt.Errorf("registering prometheus metrics: %w", err)
		}
		h := p.Handler
		if h == nil {
			h = promhttp.HandlerFor(p.Gatherer, promhttp.HandlerOpts{})
		}
		mux := http.NewServeMux()
		mux.Handle("/metrics", h)
		c.metricsServer = &http.Server{
			Addr:              p.Host,
			Handler:           mux,
			ReadHeaderTimeout: DefaultHTTPReadHeaderTimeout,
		}
	}
	return c, nil
}

// Build applies the defaults and composes the handler.
// The routes must be registered on [Core.Mux] before it runs.
func (c *Core) Build() {
	// Reverse handlers such that they're invoked in the order of definition.
	slices.Reverse(c.middleware)

	if c.datastarJSSrc == "" {
		c.datastarJSSrc = DefaultDatastarJSSrc
	}
	c.htmlPrefix = `<!DOCTYPE html><html><head><meta charset="UTF-8"/>
		<script type="module" src="` + c.datastarJSSrc + `"></script>`

	if c.httpServer.ErrorLog == nil {
		c.httpServer.ErrorLog = slog.NewLogLogger(
			slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{}),
			slog.LevelInfo,
		)
	}

	if c.assetsFS != nil && c.assetsURLPrefix != "" {
		h := http.StripPrefix(c.assetsURLPrefix, http.FileServer(c.assetsFS))
		if datapages.IsDevMode() {
			h = DevNoCache(h)
		}
		c.mux.Handle("GET "+c.assetsURLPrefix, h)
	}

	c.handler = http.Handler(c.mux)
	for _, h := range c.middleware {
		c.handler = h(c.handler)
	}
	if c.outermost != nil {
		c.handler = c.outermost(c.handler)
	}
	c.httpServer.Handler = c
}

// Mux is the router the routes are registered on.
func (c *Core) Mux() *http.ServeMux { return c.mux }

// Logger is the logger the server writes to.
func (c *Core) Logger() *slog.Logger { return c.logger }

// LogErr logs err under msg.
func (c *Core) LogErr(msg string, err error) {
	c.logger.Error(msg, slog.Any("err", err))
}

// HTTPErrBad answers r with 400 and logs msg as the cause.
func (c *Core) HTTPErrBad(w http.ResponseWriter, msg string, err error) {
	c.logger.Debug("bad request", slog.String("cause", msg), slog.Any("err", err))
	http.Error(w, msg, http.StatusBadRequest)
}

// CheckDatastarRequest reports whether r was issued by the Datastar client.
// It answers r with 406 when it was not, in which case the handler must
// write nothing more.
func (c *Core) CheckDatastarRequest(w http.ResponseWriter, r *http.Request) (ok bool) {
	if !IsDatastarRequest(r) {
		c.logger.Debug("not a datastar request",
			slog.Any("method", r.Method),
			slog.String("path", r.URL.Path))
		http.Error(w,
			http.StatusText(http.StatusNotAcceptable),
			http.StatusNotAcceptable)
		return false
	}
	return true
}

// ShutdownCh is closed when the shutdown begins.
func (c *Core) ShutdownCh() <-chan struct{} { return c.shutdownCh }

// HTMLPrefix is the head of a page up to the Datastar script tag.
func (c *Core) HTMLPrefix() string { return c.htmlPrefix }

// AssetsFS is the file system static files are served from, nil when unset.
func (c *Core) AssetsFS() http.FileSystem { return c.assetsFS }

// MetricsEnabled reports whether a metrics server is configured.
func (c *Core) MetricsEnabled() bool { return c.metricsServer != nil }

// BodySizeLimit is how much of an action's request body a handler reads.
func (c *Core) BodySizeLimit() int64 { return c.bodySizeLimit }

// TLSEnabled reports whether the server listens for HTTPS connections.
func (c *Core) TLSEnabled() bool {
	c.lockListen.Lock()
	defer c.lockListen.Unlock()
	return c.enabledTLS
}

// Addr is the address the server bound. A caller that passed port 0
// reads the chosen port here. Empty until it listens.
func (c *Core) Addr() string {
	c.lockListen.Lock()
	defer c.lockListen.Unlock()
	return c.addr
}

// tracked records whether any of the response body went out. A body cannot be
// taken back: an error page appended to a half-written page is two documents.
//
// The status alone does not count. A handler that wrote one and nothing else
// still has a body to write, which is what the error path writes into.
type tracked struct {
	http.ResponseWriter
	wroteBody bool
}

func (t *tracked) Write(b []byte) (int, error) {
	t.wroteBody = true
	return t.ResponseWriter.Write(b)
}

func (t *tracked) Flush() {
	if f, ok := t.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (t *tracked) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := t.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("http.Hijacker not supported")
}

func (t *tracked) Push(target string, opts *http.PushOptions) error {
	if p, ok := t.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

// ResponseBodyWritten reports whether any of the response body was written.
// A handler that failed past this point can be logged and nothing more.
func ResponseBodyWritten(w http.ResponseWriter) bool {
	t, ok := w.(*tracked)
	return ok && t.wroteBody
}

func (c *Core) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Normalize trailing slashes: ensure all paths end with /
	// except for static file paths.
	if p := r.URL.Path; p != "/" && !strings.HasSuffix(p, "/") {
		if c.assetsURLPrefix == "" || !strings.HasPrefix(p, c.assetsURLPrefix) {
			r.URL.Path = p + "/"
			if r.URL.RawPath != "" {
				r.URL.RawPath = r.URL.RawPath + "/"
			}
		}
	}

	c.handler.ServeHTTP(&tracked{ResponseWriter: w}, r)
}

// WildcardPathValue reads the value of a {name...} route wildcard.
//
// [Core.ServeHTTP] appends a slash to a path that carries none.
// A wildcard runs to the end of the path, so that slash lands inside its
// value instead of after it, and every request reaches the handler with one,
// however the client wrote the URL. Trimming it is what makes the value
// the handler reads the value the caller built the URL with.
//
// A value that ends in a slash of its own cannot be told apart from one the
// normalization added and loses it too.
func WildcardPathValue(r *http.Request, name string) string {
	return strings.TrimSuffix(r.PathValue(name), "/")
}

// ListenAndServe starts the HTTP server.
//
// The provided context controls graceful shutdown.
func (c *Core) ListenAndServe(ctx context.Context, addr string) error {
	return c.listenAndServe(ctx, addr, false, func(ln net.Listener) error {
		c.logger.Info("listening HTTP", slog.String("addr", c.Addr()))
		return c.httpServer.Serve(ln)
	})
}

// ListenAndServeTLS acts identically to [Core.ListenAndServe],
// except that it expects HTTPS connections.
func (c *Core) ListenAndServeTLS(
	ctx context.Context, addr, certFile, keyFile string,
) error {
	return c.listenAndServe(ctx, addr, true, func(ln net.Listener) error {
		c.logger.Info("listening HTTP",
			slog.String("addr", c.Addr()),
			slog.String("tls.cert", certFile),
			slog.String("tls.key", keyFile))
		return c.httpServer.ServeTLS(ln, certFile, keyFile)
	})
}

func (c *Core) listenAndServe(
	ctx context.Context, addr string, tls bool, serve func(net.Listener) error,
) error {
	if datapages.IsDevMode() {
		// Assets come from the source tree here,
		// which a deployment that inherited the variable does not have.
		c.logger.Warn("dev mode is on",
			slog.String("env", datapages.EnvVarDevMode+"/TEMPL_DEV_MODE"))
	}
	c.httpServer.Addr = addr
	// Bind here rather than in [http.Server.ListenAndServe] so that the port
	// is known before anything serves. A caller that passed port 0 reads the
	// one the kernel chose from [Core.Addr].
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", addr, err)
	}
	boundAddr := ln.Addr().String()
	c.lockListen.Lock()
	c.addr, c.enabledTLS = boundAddr, tls
	c.lockListen.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	c.runCancel = cancel

	g, ctx := errgroup.WithContext(ctx)

	c.httpServer.BaseContext = func(net.Listener) context.Context { return ctx }

	// Main frontend server
	g.Go(func() error {
		if err := serve(ln); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	// Metrics server
	if c.metricsServer != nil {
		c.metricsServer.BaseContext = func(net.Listener) context.Context {
			return ctx
		}
		g.Go(func() error {
			err := c.metricsServer.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("metrics server failed: %w", err)
			}
			return nil
		})
	}

	// Coordinated shutdown
	g.Go(func() error {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(
			context.Background(), 10*time.Second,
		)
		defer cancel()

		// An SSE stream open when the grace period ends is routine here.
		// Returning the error would mark every such shutdown a failed run.
		if err := c.Shutdown(shutdownCtx); err != nil {
			c.LogErr("shutting down", err)
		}
		return nil
	})

	return g.Wait()
}

// Shutdown gracefully shuts down all server components.
func (c *Core) Shutdown(ctx context.Context) error {
	c.shutdownOnce.Do(func() {
		c.logger.Info("server shutdown initiated")
		if c.runCancel != nil {
			c.runCancel()
		}
		close(c.shutdownCh)
	})
	var errs []error
	if c.metricsServer != nil {
		if err := c.metricsServer.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if err := c.httpServer.Shutdown(ctx); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
