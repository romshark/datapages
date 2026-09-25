package offline_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
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

// TestMiddlewareInjectsBeforeHead tests closing-tag insertion
// when lowercasing preceding text changes its byte length.
func TestMiddlewareInjectsBeforeHead(t *testing.T) {
	mw := offline.Middleware("/offline/", offline.Config{WorkerVersion: 1})
	script := injectedScript(mw)
	require.True(t, strings.HasPrefix(script, "<script>"))

	for name, tc := range map[string]struct{ head, tail string }{
		"capital I with dot": {
			head: "<!DOCTYPE html><html><head><title>İletişim</title>",
			tail: "</head><body><h1>x</h1></body></html>",
		},
		"kelvin sign": {
			head: "<!DOCTYPE html><html><head><title>300K</title>",
			tail: "</head><body><h1>x</h1></body></html>",
		},
		// Lowercasing replaces each invalid byte with the three-byte U+FFFD encoding.
		"invalid utf-8": {
			head: "<!DOCTYPE html><html><head><title>" +
				strings.Repeat("\xff", 20) + "</title>",
			tail: "</head><body>x</body></html>",
		},
		"uppercase tag": {
			head: "<!DOCTYPE html><html><head><title>x</title>",
			tail: "</HEAD><BODY>x</BODY></HTML>",
		},
		"no head": {
			head: "<!DOCTYPE html><html><body>İ",
			tail: "</body></html>",
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.head+script+tc.tail, serve(mw, tc.head+tc.tail))
		})
	}
}

// TestMiddlewareInjectsBeforeHeadAfterDenseTags tests every </head> offset
// across the first two chunks of the dense-candidate fallback.
func TestMiddlewareInjectsBeforeHeadAfterDenseTags(t *testing.T) {
	mw := offline.Middleware("/offline/", offline.Config{WorkerVersion: 1})
	script := injectedScript(mw)
	for n := range 4200 {
		head := "<html>" + strings.Repeat("<", n)
		tail := "</HeAd><body>x</body>"
		if got := serve(mw, head+tail); got != head+script+tail {
			t.Fatalf("%d '<' before </head>: got %q", n, got)
		}
	}
}

// FuzzMiddlewareInjectsBeforeHead uses an ASCII-lowercased copy as the
// insertion-position oracle for arbitrary pages.
func FuzzMiddlewareInjectsBeforeHead(f *testing.F) {
	for _, seed := range []string{
		"", "<head></head>", "</HEAD>", "</body>", "<</head>", "</hea</head>",
		"İ</head>", "\xff</head>", strings.Repeat("<", 100) + "</HeAd>",
		strings.Repeat("</", 3000) + "</head>",
	} {
		f.Add(seed)
	}
	mw := offline.Middleware("/offline/", offline.Config{WorkerVersion: 1})
	script := injectedScript(mw)
	f.Fuzz(func(t *testing.T, s string) {
		page := "<html>" + s
		lower := []byte(page)
		for i, c := range lower {
			if 'A' <= c && c <= 'Z' {
				lower[i] = c + 'a' - 'A'
			}
		}
		want := page + script
		for _, marker := range []string{"</head>", "</body>"} {
			if i := strings.Index(string(lower), marker); i >= 0 {
				want = page[:i] + script + page[i:]
				break
			}
		}
		if got := serve(mw, page); got != want {
			t.Fatalf("page %q: got %q", page, got)
		}
	})
}

func serve(mw func(http.Handler) http.Handler, page string) string {
	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, page)
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	return rec.Body.String()
}

func injectedScript(mw func(http.Handler) http.Handler) string {
	return strings.TrimSuffix(
		strings.TrimPrefix(serve(mw, "<html></head>"), "<html>"), "</head>",
	)
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
// reporting the configured worker version receives the online/offline
// reflection script but not the registration script.
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

// TestServiceWorkerIncludesTheReflectionScript tests that the service worker
// adds the connectivity script to cached pages that bypass the middleware.
func TestServiceWorkerIncludesTheReflectionScript(t *testing.T) {
	t.Parallel()
	js := string(offline.ServiceWorkerJS("/offline/",
		offline.Config{WorkerVersion: 1, OfflineClass: "app-offline"}))

	require.Contains(t, js, `classList.toggle(\"app-offline\"`)
}

// TestMiddlewareVariesByWorkerVersion tests that an HTML response includes its
// worker-version request header in Vary to prevent a shared cache from mixing
// registration states.
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

// TestOfflineClassCannotEndTheScript tests that
// [offline.Config.OfflineClass] cannot close its inline script element.
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
		"OfflineClass must not close the script element")
	require.Contains(t, rec.Body.String(), `</script>`,
		"json.Marshal must escape angle brackets")
}

