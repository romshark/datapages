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
	cfg := offline.Config{WorkerVersion: 1}
	mw := offline.Middleware("/offline/", cfg)

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

	// The zero value must produce a usable worker at version 1. A client
	// reporting no version is recognised as having none installed.
	js := string(offline.ServiceWorkerJS("", offline.Config{}))
	require.Contains(t, js, `"workerVersion":1`)
	require.Equal(t, uint64(1), offline.DefaultWorkerVersion)

	// An empty offline path leaves the worker's own fallback in place.
	require.Contains(t, js, `"offlineURL":""`)
}
