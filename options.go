package datapages

import (
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"

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

	// Prometheus is what [WithPrometheus] recorded, nil when it was not given.
	// The metrics endpoint is built from it when the server is.
	Prometheus *PrometheusConfig

	// DatastarJS is the URL of the Datastar bundle the page shell loads.
	DatastarJS string

	// AssetsFS is the file system static files are served from.
	// It overrides AssetsEmbed.
	AssetsFS http.FileSystem

	// AssetsEmbed is what [WithAssets] carries. The generated server takes
	// the subdirectory and the dev-mode disk path out of it, both of which
	// the app package declared and only the generated code knows.
	AssetsEmbed *embed.FS

	// Sessions configures the session cookie and the token generator.
	Sessions SessionsConfig

	// CSRF configures the CSRF protection.
	// A nil value leaves it on with the built-in defaults.
	CSRF *CSRFConfig

	// State configures the per-page-instance state runtime.
	// A nil value leaves it unconfigured, which an application whose handlers
	// take datapages.State[T] is rejected for.
	State *StateConfig

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
	// TokenGenerator makes the token a new session is stored under.
	// The token is a bearer credential: whoever holds it is the session,
	// which is why it has to be unguessable.
	//
	// Optional. Defaults to sessions.DefaultTokenGenerator with sessions.DefaultTokenLen
	// (32 random bytes). Replace it to lengthen the token or to draw the
	// randomness from somewhere else.
	TokenGenerator sessions.TokenGenerator

	// Cookie is the cookie the token travels in.
	Cookie AuthCookieConfig

	// DisableSecureCookie writes the session cookie without the Secure flag.
	//
	// Optional. By default, secure cookies are forced, which keeps the browser from
	// sending the token over plain HTTP. It remains forced even when this process
	// serves HTTP, because TLS usually gets terminated at a proxy in front of it.
	//
	// Set this only when the whole deployment runs on plain HTTP.
	// A browser refuses a Secure cookie that arrives over plain HTTP and
	// stores no session, which would leave the visitor stuck on the sign-in page.
	DisableSecureCookie bool

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
// the path is always "/" and SameSite is always Lax.
// Secure is set unless [SessionsConfig.DisableSecureCookie] clears it.
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
// state-changing actions, that is already enabled by default for applications
// that use session based authentication.
type CSRFConfig struct {
	// Disabled turns the protection off. The other fields are then unused.
	//
	// WARNING: Every state-changing action of an authenticated visitor is then
	// accepted on the session cookie alone, which is what a cross-site page
	// needs to make the browser act for them. Set this only when something in
	// front of the server already rejects cross-origin requests.
	Disabled bool

	// Tokens writes and validates CSRF tokens.
	//
	// Optional. Defaults to [csrf.Tokens],
	// which derives the token from the session token.
	Tokens interface {
		csrf.TokenWriter
		csrf.TokenValidator
	}
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
// Without it the page shell loads
// [github.com/romshark/datapages/runtime/httpserve.DefaultDatastarJSSrc].
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
		if o.Cookie.Name == "" {
			o.Cookie.Name = DefaultSessionCookieName
		}
		if !httpread.IsCookieName(o.Cookie.Name) {
			return fmt.Errorf(
				"WithSessions: invalid cookie name: %q", o.Cookie.Name,
			)
		}
		c.Sessions = o
		return nil
	}
}

// WithCSRFProtection configures the Cross-Site-Request-Forgery protection of
// POST/PUT/PATCH/DELETE action endpoints.
//
// The protection is on for every application that declares a session type,
// with or without this option. Use it to replace the built-in tokens or to
// turn the protection off, not to switch it on.
func WithCSRFProtection(conf CSRFConfig) ServerOption {
	return func(c *ServerConfig) error {
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
//
// The HTTP metrics are labelled with the route pattern, not the request path:
// /user/{uid} carries the requests of every user. A request that matched no route,
// and a method no route registers, each carry one label of their own,
// which is what keeps a visitor from opening time series at will.
func WithPrometheus(conf PrometheusConfig) ServerOption {
	return func(c *ServerConfig) error {
		if conf.Host == "" {
			return errors.New("prometheus host address must not be empty")
		}
		if conf.Registerer == nil {
			conf.Registerer = prometheus.DefaultRegisterer
		}
		if conf.Gatherer == nil {
			conf.Gatherer = prometheus.DefaultGatherer
		}
		c.Prometheus = &conf
		c.OutermostMiddleware = prom.Middleware
		return nil
	}
}

// StateHMACKeyMinLen is the shortest key [WithStateConfig] accepts,
// the output size of the hash the key is used with.
const StateHMACKeyMinLen = 32

// DefaultMaxConcurrentInstances is the default value of
// [StateConfig.MaxConcurrentInstances].
const DefaultMaxConcurrentInstances = 10_000

// StateConfig configures the per-page-instance server-side state runtime.
// Pass it via [WithStateConfig] when at least one handler takes datapages.State[T].
type StateConfig struct {
	// HMACKey signs the Datapages-Instance identifier that rides on
	// request/response headers. Required; 32 bytes or more.
	// Rotating the key invalidates all live instances. A client whose
	// request is rejected reloads the page once and starts a fresh instance;
	// what it had not sent is lost.
	//
	// Give this purpose a key of its own, 32 random bytes or more. Sharing one key
	// across subsystems makes the security of each of them the security of all.
	HMACKey []byte

	// MaxConcurrentInstances caps how many instances exist at the same time,
	// across all state types. An instance lives exactly as long as its stream.
	// A stream open beyond the cap is answered 503 with Retry-After.
	//
	// Zero selects [DefaultMaxConcurrentInstances]. A negative value removes the cap.
	MaxConcurrentInstances int
}

// WithStateConfig enables the per-page-instance server-side state runtime.
// Required when at least one handler takes datapages.State[T].
//
// On multi-server deployments the load balancer MUST route requests for a
// given client consistently to the same backend (sticky sessions),
// since state lives in process memory.
func WithStateConfig(conf StateConfig) ServerOption {
	return func(c *ServerConfig) error {
		// The bound is the output size of SHA-256, which is what RFC 2104
		// asks of a key. It catches a short literal and nothing else:
		// length is not entropy, and 32 bytes derived from one weak
		// passphrase pass this check.
		if len(conf.HMACKey) < StateHMACKeyMinLen {
			return fmt.Errorf(
				"WithStateConfig: HMACKey must be at least %d bytes, got %d",
				StateHMACKeyMinLen, len(conf.HMACKey))
		}
		if conf.MaxConcurrentInstances == 0 {
			conf.MaxConcurrentInstances = DefaultMaxConcurrentInstances
		}
		c.State = &conf
		return nil
	}
}
