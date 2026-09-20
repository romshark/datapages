package offline_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/modules/offline"
)

func TestServiceWorkerJS(t *testing.T) {
	js := string(offline.ServiceWorkerJS("/offline/", offline.Config{
		WorkerVersion: 42,
		Assets:        []string{"/static/style.css"},
	}))

	require.NotContains(t, js, "__CONFIG__", "config placeholder must be replaced")
	require.Contains(t, js, `"workerVersion":42`)
	require.Contains(t, js, `"offlineURL":"/offline/"`)
	require.Contains(t, js, `/static/style.css`)
}

func TestMiddleware(t *testing.T) {
	conf := offline.Config{WorkerVersion: 1}
	mw := offline.Middleware("/offline/", conf)

	type want struct {
		status      int
		contentType string
		bodyHas     []string
		bodyLacks   []string
	}
	cases := map[string]struct {
		req  *http.Request
		next http.HandlerFunc
		want want
	}{
		"serves service worker": {
			req: httptest.NewRequest(http.MethodGet, "/service-worker.js", nil),
			next: func(w http.ResponseWriter, r *http.Request) {
				t.Error("next must not be called for the worker path")
			},
			want: want{
				status:      http.StatusOK,
				contentType: "text/javascript; charset=utf-8",
				bodyHas:     []string{"addEventListener"},
			},
		},
		"injects registration into HTML": {
			req: httptest.NewRequest(http.MethodGet, "/shows/", nil),
			next: func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(
					"<!DOCTYPE html><html><head><title>x</title></head><body>hi</body></html>",
				))
			},
			want: want{
				status:  http.StatusOK,
				bodyHas: []string{"serviceWorker.register('/service-worker.js')", "</head>"},
			},
		},
		"passes through SSE untouched": {
			req: httptest.NewRequest(http.MethodGet, "/shows/search/", nil),
			next: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("event: datastar-patch-elements\ndata: x\n\n"))
			},
			want: want{
				status:    http.StatusOK,
				bodyHas:   []string{"datastar-patch-elements"},
				bodyLacks: []string{"serviceWorker.register"},
			},
		},
		"passes through non-HTML asset untouched": {
			req: httptest.NewRequest(http.MethodGet, "/static/style.css", nil),
			next: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/css")
				_, _ = w.Write([]byte("body{color:red}"))
			},
			want: want{
				status:    http.StatusOK,
				bodyHas:   []string{"body{color:red}"},
				bodyLacks: []string{"serviceWorker.register"},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mw(tc.next).ServeHTTP(rec, tc.req)

			require.Equal(t, tc.want.status, rec.Code)
			if tc.want.contentType != "" {
				require.Equal(t, tc.want.contentType, rec.Header().Get("Content-Type"))
			}
			body := rec.Body.String()
			for _, s := range tc.want.bodyHas {
				require.Contains(t, body, s)
			}
			for _, s := range tc.want.bodyLacks {
				require.NotContains(t, body, s)
			}
		})
	}
}

func TestMiddlewareServiceWorkerHeaders(t *testing.T) {
	mw := offline.Middleware("/offline/", offline.Config{WorkerVersion: 1})
	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/service-worker.js", nil))

	require.Equal(t, "/", rec.Header().Get("Service-Worker-Allowed"))
	require.True(t, strings.Contains(rec.Header().Get("Cache-Control"), "no-cache"))
}

func TestMiddlewareOfflineClass(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		conf      offline.Config
		wantHas   string
		wantLacks string
	}{
		"default": {
			conf:    offline.Config{WorkerVersion: 1},
			wantHas: `classList.toggle("is-offline"`,
		},
		"custom": {
			conf:      offline.Config{WorkerVersion: 1, OfflineClass: "app-offline"},
			wantHas:   `classList.toggle("app-offline"`,
			wantLacks: "is-offline",
		},
		"escaped": {
			conf:    offline.Config{WorkerVersion: 1, OfflineClass: `a"b`},
			wantHas: `classList.toggle("a\"b"`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mw := offline.Middleware("/offline/", tc.conf)
			rec := httptest.NewRecorder()
			mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("<!DOCTYPE html><html><head></head><body>x</body></html>"))
			})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

			body := rec.Body.String()
			require.Contains(t, body, tc.wantHas)
			if tc.wantLacks != "" {
				require.NotContains(t, body, tc.wantLacks)
			}
		})
	}
}

func TestServiceWorkerCrossOriginDestinations(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		conf offline.Config
		want string
	}{
		"default when nil": {
			conf: offline.Config{WorkerVersion: 1},
			want: `"crossOriginDestinations":["image","style","script","font"]`,
		},
		"custom": {
			conf: offline.Config{
				WorkerVersion:           1,
				CrossOriginDestinations: []string{"image"},
			},
			want: `"crossOriginDestinations":["image"]`,
		},
		"empty disables cross-origin caching": {
			conf: offline.Config{
				WorkerVersion:           1,
				CrossOriginDestinations: []string{},
			},
			want: `"crossOriginDestinations":[]`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Contains(t, string(offline.ServiceWorkerJS("/offline/", tc.conf)), tc.want)
		})
	}
}

func TestConfigDefaults(t *testing.T) {
	t.Parallel()

	js := string(offline.ServiceWorkerJS("", offline.Config{}))
	require.Contains(t, js, `"workerVersion":1`)
	require.Equal(t, uint64(1), offline.DefaultWorkerVersion)

	require.Contains(t, js, `"offlineURL":""`)
}