// TestServiceWorkerPatchesTheHead tests that a shim update sends the live head
// before the body. Body bindings need the session CSRF script from the head.
func TestServiceWorkerPatchesTheHead(t *testing.T) {
	t.Parallel()
	js := string(offline.ServiceWorkerJS("/offline/", offline.Config{WorkerVersion: 1}))

	head := strings.Index(js, `"selector","head"`)
	body := strings.Index(js, `"selector","body"`)
	require.GreaterOrEqual(t, head, 0, "head patch missing")
	require.GreaterOrEqual(t, body, 0, "body patch missing")
	require.Less(t, head, body,
		"head must precede body; bindings need the CSRF script before sending actions")
}

// TestServiceWorkerHydratesWithoutThePrefetch tests that the worker can fetch a
// new response after a restart removes the saved response.
func TestServiceWorkerHydratesWithoutThePrefetch(t *testing.T) {
	t.Parallel()
	js := string(offline.ServiceWorkerJS("/offline/", offline.Config{WorkerVersion: 1}))

	require.Contains(t, js, `HYDRATE_HEADER="X-Datapages-Shim-Hydrate"`)
	require.Contains(t, js, "pageFetchHeaders",
		"worker must fetch a replacement when the prefetched response is unavailable")
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

// TestMiddlewareCSPNonce tests that connectivity and registration scripts use
// the request nonce when configured.
func TestMiddlewareCSPNonce(t *testing.T) {
	t.Parallel()

	const page = "<!DOCTYPE html><html><head></head><body>x</body></html>"

	for name, tc := range map[string]struct {
		nonce      func(*http.Request) string
		clientVer  string
		wantTags   int
		wantNonced int
	}{
		"registration and connectivity": {
			nonce:      func(*http.Request) string { return "n0nce" },
			wantTags:   2,
			wantNonced: 2,
		},
		"connectivity after installation": {
			nonce:      func(*http.Request) string { return "n0nce" },
			clientVer:  "1",
			wantTags:   1,
			wantNonced: 1,
		},
		"unset": {
			nonce:    nil,
			wantTags: 2,
		},
		"empty": {
			nonce:    func(*http.Request) string { return "" },
			wantTags: 2,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mw := offline.Middleware("/offline/", offline.Config{
				WorkerVersion: 1, CSPNonce: tc.nonce,
			})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.clientVer != "" {
				req.Header.Set(datapages.HeaderWorkerVersion, tc.clientVer)
			}
			mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(page))
			})).ServeHTTP(rec, req)

			body := rec.Body.String()
			require.Equal(t, tc.wantTags, strings.Count(body, "<script"))
			require.Equal(t, tc.wantNonced,
				strings.Count(body, `<script nonce="n0nce">`))
		})
	}
}

// TestMiddlewareCSPNonceEscaped tests that a nonce cannot end the script tag.
func TestMiddlewareCSPNonceEscaped(t *testing.T) {
	t.Parallel()

	mw := offline.Middleware("/offline/", offline.Config{
		WorkerVersion: 1,
		CSPNonce:      func(*http.Request) string { return `a"><img src=x>` },
	})
	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<!DOCTYPE html><html><head></head><body></body></html>"))
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	require.Contains(t, rec.Body.String(), `<script nonce="a&#34;&gt;&lt;img src=x&gt;">`)
	require.NotContains(t, rec.Body.String(), "<img src=x>")
}

// TestWithServiceWorkerUsesServerNonce tests that a later
// [datapages.WithCSPNonce] setting reaches the middleware.
//
// [offline.Config.CSPNonce] takes precedence over the server setting.
func TestWithServiceWorkerUsesServerNonce(t *testing.T) {
	t.Parallel()

	const page = "<!DOCTYPE html><html><head></head><body></body></html>"
	serverNonce := func(*http.Request) string { return "fromServer" }

	for name, tc := range map[string]struct {
		conf offline.Config
		want string
	}{
		"server nonce": {
			conf: offline.Config{WorkerVersion: 1},
			want: `<script nonce="fromServer">`,
		},
		"config nonce": {
			conf: offline.Config{
				WorkerVersion: 1,
				CSPNonce:      func(*http.Request) string { return "ownNonce" },
			},
			want: `<script nonce="ownNonce">`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var cfg datapages.ServerConfig
			require.NoError(t, offline.WithServiceWorker("/offline/", tc.conf)(&cfg))
			require.NoError(t, datapages.WithCSPNonce(serverNonce)(&cfg))
			require.Len(t, cfg.Middleware, 1)

			rec := httptest.NewRecorder()
			cfg.Middleware[0](http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					_, _ = w.Write([]byte(page))
				},
			)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

			require.Contains(t, rec.Body.String(), tc.want)
		})
	}
}
