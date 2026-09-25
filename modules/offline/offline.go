// Package offline serves and registers the service worker used by
// [datapages.PageCacheWriter]. [Middleware] reflects browser connectivity on
// the document's root element.
package offline

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"

	"github.com/romshark/datapages"
)

// Run mage genOfflineWorker to write sw.min.js from sw.js.
//
//go:embed sw.min.js
var serviceWorkerTemplate string

//go:embed register.js
var registerTemplate string

//go:embed netstate.js
var netStateTemplate string

// Config configures the service worker and its HTML response middleware.
type Config struct {
	// WorkerVersion identifies the installed service worker and its cache.
	// Increasing it makes the browser install the new worker and delete caches
	// from earlier versions. Zero selects [DefaultWorkerVersion].
	//
	// Increase WorkerVersion after a Datapages upgrade or a change to
	// [Config.Assets], [Config.ExcludePaths], [Config.CrossOriginDestinations],
	// [Config.OfflineClass], or PageOffline. [ServiceWorkerJS] embeds the config
	// values and offlinePath in the script it serves. The worker fetches
	// [Config.Assets] and PageOffline only during installation.
	//
	// A changed asset at an unchanged URL does not require a new worker version.
	// The cache refreshes it from the network after serving the stored copy.
	WorkerVersion uint64

	// ScriptURL is the path the worker script is served from. Empty selects
	// [DefaultScriptURL]. Its scope is widened to the whole origin via the
	// Service-Worker-Allowed header regardless of this path.
	ScriptURL string

	// Assets lists the CSS, JavaScript and images cached during installation.
	// Cached pages can use these files while offline.
	Assets []string

	// OfflineClass is the class toggled on <html> while the browser is offline,
	// for styling offline state in CSS. Empty selects [DefaultOfflineClass].
	OfflineClass string

	// CrossOriginDestinations lists Fetch request destinations cached when a
	// request goes to another origin, for example assets loaded from a CDN.
	// Same-origin requests are cached regardless of destination.
	// Nil selects [DefaultCrossOriginDestinations]. An empty non-nil slice
	// disables cross-origin caching. Exclude API and analytics destinations
	// because their responses must not come from a stale cache.
	CrossOriginDestinations []string

	// CSPNonce returns the Content-Security-Policy nonce for a request.
	// [Middleware] adds it to each script it writes. When CSPNonce is nil,
	// [Middleware] writes scripts without a nonce.
	//
	// [WithServiceWorker] fills a nil CSPNonce from [datapages.WithCSPNonce],
	// regardless of option order.
	CSPNonce func(r *http.Request) string

	// ExcludePaths lists same-origin URL path prefixes the worker never caches
	// and always passes to the network. Use it for an endpoint the application's
	// own JavaScript calls whose answer must not come from a stale copy.
	// Navigations and the requests Datastar issues bypass the cache already.
	ExcludePaths []string
}

// DefaultWorkerVersion is the service worker version used when
// [Config.WorkerVersion] is zero. Versions start at 1. [Middleware] treats a
// request without [datapages.HeaderWorkerVersion] as version 0 and adds the
// registration script.
const DefaultWorkerVersion uint64 = 1

// DefaultScriptURL is the path the worker script is
// served from when [Config.ScriptURL] is empty.
const DefaultScriptURL = "/service-worker.js"

// DefaultOfflineClass is the class toggled on <html> while
// offline when [Config.OfflineClass] is empty.
const DefaultOfflineClass = "is-offline"

// DefaultCrossOriginDestinations are the Fetch request destinations cached
// cross-origin when [Config.CrossOriginDestinations] is nil:
// the static subresource types a cached page needs in order to render.
var DefaultCrossOriginDestinations = []string{"image", "style", "script", "font"}

func (c Config) workerVersion() uint64 {
	if c.WorkerVersion == 0 {
		return DefaultWorkerVersion
	}
	return c.WorkerVersion
}

func (c Config) offlineClass() string {
	if c.OfflineClass == "" {
		return DefaultOfflineClass
	}
	return c.OfflineClass
}

func (c Config) crossOriginDestinations() []string {
	if c.CrossOriginDestinations == nil {
		return DefaultCrossOriginDestinations
	}
	return c.CrossOriginDestinations
}

func (c Config) scriptURL() string {
	if c.ScriptURL == "" {
		return DefaultScriptURL
	}
	return c.ScriptURL
}