// TestMiddlewareReflectsStateOnceTheWorkerIsCurrent tests that a client
// reporting the shipped worker version still receives the online/offline
// reflection script and no longer receives the registration script.
func TestMiddlewareReflectsStateOnceTheWorkerIsCurrent(t *testing.T) {
	t.Parallel()
	mw := offline.Middleware("/offline/", offline.Config{WorkerVersion: 3})

	req := httptest.NewRequest(http.MethodGet, "/shows/", nil)
	req.Header.Set("X-Datapages-Worker-Version", "3")
	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(
			"<!DOCTYPE html><html><head><title>x</title></head><body>hi</body></html>",
		))
	})).ServeHTTP(rec, req)

	require.Contains(t, rec.Body.String(), `classList.toggle("is-offline"`)
	require.NotContains(t, rec.Body.String(), "serviceWorker.register")
}

// TestServiceWorkerIncludesTheReflectionScript tests cached pages that bypass
// the middleware.
func TestServiceWorkerIncludesTheReflectionScript(t *testing.T) {
	t.Parallel()
	js := string(offline.ServiceWorkerJS("/offline/",
		offline.Config{WorkerVersion: 1, OfflineClass: "app-offline"}))

	require.Contains(t, js, `classList.toggle(\"app-offline\"`)
}

// TestMiddlewareVariesByWorkerVersion tests that an HTML response declares the
// request header its body depends on, which keeps a shared cache from serving
// one client's copy to another.
func TestMiddlewareVariesByWorkerVersion(t *testing.T) {
	t.Parallel()
	mw := offline.Middleware("/offline/", offline.Config{WorkerVersion: 1})

	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<!DOCTYPE html><html><head></head><body>hi</body></html>"))
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/shows/", nil))

	require.Equal(t, "X-Datapages-Worker-Version", rec.Header().Get("Vary"))
}

// TestServiceWorkerExcludePaths tests that configured same-origin prefixes use
// the network.
func TestServiceWorkerExcludePaths(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		conf offline.Config
		want string
	}{
		"none": {offline.Config{WorkerVersion: 1}, `"excludePaths":null`},
		"configured": {
			offline.Config{WorkerVersion: 1, ExcludePaths: []string{"/api/", "/live/"}},
			`"excludePaths":["/api/","/live/"]`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Contains(t,
				string(offline.ServiceWorkerJS("/offline/", tc.conf)), tc.want)
		})
	}
}

// TestOfflineClassCannotEndTheScript tests that a class spelling a closing
// script tag reaches the browser as an escaped string literal.
func TestOfflineClassCannotEndTheScript(t *testing.T) {
	t.Parallel()
	mw := offline.Middleware("/offline/", offline.Config{
		WorkerVersion: 1,
		OfflineClass:  `x</script><script>alert(1)`,
	})

	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<!DOCTYPE html><html><head></head><body>hi</body></html>"))
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/shows/", nil))

	require.NotContains(t, rec.Body.String(), "<script>alert(1)",
		"the class must not end the script element it sits in")
	require.Contains(t, rec.Body.String(), `</script>`,
		"json.Marshal escapes < and >")
}

// TestServiceWorkerPatchesTheHead tests that a shim update includes the live
// head before the body. Sessionless shims contain no CSRF script.
func TestServiceWorkerPatchesTheHead(t *testing.T) {
	t.Parallel()
	js := string(offline.ServiceWorkerJS("/offline/", offline.Config{WorkerVersion: 1}))

	head := strings.Index(js, `"selector","head"`)
	body := strings.Index(js, `"selector","body"`)
	require.GreaterOrEqual(t, head, 0, "no head frame")
	require.GreaterOrEqual(t, body, 0, "no body frame")
	require.Less(t, head, body,
		"the head frame is written first: the CSRF wrapper has to be"+
			" installed before a binding in the new body fires an action")
}

// TestServiceWorkerHydratesWithoutThePrefetch tests that the worker can fetch a
// new response after a restart removes the saved response.
func TestServiceWorkerHydratesWithoutThePrefetch(t *testing.T) {
	t.Parallel()
	js := string(offline.ServiceWorkerJS("/offline/", offline.Config{WorkerVersion: 1}))

	require.Contains(t, js, `HYDRATE_HEADER="X-Datapages-Shim-Hydrate"`)
	require.Contains(t, js, "pageFetchHeaders",
		"a lost prefetch is replaced by a fetch the worker makes itself")
}

// TestMiddlewareContentEncoding tests that a body carrying a content coding
// passes through untouched, because splicing a script into it would corrupt it.
func TestMiddlewareContentEncoding(t *testing.T) {
	t.Parallel()

	const page = "<!DOCTYPE html><html><head></head><body>x</body></html>"

	for name, tc := range map[string]struct {
		encoding   string
		wantScript bool
	}{
		"gzip":     {encoding: "gzip"},
		"brotli":   {encoding: "br"},
		"chained":  {encoding: "gzip, br"},
		"padded":   {encoding: " gzip "},
		"identity": {encoding: "identity", wantScript: true},
		"absent":   {encoding: "", wantScript: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mw := offline.Middleware("/offline/", offline.Config{WorkerVersion: 1})
			rec := httptest.NewRecorder()
			mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				if tc.encoding != "" {
					w.Header().Set("Content-Encoding", tc.encoding)
				}
				_, _ = w.Write([]byte(page))
			})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

			if !tc.wantScript {
				require.Equal(t, page, rec.Body.String())
				return
			}
			require.Contains(t, rec.Body.String(), "<script>")
		})
	}
}
