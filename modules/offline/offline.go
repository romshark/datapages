// Package offline adds service-worker-based offline support to a Datapages
// application.
//
// It is a pluggable module wired in through a single Datapages extension point:
// a middleware that (a) serves the generated service worker with whole-origin
// scope and (b) injects the worker-registration script into an HTML page only
// when the client reports (via the X-Datapages-Worker-Version header) that it
// has no current worker. The application's templates and its <head> stay
// untouched.
//
// The page cache itself is written by handlers through the
// datapages.PageCacheWriter parameter; this module only serves and registers
// the worker that stores and serves those entries. See the Service Worker
// section of SPECIFICATION.md.
package offline

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/romshark/datapages"
)

//go:embed sw.js
var serviceWorkerTemplate string

//go:embed register.js
var registerTemplate string

// Config configures offline behaviour.
type Config struct {
	// WorkerVersion is the service worker's own version. Bump it whenever the
	// worker script or the precached shell/offline set changes; the browser then
	// installs the new worker and drops caches from older versions. Zero selects
	// [DefaultWorkerVersion].
	WorkerVersion uint64

	// ScriptURL is the path the worker script is served from. Empty selects
	// [DefaultScriptURL]. Its scope is widened to the whole origin via the
	// Service-Worker-Allowed header regardless of this path.
	ScriptURL string

	// Assets is the application shell (CSS, JS, icons) precached on install so
	// cached pages still render while offline.
	Assets []string

	// OfflineClass is the class toggled on <html> while the browser is offline,
	// for styling offline state in CSS. Empty selects [DefaultOfflineClass].
	OfflineClass string

	// CrossOriginDestinations lists the Fetch request destinations cached when the
	// request goes to another origin, e.g. assets loaded from a CDN. Same-origin
	// requests are cached regardless of destination. Nil selects
	// [DefaultCrossOriginDestinations]; an empty non-nil slice disables cross-origin
	// caching entirely. Keep API and analytics destinations ("empty") out of it,
	// they must not be answered from a stale cache.
	CrossOriginDestinations []string
}

// DefaultWorkerVersion is the service worker version used when
// [Config.WorkerVersion] is zero. Versions start at 1 so that a client reporting
// no version at all is always recognised as having no worker installed.
const DefaultWorkerVersion uint64 = 1

// DefaultScriptURL is the path the worker script is served from when
// [Config.ScriptURL] is empty.
const DefaultScriptURL = "/service-worker.js"

// DefaultOfflineClass is the class toggled on <html> while offline when
// [Config.OfflineClass] is empty.
const DefaultOfflineClass = "is-offline"

// DefaultCrossOriginDestinations are the Fetch request destinations cached
// cross-origin when [Config.CrossOriginDestinations] is nil: the static subresource
// types a cached page needs in order to render.
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

// ServiceWorkerJS returns the generated service-worker JavaScript for cfg.
// offlinePath is the route of PageOffline, which the worker precaches and serves
// for navigations to uncached URLs while offline. Empty leaves the worker with its
// own minimal fallback.
func ServiceWorkerJS(offlinePath string, cfg Config) []byte {
	payload := struct {
		WorkerVersion           uint64   `json:"workerVersion"`
		OfflineURL              string   `json:"offlineURL"`
		Assets                  []string `json:"assets"`
		CrossOriginDestinations []string `json:"crossOriginDestinations"`
	}{
		WorkerVersion:           cfg.workerVersion(),
		OfflineURL:              offlinePath,
		Assets:                  cfg.Assets,
		CrossOriginDestinations: cfg.crossOriginDestinations(),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Errorf("offline: marshalling service worker config: %w", err))
	}
	js := strings.ReplaceAll(serviceWorkerTemplate, "__CONFIG__", string(encoded))
	return []byte(js)
}

// Middleware wires offline support into a Datapages server. Prefer the generated
// datapagesgen.WithOffline, which supplies offlinePath from the route declared on
// PageOffline; call this directly only when there is no PageOffline to derive it
// from. It:
//
//   - serves the generated service worker at cfg.ScriptURL, and
//   - injects the worker-registration script into an HTML page only when the
//     client has no current worker, i.e. when the X-Datapages-Worker-Version
//     request header is absent (not installed) or below cfg.WorkerVersion
//     (outdated). Once a current worker is installed it reports its version on
//     every request and the registration script is no longer sent, keeping
//     steady-state responses lean.
//
// Non-HTML responses (SSE streams, redirects, static assets) are passed through
// untouched.
func Middleware(offlinePath string, cfg Config) func(http.Handler) http.Handler {
	js := ServiceWorkerJS(offlinePath, cfg)
	target := cfg.scriptURL()
	targetSlash := target + "/"
	workerVer := cfg.workerVersion()
	// JSON-encode the class so any value is a safely escaped JS string literal.
	offlineClass, err := json.Marshal(cfg.offlineClass())
	if err != nil {
		panic(fmt.Errorf("offline: marshalling offline class: %w", err))
	}
	registerJS := strings.ReplaceAll(registerTemplate, "__SCRIPT_URL__", target)
	registerJS = strings.ReplaceAll(registerJS, "__OFFLINE_CLASS__", string(offlineClass))
	register := []byte("<script>" + registerJS + "</script>")

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

			// The client's installed worker reports its version. Skip injecting
			// the registration script when it is already current. A missing or
			// malformed header parses to 0 and injects, which is the safe way to fail.
			clientVer, _ := strconv.ParseUint(
				r.Header.Get(datapages.HeaderWorkerVersion), 10, 64,
			)
			if clientVer >= workerVer {
				next.ServeHTTP(w, r)
				return
			}

			iw := &injectingWriter{ResponseWriter: w, script: register}
			next.ServeHTTP(iw, r)
			iw.finish()
		})
	}
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
	// If the handler already committed to a non-HTML content type (SSE stream,
	// JS redirect, …), decide immediately and stream through.
	if ct := iw.Header().Get("Content-Type"); ct != "" &&
		!strings.HasPrefix(ct, "text/html") {
		iw.decided = true
		iw.inject = false
		iw.ResponseWriter.WriteHeader(code)
	}
	// Otherwise defer the decision until the first Write, where we can sniff the
	// body (Datapages relies on content sniffing for HTML pages).
}

func (iw *injectingWriter) Write(p []byte) (int, error) {
	if !iw.decided {
		if ct := iw.Header().Get("Content-Type"); ct != "" {
			iw.inject = strings.HasPrefix(ct, "text/html")
		} else {
			iw.inject = looksHTML(p)
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

// Flush supports streaming responses (e.g. Datastar SSE), which are never
// buffered.
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
		return // already streamed through
	}
	body := injectBefore(iw.buf.Bytes(), iw.script)
	h := iw.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Length", strconv.Itoa(len(body)))
	if iw.status == 0 {
		iw.status = http.StatusOK
	}
	iw.ResponseWriter.WriteHeader(iw.status)
	_, _ = iw.ResponseWriter.Write(body)
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