// ServiceWorkerJS returns the generated service worker JavaScript for conf.
// offlinePath is the route of PageOffline. The worker precaches it and serves it
// for navigations to uncached URLs while offline. An empty offlinePath uses the
// worker's minimal fallback.
func ServiceWorkerJS(offlinePath string, conf Config) []byte {
	payload := struct {
		WorkerVersion           uint64   `json:"workerVersion"`
		OfflineURL              string   `json:"offlineURL"`
		Assets                  []string `json:"assets"`
		CrossOriginDestinations []string `json:"crossOriginDestinations"`
		ExcludePaths            []string `json:"excludePaths"`
		NetStateJS              string   `json:"netStateJS"`
	}{
		WorkerVersion:           conf.workerVersion(),
		OfflineURL:              offlinePath,
		Assets:                  conf.Assets,
		CrossOriginDestinations: conf.crossOriginDestinations(),
		ExcludePaths:            conf.ExcludePaths,
		NetStateJS:              netStateJS(conf),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Errorf("offline: marshalling service worker config: %w", err))
	}
	js := strings.ReplaceAll(serviceWorkerTemplate, "__CONFIG__", string(encoded))
	return []byte(js)
}

// [json.Marshal] escapes <, > and &. The encoded class cannot close the script element.
// A [json.Encoder] with SetEscapeHTML(false) would permit that.
func netStateJS(conf Config) string {
	class, err := json.Marshal(conf.offlineClass())
	if err != nil {
		panic(fmt.Errorf("offline: marshalling offline class: %w", err))
	}
	return strings.ReplaceAll(netStateTemplate, "__OFFLINE_CLASS__", string(class))
}

// WithServiceWorker returns the server option that installs [Middleware].
//
// When [Config.CSPNonce] is nil, the option reads the nonce configured by
// [datapages.WithCSPNonce] for each request. Either option order works.
// Installing [Middleware] through [datapages.WithMiddleware] does not read
// [datapages.ServerConfig.CSPNonce]. Set [Config.CSPNonce] explicitly when
// using the middleware directly; a nonce-only policy otherwise blocks its scripts.
//
// Applications declaring PageOffline use the generated datapagesgen.WithOffline option.
// It supplies offlinePath from that page's route.
func WithServiceWorker(offlinePath string, conf Config) datapages.ServerOption {
	return func(c *datapages.ServerConfig) error {
		// Copy the config for each server. A reused option must bind
		// [Config.CSPNonce] to the current [datapages.ServerConfig].
		conf := conf
		if conf.CSPNonce == nil {
			conf.CSPNonce = func(r *http.Request) string {
				if c.CSPNonce == nil {
					return ""
				}
				return c.CSPNonce(r)
			}
		}
		return datapages.WithMiddleware(Middleware(offlinePath, conf))(c)
	}
}

// Middleware serves the service worker and adds its connectivity script to HTML
// responses. It adds registration when [datapages.HeaderWorkerVersion] is absent or
// lower than the configured worker version. Non-HTML responses pass through unchanged.
//
// A response that already carries a Content-Encoding passes through unchanged,
// because an encoded body cannot be edited as bytes. Register a compressing
// middleware before [Middleware]. The compressor then receives the rewritten HTML.
//
// Applications declaring PageOffline should use the generated
// datapagesgen.WithOffline option. It supplies offlinePath from that page's route.
func Middleware(offlinePath string, conf Config) func(http.Handler) http.Handler {
	js := ServiceWorkerJS(offlinePath, conf)
	target := conf.scriptURL()
	targetSlash := target + "/"
	workerVer := conf.workerVersion()
	netStateSrc := netStateJS(conf)
	registerSrc := strings.ReplaceAll(registerTemplate, "__SCRIPT_URL__", target)
	netState := []byte(scriptTag("") + netStateSrc + "</script>")
	register := append(append([]byte(nil), netState...),
		scriptTag("")+registerSrc+"</script>"...)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet &&
				(r.URL.Path == target || r.URL.Path == targetSlash) {
				w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
				w.Header().Set("Service-Worker-Allowed", "/")
				w.Header().Set("Cache-Control", "no-cache")
				_, _ = w.Write(js)
				return
			}

			// A missing or malformed [datapages.HeaderWorkerVersion] parses as 0
			// and triggers registration.
			clientVer, _ := strconv.ParseUint(
				r.Header.Get(datapages.HeaderWorkerVersion), 10, 64,
			)
			withRegister := clientVer < workerVer
			script := netState
			if withRegister {
				script = register
			}
			if conf.CSPNonce != nil {
				// Build script tags per request because the nonce differs per response.
				if nonce := html.EscapeString(conf.CSPNonce(r)); nonce != "" {
					script = []byte(scriptTag(nonce) + netStateSrc + "</script>")
					if withRegister {
						script = append(script,
							scriptTag(nonce)+registerSrc+"</script>"...)
					}
				}
			}

			iw := &injectingWriter{ResponseWriter: w, script: script}
			next.ServeHTTP(iw, r)
			iw.finish()
		})
	}
}

func scriptTag(nonce string) string {
	if nonce == "" {
		return "<script>"
	}
	return `<script nonce="` + nonce + `">`
}

