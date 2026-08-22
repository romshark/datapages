package httpserve

import (
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

	"golang.org/x/sync/errgroup"

	"github.com/romshark/datapages"
)

const (
	DefaultHTTPReadTimeout       = 30 * time.Second
	DefaultHTTPReadHeaderTimeout = 5 * time.Second
	DefaultHTTPWriteTimeout      = 0 // SSE needs this disabled.
	DefaultHTTPIdleTimeout       = 60 * time.Second
	DefaultHTTPMaxHeaderBytes    = 1 << 20 // 1 MB

	// DefaultDatastarJSSrc is the default URL for the Datastar JavaScript bundle.
	DefaultDatastarJSSrc = "https://cdn.jsdelivr.net/gh/starfederation/datastar@1.0.2/bundles/datastar.js"
)

// Core is the HTTP server a generated server is built on.
// It holds the parts that carry nothing of the application:
// the listener, the routes, the middleware chain, the logger and the shutdown.
//
// A generated server embeds it, registers its routes on [Core.Mux] and
// calls [Core.Build].
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
	htmlHead      string
	htmlDatastar  string
	enabledTLS    bool
}

// NewCore returns a core configured by cfg, serving static files under
// assetsURLPrefix. An empty prefix serves none.
func NewCore(cfg datapages.ServerConfig, assetsURLPrefix string) *Core {
	c := &Core{
		assetsURLPrefix: assetsURLPrefix,
		shutdownCh:      make(chan struct{}),
		mux:             http.NewServeMux(),
		middleware:      cfg.Middleware,
		outermost:       cfg.OutermostMiddleware,
		metricsServer:   cfg.MetricsServer,
		assetsFS:        cfg.AssetsFS,
		datastarJSSrc:   cfg.DatastarJS,
		logger:          cfg.Logger,
		httpServer:      cfg.HTTPServer,
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
	return c
}

// Build applies the defaults and composes the handler.
// The routes must be registered on [Core.Mux] before it runs.
func (c *Core) Build() {
	// Reverse handlers such that they're invoked in the order of definition.
	slices.Reverse(c.middleware)

	if c.datastarJSSrc == "" {
		c.datastarJSSrc = DefaultDatastarJSSrc
	}
	// The two halves are kept apart as well: a page that must install a script
	// of its own before Datastar loads writes them around it.
	c.htmlHead = `<!DOCTYPE html><html><head><meta charset="UTF-8"/>`
	c.htmlDatastar = "\n\t\t" + `<script type="module" src="` +
		c.datastarJSSrc + `"></script>`
	c.htmlPrefix = c.htmlHead + c.htmlDatastar

	if c.logger == nil {
		opt := &slog.HandlerOptions{Level: slog.LevelInfo}
		if datapages.IsDevMode() {
			opt.Level = slog.LevelDebug
		}
		c.logger = slog.New(slog.NewJSONHandler(os.Stderr, opt))
	}
	if c.httpServer.ErrorLog == nil {
		c.httpServer.ErrorLog = slog.NewLogLogger(
			slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{}),
			slog.LevelInfo,
		)
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

// ShutdownCh is closed when the shutdown begins.
func (c *Core) ShutdownCh() <-chan struct{} { return c.shutdownCh }

// HTMLPrefix is the head of a page up to the Datastar script tag.
func (c *Core) HTMLPrefix() string { return c.htmlPrefix }

// HTMLHead is [Core.HTMLPrefix] up to, but excluding, the Datastar script tag.
// A page that installs a script before Datastar loads writes this first.
func (c *Core) HTMLHead() string { return c.htmlHead }

// HTMLDatastarScript is the Datastar script tag [Core.HTMLHead] stops short of.
// Writing the two in order produces [Core.HTMLPrefix].
func (c *Core) HTMLDatastarScript() string { return c.htmlDatastar }

// AssetsFS is the file system static files are served from, nil when unset.
func (c *Core) AssetsFS() http.FileSystem { return c.assetsFS }

// MetricsEnabled reports whether a metrics server is configured.
func (c *Core) MetricsEnabled() bool { return c.metricsServer != nil }

// TLSEnabled reports whether the server listens for HTTPS connections.
func (c *Core) TLSEnabled() bool { return c.enabledTLS }

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

	c.handler.ServeHTTP(w, r)
}

// ListenAndServe starts the HTTP server.
//
// The provided context controls graceful shutdown.
func (c *Core) ListenAndServe(ctx context.Context, addr string) error {
	c.httpServer.Addr = addr
	c.enabledTLS = false
	return c.listenAndServe(ctx, func() error {
		c.logger.Info("listening HTTP", slog.String("addr", addr))
		return c.httpServer.ListenAndServe()
	})
}

// ListenAndServeTLS acts identically to [Core.ListenAndServe],
// except that it expects HTTPS connections.
func (c *Core) ListenAndServeTLS(
	ctx context.Context, addr, certFile, keyFile string,
) error {
	c.httpServer.Addr = addr
	c.enabledTLS = true
	return c.listenAndServe(ctx, func() error {
		c.logger.Info("listening HTTP",
			slog.String("addr", addr),
			slog.String("tls.cert", certFile),
			slog.String("tls.key", keyFile))
		return c.httpServer.ListenAndServeTLS(certFile, keyFile)
	})
}

func (c *Core) listenAndServe(
	ctx context.Context, listenAndServe func() error,
) error {
	ctx, cancel := context.WithCancel(ctx)
	c.runCancel = cancel

	g, ctx := errgroup.WithContext(ctx)

	c.httpServer.BaseContext = func(net.Listener) context.Context { return ctx }

	// Main frontend server
	g.Go(func() error {
		if err := listenAndServe(); err != nil &&
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

		_ = c.Shutdown(shutdownCtx)
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
