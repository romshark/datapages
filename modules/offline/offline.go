// Package offline serves and registers the service worker used by
// datapages.PageCacheWriter. Its middleware also reflects browser connectivity
// on the document's root element.
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

// "mage genOfflineWorker" writes sw.min.js from sw.js.
//
//go:embed sw.min.js
var serviceWorkerTemplate string

//go:embed register.js
var registerTemplate string

//go:embed netstate.js
var netStateTemplate string

// Config configures offline behavior.
type Config struct {
	// WorkerVersion is the service worker's own version. Increment it when the
	// worker script or precached files change. The browser installs the new
	// worker and removes caches from older versions.
	// Zero selects [DefaultWorkerVersion].
	//
	// A changed asset at an unchanged URL needs no bump: a cached asset is
	// refreshed from the network behind the copy it serves.
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
	// [Middleware] adds it to each script it writes. If nil, the scripts have no
	// nonce.
	//
	// Generated code uses
	// [github.com/romshark/datapages.WithCSPNonce] when CSPNonce is nil,
	// regardless of option order.
	CSPNonce func(r *http.Request) string

	// ExcludePaths lists same-origin URL path prefixes the worker never caches
	// and always passes to the network. Use it for an endpoint the application's
	// own JavaScript calls whose answer must not come from a stale copy.
	// Navigations and the requests Datastar issues bypass the cache already.
	ExcludePaths []string
}

// DefaultWorkerVersion is the service worker version used when
// [Config.WorkerVersion] is zero. Versions start at 1 so that a client reporting
// no version at all is always recognised as having no worker installed.
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
// offlinePath is the route of PageOffline, which the worker precaches and serves
// for navigations to uncached URLs while offline.
// Empty leaves the worker with its own minimal fallback.
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

// netStateJS returns the online/offline reflection script of conf.
//
// [json.Marshal] escapes <, > and &. The encoded class cannot close the script element.
// A [json.Encoder] with SetEscapeHTML(false) would permit that.
func netStateJS(conf Config) string {
	class, err := json.Marshal(conf.offlineClass())
	if err != nil {
		panic(fmt.Errorf("offline: marshalling offline class: %w", err))
	}
	return strings.ReplaceAll(netStateTemplate, "__OFFLINE_CLASS__", string(class))
}

// Middleware serves the service worker and adds its connectivity script to HTML
// responses. It adds registration when the X-Datapages-Worker-Version request
// header is absent or lower than conf.WorkerVersion. Non-HTML responses pass
// through unchanged.
//
// A response that already carries a Content-Encoding passes through unchanged,
// because an encoded body cannot be edited as bytes. Register a compressing
// middleware before this one so that it compresses what this one rewrote.
//
// Prefer datapagesgen.WithOffline when the application declares PageOffline.
// The generated option supplies offlinePath from that page's route.
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

			// A missing or malformed value becomes 0, which causes registration.
			clientVer, _ := strconv.ParseUint(
				r.Header.Get(datapages.HeaderWorkerVersion), 10, 64,
			)
			withRegister := clientVer < workerVer
			script := netState
			if withRegister {
				script = register
			}
			if conf.CSPNonce != nil {
				// Build the tags per request because the nonce differs per response.
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

// injectingWriter buffers HTML responses so the registration script can be
// injected before </head>, and streams everything else through untouched.
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
	// Datapages pages rely on content sniffing, so defer an undeclared type until
	// the first Write.
}

func (iw *injectingWriter) Write(p []byte) (int, error) {
	if !iw.decided {
		if ct := iw.Header().Get("Content-Type"); ct != "" {
			iw.inject = strings.HasPrefix(ct, "text/html")
		} else {
			iw.inject = looksHTML(p)
		}
		if iw.inject && encoded(iw.Header()) {
			// Splicing a script into a compressed body corrupts it.
			// Register the compressing middleware before this one to keep the injection.
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

// Flush supports streaming responses (e.g. Datastar SSE), which are never buffered.
func (iw *injectingWriter) Flush() {
	if iw.inject {
		return
	}
	if f, ok := iw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap exposes the underlying writer to http.ResponseController.
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
	// The registration script is injected by version header.
	// Without Vary a shared cache can serve a page carrying it to a client that
	// already runs the worker, and one without it to a client that has none.
	h.Add("Vary", datapages.HeaderWorkerVersion)
	h.Set("Content-Length", strconv.Itoa(len(body)))
	if iw.status == 0 {
		iw.status = http.StatusOK
	}
	iw.ResponseWriter.WriteHeader(iw.status)
	_, _ = iw.ResponseWriter.Write(body)
}

// encoded reports whether the body carries a content coding, which leaves it
// unreadable as HTML. A middleware registered after this one compresses what
// this one already rewrote, so only an inverted order reaches this case.
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

// injectBefore inserts script before the first </head> (or </body>) tag in body,
// appending it if neither is present.
func injectBefore(body, script []byte) []byte {
	lower := bytes.ToLower(body)
	for _, marker := range [][]byte{[]byte("</head>"), []byte("</body>")} {
		if i := bytes.Index(lower, marker); i >= 0 {
			out := make([]byte, 0, len(body)+len(script))
			out = append(out, body[:i]...)
			out = append(out, script...)
			out = append(out, body[i:]...)
			return out
		}
	}
	return append(body, script...)
}
