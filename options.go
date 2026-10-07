package datapages

import (
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/romshark/datapages/internal/logsample"
	"github.com/romshark/datapages/modules/csrf"
	"github.com/romshark/datapages/modules/sessions"
	"github.com/romshark/datapages/runtime/httpread"
	"github.com/romshark/datapages/runtime/prom"
)

// DefaultSessionCookieName is what [AuthCookieConfig.Name] falls back to.
const DefaultSessionCookieName = "sessiontoken"

// DefaultLogSamplingLimit is what [LogSamplingConfig.Limit] falls back to.
const DefaultLogSamplingLimit = logsample.DefaultLimit

// DefaultLogSamplingInterval is what [LogSamplingConfig.Interval] falls back to.
const DefaultLogSamplingInterval = logsample.DefaultInterval

// ServerConfig is what a generated server is configured with.
// [ServerOption] values fill it and [NewServer] hands it to the generated
// [ServerInitializer.Init], which reads it.
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

	// CSPNonce reports the Content-Security-Policy nonce of a request, nil when
	// [WithCSPNonce] was not given. Calls with the same request must return the
	// same value.
	CSPNonce func(r *http.Request) string

	// AssetsFS is the file system static files are served from.
	// It overrides AssetsEmbed.
	AssetsFS http.FileSystem

	// AssetsEmbed is what [WithAssets] carries. The generated server takes
	// the subdirectory and the dev-mode disk path out of it, both of which
	// the app package declared and only the generated code knows.
	AssetsEmbed *embed.FS

	// AssetsBrowsable is the browsable argument of [WithAssets] and [WithAssetsFS].
	AssetsBrowsable bool

	// AssetsCache configures production asset cache headers.
	// A nil value adds no Cache-Control or ETag header.
	AssetsCache *AssetsCacheConfig

	// Sessions configures the session cookie and the token generator.
	Sessions SessionsConfig

	// CSRF configures the CSRF protection.
	// A nil value leaves it on with the built-in defaults.
	CSRF *CSRFConfig

	// LogSampling is what [WithLogSampling] carries.
	// A nil value samples with the built-in default.
	LogSampling *LogSamplingConfig

	// BodySizeLimit is what [WithBodySizeLimit] carries.
	// Zero selects httpserve.DefaultBodySizeLimit.
	BodySizeLimit int64

	// ShutdownTimeout is the grace period used by
	// [github.com/romshark/datapages/runtime/httpserve.Core.ListenAndServe].
	// Zero selects httpserve.DefaultShutdownTimeout.
	ShutdownTimeout time.Duration

	// State sets the per-tab state limit. Nil selects the default.
	State *StateConfig

	// BuildID identifies the application build, set by [WithBuildID].
	// Empty selects the hash of the executable.
	BuildID string

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
// Max-Age and Expires come from [NewSession.ExpiresAt].
type AuthCookieConfig struct {
	// Name is what the browser sends the token back under.
	//
	// Optional. Defaults to [DefaultSessionCookieName]. Change it when
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
//
// src must be an http, https or relative URL according to RFC 3986.
func WithDatastarJS(src string) ServerOption {
	return func(c *ServerConfig) error {
		if err := validateDatastarJS(src); err != nil {
			return fmt.Errorf("WithDatastarJS: %w", err)
		}
		c.DatastarJS = src
		return nil
	}
}

// validateDatastarJS refuses a src that is not a URL according to RFC 3986,
// which is a configuration mistake to report at startup.
// The checks do not protect the page: the page shell writes src HTML-escaped.
// net/url takes characters RFC 3986 has no place for, a quote and a space among them,
// hence the check of its own.
func validateDatastarJS(src string) error {
	if src == "" {
		return errors.New("empty URL")
	}
	for _, r := range src {
		if !isURLChar(r) {
			return fmt.Errorf("URL contains %q, which RFC 3986 does not allow", r)
		}
	}
	u, err := url.Parse(src)
	if err != nil {
		return fmt.Errorf("parsing URL: %w", err)
	}
	// A URL with no scheme is relative, which is the app's own asset route.
	switch u.Scheme {
	case "", "http", "https":
		return nil
	}
	return fmt.Errorf("URL scheme %q is neither http nor https", u.Scheme)
}

// isURLChar reports whether RFC 3986 allows r in a URL: the unreserved and the
// reserved characters, and the % of a percent-encoding, which [url.Parse] checks.
func isURLChar(r rune) bool {
	switch {
	case 'a' <= r && r <= 'z', 'A' <= r && r <= 'Z', '0' <= r && r <= '9':
		return true
	}
	return strings.ContainsRune("-._~:/?#[]@!$&'()*+,;=%", r)
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
//
// browsable true lists a directory that has no index.html, false returns 404.
// Pass browsable=false in production to avoid exposing every embedded file.
func WithAssets(fsys embed.FS, browsable bool) ServerOption {
	return func(c *ServerConfig) error {
		c.AssetsEmbed, c.AssetsBrowsable = &fsys, browsable
		return nil
	}
}

// WithAssetsFS serves static files from fsys, overriding [WithAssets].
// It takes the file system as it is, applying nothing the app package declared:
// no subdirectory is extracted and dev mode changes nothing.
//
// browsable true lists a directory that has no index.html, false returns 404.
// Pass browsable=false in production to avoid exposing every embedded file.
func WithAssetsFS(fsys http.FileSystem, browsable bool) ServerOption {
	return func(c *ServerConfig) error {
		c.AssetsFS, c.AssetsBrowsable = fsys, browsable
		return nil
	}
}

// AssetsCacheConfig configures cache headers for static assets.
// Its zero value sends "Cache-Control: public, max-age=0" and an ETag.
// A conditional request for an unchanged asset then receives 304 with no body.
type AssetsCacheConfig struct {
	// Disabled prevents [WithAssetsCache] from adding headers.
	// It supports configuration files and flags that cannot omit the option.
	Disabled bool

	// MaxAge is how long a browser may reuse an asset without asking again.
	// The max-age directive has one-second precision.
	//
	// Zero writes max-age=0, which requires revalidation.
	// Use a positive value only when the asset URL changes with its content.
	// Otherwise the browser may use stale content until MaxAge expires.
	MaxAge time.Duration

	// Immutable adds the immutable directive, which prevents reloads from
	// revalidating a fresh response. It requires a positive MaxAge.
	Immutable bool

	// CacheControl sets the header value directly.
	// It cannot be combined with MaxAge or Immutable.
	CacheControl string

	// DisableETag prevents [WithAssetsCache] from adding an ETag.
	// Use it when a CDN or compressing middleware writes its own ETag,
	// or when files can change during the process lifetime.
	DisableETag bool
}

// WithAssetsCache adds Cache-Control and an optional ETag to static files
// served by [WithAssets] and [WithAssetsFS].
//
// Optional. Without it Datapages adds neither header.
//
// The server computes each ETag on the first request and caches it for the
// process lifetime. This matches the embedded file system used by [WithAssets].
// Set [AssetsCacheConfig.DisableETag] for a [WithAssetsFS] file system whose
// files can change while the server runs.
//
// Dev mode ignores this option and sends Cache-Control: no-store.
func WithAssetsCache(conf AssetsCacheConfig) ServerOption {
	return func(c *ServerConfig) error {
		switch {
		case conf.MaxAge < 0:
			return fmt.Errorf("WithAssetsCache: max age (%s) must not be negative",
				conf.MaxAge)
		case conf.MaxAge > 0 && conf.MaxAge < time.Second:
			return fmt.Errorf("WithAssetsCache: max age (%s) must be at least 1s",
				conf.MaxAge)
		case conf.CacheControl != "" && (conf.MaxAge != 0 || conf.Immutable):
			return fmt.Errorf(
				"WithAssetsCache: cache control (%q) cannot be combined with "+
					"max age (%s) or immutable (%t)",
				conf.CacheControl, conf.MaxAge, conf.Immutable,
			)
		case conf.Immutable && conf.MaxAge <= 0:
			return fmt.Errorf("WithAssetsCache: immutable (%t) requires "+
				"a max age (%s) above zero",
				conf.Immutable, conf.MaxAge)
		}
		// Reject control characters because CR or LF could inject response headers.
		if i := strings.IndexFunc(conf.CacheControl, func(r rune) bool {
			return r < 0x20 || r == 0x7f
		}); i != -1 {
			return fmt.Errorf(
				"WithAssetsCache: cache control (%q) contains "+
					"a control character at byte %d",
				conf.CacheControl, i,
			)
		}
		c.AssetsCache = &conf
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

// LogSamplingConfig is what [WithLogSampling] applies.
type LogSamplingConfig struct {
	// Disabled reports every occurrence.
	//
	// WARNING: One bad value then writes a line per element per request.
	// Disable only when a shorter Interval won't do.
	Disabled bool

	// Interval is how long a warning is throttled for after it was reported.
	// The next one carries what the throttling swallowed as "suppressed".
	//
	// Zero selects [DefaultLogSamplingInterval].
	Interval time.Duration

	// Limit is how many distinct warnings are tracked at once.
	// Zero selects [DefaultLogSamplingLimit]. A full set first drops what has
	// gone quiet, so a mistake found later is still reported.
	Limit int
}

// WithLogSampling configures how the framework's own warnings are reported:
//
//   - an action option [github.com/romshark/datapages/runtime/actionexpr]
//     drops for an invalid value
//   - a URL the generated href.External cannot use
//
// Since such warnings are written per page render, one bad value may spike log volume.
// This applies to the warnings named above and to nothing else,
// neither to what the application logs nor to the rest of the built-in ones.
//
// Optional. By default one warning of each kind is reported per minute,
// carrying how many were held back since the last one.
func WithLogSampling(conf LogSamplingConfig) ServerOption {
	return func(c *ServerConfig) error {
		if conf.Limit < 0 {
			return fmt.Errorf("log sampling limit must not be negative: %d",
				conf.Limit)
		}
		if conf.Interval < 0 {
			return fmt.Errorf("log sampling interval must not be negative: %s",
				conf.Interval)
		}
		c.LogSampling = &conf
		return nil
	}
}

// WithBodySizeLimit caps the request body of an action that declares [Signals].
// A handler that reads r.Body itself must cap it with [http.MaxBytesReader].
//
// Optional.
// Defaults to [github.com/romshark/datapages/runtime/httpserve.DefaultBodySizeLimit].
//
// A client sending more receives 400 with the body "reading signals" and the
// handler does not run. The cause is logged at debug level.
// The client is not told what the limit is.
func WithBodySizeLimit(bytes int64) ServerOption {
	return func(c *ServerConfig) error {
		if bytes <= 0 {
			return errors.New("WithBodySizeLimit: limit must be greater than zero")
		}
		c.BodySizeLimit = bytes
		return nil
	}
}

// WithShutdownTimeout limits how long
// [github.com/romshark/datapages/runtime/httpserve.Core.ListenAndServe] waits
// after context cancellation for in-flight requests, open SSE streams,
// and StreamClose hooks.
//
// Optional.
// Defaults to [github.com/romshark/datapages/runtime/httpserve.DefaultShutdownTimeout].
//
// If the timeout expires, ListenAndServe logs the shutdown error and returns.
// A direct call to
// [github.com/romshark/datapages/runtime/httpserve.Core.Shutdown] uses the
// deadline of its context instead.
func WithShutdownTimeout(d time.Duration) ServerOption {
	return func(c *ServerConfig) error {
		if d <= 0 {
			return errors.New(
				"WithShutdownTimeout: timeout must be greater than zero",
			)
		}
		c.ShutdownTimeout = d
		return nil
	}
}

// maxBuildIDLen limits IDs accepted by [WithBuildID].
const maxBuildIDLen = 128

// WithBuildID sets the ID that distinguishes one application build from another.
//
// Optional. The default is a hash of the executable. Replicas that run the same
// binary share an ID. Set a shared release ID when replicas use different
// binaries but serve the same pages.
//
// Each page sends the ID in [HeaderBuild] with Datastar requests.
// A server with a different ID returns 205 Reset Content without running the handler.
// The page then reloads.
//
// id must contain 1 to 128 printable ASCII characters without spaces.
func WithBuildID(id string) ServerOption {
	return func(c *ServerConfig) error {
		if err := checkBuildID(id); err != nil {
			return fmt.Errorf("WithBuildID: %w", err)
		}
		c.BuildID = id
		return nil
	}
}

// checkBuildID rejects an id that would not reach the server unchanged in
// [HeaderBuild]. Fetch rejects control chars and chars above U+00FF,
// strips surrounding whitespace and sends U+0080 to U+00FF as Latin-1 bytes.
// A changed ID makes every Datastar request fail or reload its page.
// An empty ID would select the executable hash,
// and [maxBuildIDLen] keeps the header short.
func checkBuildID(id string) error {
	if id == "" {
		return errors.New("id is empty")
	}
	if len(id) > maxBuildIDLen {
		return fmt.Errorf("id length (%d) exceeds %d bytes", len(id), maxBuildIDLen)
	}
	for i, r := range id {
		switch {
		case r == ' ':
			return fmt.Errorf("id contains a space at byte %d", i)
		case r < ' ' || r == 0x7f:
			return fmt.Errorf("id contains control character %q at byte %d", r, i)
		case r > '~':
			return fmt.Errorf("id contains non-ASCII character %q at byte %d", r, i)
		}
	}
	return nil
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

// DefaultMaxConcurrentInstances is the default value of
// [StateConfig.MaxConcurrentInstances].
const DefaultMaxConcurrentInstances = 10_000

// StateConfig sets the live-instance limit for per-tab [State].
type StateConfig struct {
	// MaxConcurrentInstances limits the live state instances held by this server
	// across all state types. A stream allocates one instance before it opens and
	// releases it when it closes. Each server keeps its own count.
	//
	// A stream over the limit receives 503 with Retry-After. The generated page
	// configures Datastar to retry the connection 10 times over about three minutes.
	//
	// Reaching the limit does not notify application code. Read one server's live
	// count with
	// [github.com/romshark/datapages/runtime/httpserve.Core.StateLiveInstances],
	// which the generated server embeds. Servers built with [EnablePrometheus]
	// also add their counts to the datapages_state_instances process gauge.
	//
	// Zero selects [DefaultMaxConcurrentInstances].
	// A negative value disables the limit.
	MaxConcurrentInstances int
}

// WithStateConfig sets the concurrent instance limit for handlers that use [State].
// A server built without it uses [DefaultMaxConcurrentInstances].
//
// State lives in one server process. In a multi-server deployment, the load balancer
// must send a page instance's stream and action requests to the same server.
// The Datapages-Instance request header is a stable routing key for that instance.
// Otherwise, an action can reach a server that has no state for
// the instance and make the generated client reload the page.
func WithStateConfig(conf StateConfig) ServerOption {
	return func(c *ServerConfig) error {
		if conf.MaxConcurrentInstances == 0 {
			conf.MaxConcurrentInstances = DefaultMaxConcurrentInstances
		}
		c.State = &conf
		return nil
	}
}

// WithCSPNonce turns on Content-Security-Policy nonce mode. nonce reports the
// nonce of a request. Datapages may call it several times while writing one response,
// and every call with the same request must return the same value.
// Application middleware should mint the nonce once, store it in the request
// context and write the same value into the script-src directive of the policy header.
// An empty return writes the page without nonces.
//
// Datapages then writes the nonce on every script it writes and on the html
// element as data-nonce, which is how Datastar's CSP mode reads it. Datastar compiles
// an attribute expression by appending a script element with that nonce instead of
// calling Function, which removes the need for script-src 'unsafe-eval'.
//
// If [WithDatastarJS] is used then [WithCSPNonce] requires Datastar 1.0.3 or later.
//
// A nonce in the policy makes the browser ignore 'unsafe-inline' for that directive.
// The browser runs inline scripts that carry the nonce and rejects
// injected inline scripts without it.
func WithCSPNonce(nonce func(r *http.Request) string) ServerOption {
	return func(c *ServerConfig) error {
		if nonce == nil {
			return errors.New("WithCSPNonce: nil function")
		}
		c.CSPNonce = nonce
		return nil
	}
}
