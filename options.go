package datapages

import (
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/romshark/datapages/modules/csrf"
	"github.com/romshark/datapages/modules/sessions"
	"github.com/romshark/datapages/runtime/httpread"
	"github.com/romshark/datapages/runtime/prom"
)

// DefaultSessionCookieName is the name of the session cookie.
const DefaultSessionCookieName = "sessiontoken"

// ServerConfig is what a generated server is configured with.
// [ServerOption] values fill it, the generated NewServer reads it.
type ServerConfig struct {
	// Logger receives what the server logs. Defaults to a JSON handler on stderr.
	Logger *slog.Logger

	// Middleware wraps the router, in the order it was added.
	Middleware []func(http.Handler) http.Handler

	// OutermostMiddleware wraps every other middleware.
	OutermostMiddleware func(http.Handler) http.Handler

	// HTTPServer serves the application. Its Addr and Handler are always overwritten.
	HTTPServer *http.Server

	// MetricsServer serves the metrics endpoint.
	MetricsServer *http.Server

	// DatastarJS is the URL of the Datastar bundle the page shell loads.
	DatastarJS string

	// AssetsFS is the file system static files are served from.
	// It overrides AssetsEmbed.
	AssetsFS http.FileSystem

	// AssetsEmbed is what [WithAssets] carries. The generated server takes
	// the subdirectory and the dev-mode disk path out of it, both of which
	// the app package declared and only the generated code knows.
	AssetsEmbed *embed.FS

	// Auth configures the session cookie and the token generator.
	Auth SessionsConfig

	// CSRF configures the CSRF protection. A nil value disables it.
	CSRF *CSRFConfig

	// sessionManager is what [WithSessionManager] carries.
	// ServerConfig is not generic, hence the manager travels as any and
	// [NewServer] asserts it once to the type the application declares.
	sessionManager any
}

// ServerOption configures a generated server.
type ServerOption func(*ServerConfig) error

// SessionsConfig configures how a session is carried between requests.
//
// A session lives in the session manager under a token. The token travels in a
// cookie, which is all the browser ever holds: the session data itself never
// leaves the server. This decides how that token is made and how the cookie
// carrying it is written.
//
// The zero value is what most applications want.
type SessionsConfig struct {
	// SessionTokenGenerator makes the token a new session is stored under.
	// The token is a bearer credential: whoever holds it is the session,
	// which is why it has to be unguessable.
	//
	// Optional. Defaults to sessions.DefaultTokenGenerator with sessions.DefaultTokenLen
	// (32 random bytes). Replace it to lengthen the token or to draw the
	// randomness from somewhere else.
	SessionTokenGenerator sessions.TokenGenerator

	// TokenCookie is the cookie the token travels in.
	TokenCookie AuthCookieConfig

	// DisableHTTPOnly exposes the session cookie to client-side JavaScript.
	//
	// Optional. The cookie is HttpOnly by default, which keeps injected
	// script from reading the token and stealing the session. Disable it only
	// when your own JavaScript has to read the token itself, knowing that an
	// XSS hole then hands out sessions.
	DisableHTTPOnly bool
}

// AuthCookieConfig configures the cookie the session token is kept in.
//
// Only what an application has reason to change is here. The rest is fixed:
// the path is always "/", SameSite is always Lax, and Secure follows whether
// the server is serving TLS.
type AuthCookieConfig struct {
	// Name is what the browser sends the token back under.
	//
	// Optional. Defaults to DefaultSessionCookieName. Change it when
	// another application on the same domain already uses that name, since
	// two applications sharing a cookie name overwrite each other.
	Name string

	// Domain is which hosts the browser sends the cookie to.
	//
	// Optional. By default it's empty, which means the exact host that set it.
	// Set a parent domain ("example.com") to share one session
	// across subdomains such as app.example.com and admin.example.com,
	// which also hands the token to every other host under it.
	Domain string
}

// CSRFConfig configures the CSRF (Cross-Site-Request-Forgery) protection of
// state-changing actions.
//
// Without it a page on another origin could make the browser send an action
// request carrying the session cookie. The token proves the request came from
// a page this server rendered.
type CSRFConfig struct {
	// Tokens issues the token a page carries and verifies the one a request
	// comes back with. Both halves are one value: a generator and a validator
	// that disagreed would fail only once a request arrived.
	//
	// It is required, a nil value is an error. See modules/csrf/hmac for an
	// implementation that needs no storage, deriving the token from a secret
	// plus the session it belongs to.
	Tokens interface {
		csrf.TokenGenerator
		csrf.TokenValidator
	}

	// DevBypassToken, if non-empty, is accepted as a valid
	// CSRF token for any session. Use this only in development
	// to allow tools like k6 to exercise POST endpoints.
	//
	// [WithCSRFProtection] refuses it outside dev mode.
	DevBypassToken string
}

// WithLogger sets a custom error logger.
// Consider setting level DEBUG when [IsDevMode] returns true.
func WithLogger(l *slog.Logger) ServerOption {
	return func(c *ServerConfig) error {
		c.Logger = l
		return nil
	}
}