// injectingWriter buffers HTML responses for registration script injection
// before </head>. It streams every other response unchanged.
type injectingWriter struct {
	http.ResponseWriter
	script []byte

	decided bool
	inject  bool
	status  int
	buf     bytes.Buffer
}

func (iw *injectingWriter) WriteHeader(code int) {
	if iw.decided {
		return
	}
	iw.status = code
	// A declared non-HTML type can pass through without body sniffing.
	if ct := iw.Header().Get("Content-Type"); ct != "" &&
		!strings.HasPrefix(ct, "text/html") {
		iw.decided = true
		iw.inject = false
		iw.ResponseWriter.WriteHeader(code)
	}
	// Datapages pages rely on content sniffing.
	// Defer an undeclared type until the first body write.
}

func (iw *injectingWriter) Write(p []byte) (int, error) {
	if !iw.decided {
		if ct := iw.Header().Get("Content-Type"); ct != "" {
			iw.inject = strings.HasPrefix(ct, "text/html")
		} else {
			iw.inject = looksHTML(p)
		}
		if iw.inject && encoded(iw.Header()) {
			// [Middleware] cannot splice a script into a compressed body. Register
			// the compressing middleware before [Middleware] to keep script injection.
			iw.inject = false
		}
		iw.decided = true
		if !iw.inject {
			if iw.status == 0 {
				iw.status = http.StatusOK
			}
			iw.ResponseWriter.WriteHeader(iw.status)
		}
	}
	if iw.inject {
		return iw.buf.Write(p)
	}
	return iw.ResponseWriter.Write(p)
}

// Flush supports streaming responses such as Datastar SSE.
// [Middleware] doesn't buffer them.
func (iw *injectingWriter) Flush() {
	if iw.inject {
		return
	}
	if f, ok := iw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap exposes the underlying writer to [http.ResponseController].
func (iw *injectingWriter) Unwrap() http.ResponseWriter { return iw.ResponseWriter }

func (iw *injectingWriter) finish() {
	if !iw.decided {
		if iw.status != 0 {
			iw.ResponseWriter.WriteHeader(iw.status)
		}
		return
	}
	if !iw.inject {
		return
	}
	body := injectBefore(iw.buf.Bytes(), iw.script)
	h := iw.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	// Registration depends on [datapages.HeaderWorkerVersion]. Vary prevents a
	// shared cache from sending the registration script to a client with the
	// current worker or omitting it for a client without the worker.
	h.Add("Vary", datapages.HeaderWorkerVersion)
	h.Set("Content-Length", strconv.Itoa(len(body)))
	if iw.status == 0 {
		iw.status = http.StatusOK
	}
	iw.ResponseWriter.WriteHeader(iw.status)
	_, _ = iw.ResponseWriter.Write(body)
}

// encoded reports whether the body has a non-identity content coding.
// [Middleware] cannot edit encoded bytes. A compressing middleware registered
// after [Middleware] passes encoded bytes to [Middleware].
func encoded(h http.Header) bool {
	ce := strings.TrimSpace(h.Get("Content-Encoding"))
	return ce != "" && !strings.EqualFold(ce, "identity")
}

func looksHTML(p []byte) bool {
	s := bytes.TrimLeft(p, " \t\r\n")
	if len(s) > 512 {
		s = s[:512]
	}
	s = bytes.ToLower(s)
	return bytes.HasPrefix(s, []byte("<!doctype html")) ||
		bytes.HasPrefix(s, []byte("<html"))
}

// injectBefore inserts script before the first </head> tag, or before </body>
// when no </head> exists. It appends script when neither tag exists.
func injectBefore(body, script []byte) []byte {
	for _, marker := range [][]byte{[]byte("</head>"), []byte("</body>")} {
		if i := indexFoldASCII(body, marker); i >= 0 {
			out := make([]byte, 0, len(body)+len(script))
			out = append(out, body[:i]...)
			out = append(out, script...)
			out = append(out, body[i:]...)
			return out
		}
	}
	return append(body, script...)
}

// indexFoldASCII returns the first marker in s under ASCII case-insensitive matching,
// or -1. marker must be lowercase ASCII and start with a non-letter.
//
// [bytes.ToLower] cannot provide offsets into arbitrary s because Unicode
// lowercasing and invalid UTF-8 replacement can change byte lengths.
func indexFoldASCII(s, marker []byte) int {
	for off := 0; ; off++ {
		i := bytes.IndexByte(s[off:], marker[0])
		if i < 0 {
			return -1
		}
		off += i
		if len(s)-off < len(marker) {
			return -1
		}
		if equalFoldASCII(s[off:off+len(marker)], marker) {
			return off
		}
	}
}

// equalFoldASCII reports whether s matches lower,
// a same-length lowercase ASCII value, ignoring ASCII letter case.
func equalFoldASCII(s, lower []byte) bool {
	for i, c := range s {
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != lower[i] {
			return false
		}
	}
	return true
}