// WithMiddleware adds custom middleware.
//
// They wrap the router in the order they are given, across calls too:
//
//	datapages.WithMiddleware(a, b)
//	datapages.WithMiddleware(c)
//
//	a -> b -> c -> router
func WithMiddleware(middleware ...func(http.Handler) http.Handler) ServerOption {
	return func(c *ServerConfig) error {
		for i, m := range middleware {
			if m == nil {
				return fmt.Errorf("WithMiddleware: nil middleware at index %d", i)
			}
		}
		c.Middleware = append(c.Middleware, middleware...)
		return nil
	}
}

// WithHTTPServer sets a custom HTTP server.
// The Addr and Handler fields are always overwritten.
func WithHTTPServer(server *http.Server) ServerOption {
	return func(c *ServerConfig) error {
		c.HTTPServer = server
		return nil
	}
}

// WithDatastarJS sets a custom URL for the Datastar JavaScript bundle.
func WithDatastarJS(src string) ServerOption {
	return func(c *ServerConfig) error {
		c.DatastarJS = src
		return nil
	}
}

// WithAssetsFS serves static files from fsys, overriding [WithAssets].
// It takes the file system as it is, applying nothing the app package
// declared: no subdirectory is extracted and dev mode changes nothing.
func WithAssetsFS(fsys http.FileSystem) ServerOption {
	return func(c *ServerConfig) error {
		c.AssetsFS = fsys
		return nil
	}
}

// WithSessions sets session-based authentication configuration.
func WithSessions(o SessionsConfig) ServerOption {
	return func(c *ServerConfig) error {
		if o.TokenCookie.Name == "" {
			o.TokenCookie.Name = DefaultSessionCookieName
		}
		if !httpread.IsCookieName(o.TokenCookie.Name) {
			return fmt.Errorf(
				"WithSessions: invalid cookie name: %q", o.TokenCookie.Name,
			)
		}
		c.Auth = o
		return nil
	}
}

// WithCSRFProtection enables Cross-Site-Request-Forgery protection on
// POST/PUT/PATCH/DELETE action endpoints.
func WithCSRFProtection(conf CSRFConfig) ServerOption {
	return func(c *ServerConfig) error {
		if conf.Tokens == nil {
			return errors.New("nil CSRF tokens")
		}
		if conf.DevBypassToken != "" && !IsDevMode() {
			return errors.New("CSRF dev bypass token must not be set in non-dev mode")
		}
		c.CSRF = &conf
		return nil
	}
}

// WithSessionManager enables sessions and resolves them with m.
//
// The built-in managers are
// [github.com/romshark/datapages/modules/sessions/natskv] and
// [github.com/romshark/datapages/modules/sessions/inmem],
// any [sessions.Manager] works.
//
// [NewServer] returns an error when this option is missing and its SessionData
// type argument is not [DisableSessions].
func WithSessionManager[SessionData any](
	m sessions.Manager[SessionData],
) ServerOption {
	return func(c *ServerConfig) error {
		if m == nil {
			return errors.New("WithSessionManager: nil session manager")
		}
		if c.sessionManager != nil {
			return errors.New("WithSessionManager: session manager already set")
		}
		c.sessionManager = m
		return nil
	}
}

// WithAssets serves the static files embedded in fsys.
//
// The URL path they are served at, the subdirectory they are read from and
// the directory dev mode reads them from instead are all declared by the app
// package, on the embed.FS variable:
//
//	// AssetsFS is /static/
//	//go:embed static/*
//	var AssetsFS embed.FS
//
// The generated server applies them, and rejects this option when the app
// package declares none.
func WithAssets(fsys embed.FS) ServerOption {
	return func(c *ServerConfig) error {
		c.AssetsEmbed = &fsys
		return nil
	}
}

// PrometheusConfig configures the Prometheus metrics endpoint.
type PrometheusConfig struct {
	// Host is the address the metrics server listens on,
	// for example "127.0.0.1:9091" or ":9091".
	Host string

	// Registerer is optional, default: prometheus.DefaultRegisterer
	Registerer prometheus.Registerer

	// Gatherer is optional, default: prometheus.DefaultGatherer
	Gatherer prometheus.Gatherer

	// Handler optionally overrides the /metrics handler.
	Handler http.Handler

	// Collectors are user-defined metrics to register.
	Collectors []prometheus.Collector
}

// WithPrometheus starts a dedicated HTTP server exposing /metrics.
//
// Required by a server built with [EnablePrometheus],
// rejected by one built with [DisablePrometheus].
func WithPrometheus(conf PrometheusConfig) ServerOption {
	return func(c *ServerConfig) error {
		if conf.Host == "" {
			return errors.New("prometheus host address must not be empty")
		}

		// Defaults
		if conf.Registerer == nil {
			conf.Registerer = prometheus.DefaultRegisterer
		}
		if conf.Gatherer == nil {
			conf.Gatherer = prometheus.DefaultGatherer
		}

		// Register built-in metrics on the configured registerer exactly once.
		prom.Register(conf.Registerer)

		// Register user-defined collectors.
		for _, collector := range conf.Collectors {
			conf.Registerer.MustRegister(collector)
		}

		var h http.Handler
		if conf.Handler != nil {
			h = conf.Handler
		} else {
			h = promhttp.HandlerFor(conf.Gatherer, promhttp.HandlerOpts{})
		}

		mux := http.NewServeMux()
		mux.Handle("/metrics", h)

		c.MetricsServer = &http.Server{
			Addr:    conf.Host,
			Handler: mux,
		}
		c.OutermostMiddleware = prom.Middleware
		return nil
	}
}
