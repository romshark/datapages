package httpserve_test

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/runtime/httpserve"
)

// renderer renders s and fails with err when err is non-nil.
type renderer struct {
	s   string
	err error
}

func (c renderer) Render(_ context.Context, w io.Writer) error {
	if c.err != nil {
		return c.err
	}
	_, err := io.WriteString(w, c.s)
	return err
}

// csrfScript writes what a session manager would.
type csrfScript struct{ err error }

func (c csrfScript) WriteCSRFScript(w io.Writer, token, nonce string) error {
	if c.err != nil {
		return c.err
	}
	_, err := io.WriteString(w, "<csrf "+token+" "+nonce+">")
	return err
}

func builtCore(t *testing.T) *httpserve.Core {
	t.Helper()
	c := mustCore(t, datapages.ServerConfig{DatastarJS: "/ds.js"}, "")
	c.Build()
	return c
}

func writeHTML(
	t *testing.T, c *httpserve.Core, doc httpserve.HTMLDocument,
) (string, error) {
	t.Helper()
	w := httptest.NewRecorder()
	err := c.WriteHTML(w, httptest.NewRequest(http.MethodGet, "/", nil), doc)
	return w.Body.String(), err
}

// TestWriteHTML tests the order the document parts are written in: the head
// prologue before the Datastar script, the shared head before the page head,
// the CSRF script after both, and the body suffix inside a template element.
// The generated handlers rely on that order.
func TestWriteHTML(t *testing.T) {
	t.Parallel()

	c := builtCore(t)

	for name, tc := range map[string]struct {
		doc  httpserve.HTMLDocument
		want string
	}{
		"empty": {
			httpserve.HTMLDocument{},
			c.HTMLPrefix() + "</head><body></body></html>",
		},
		"prologue precedes datastar": {
			httpserve.HTMLDocument{
				WriteHeadPrologue: func(w io.Writer) error {
					_, err := io.WriteString(w, "<script>p</script>")
					return err
				},
			},
			c.HTMLHead() + "<script>p</script>" + c.HTMLDatastarScript() +
				"</head><body></body></html>",
		},
		"head and body": {
			httpserve.HTMLDocument{
				Head: renderer{s: "<title>t</title>"},
				Body: renderer{s: "<p>b</p>"},
			},
			c.HTMLPrefix() +
				"<title>t</title></head><body><p>b</p></body></html>",
		},
		"generic head goes first": {
			httpserve.HTMLDocument{
				HeadGeneric: renderer{s: "<meta g>"},
				Head:        renderer{s: "<meta p>"},
			},
			c.HTMLPrefix() + "<meta g><meta p></head><body></body></html>",
		},
		"csrf follows the head": {
			httpserve.HTMLDocument{
				CSRF:         csrfScript{},
				SessionToken: "tok",
				Head:         renderer{s: "<meta p>"},
			},
			c.HTMLPrefix() +
				"<meta p><csrf tok ></head><body></body></html>",
		},
		"body attributes and suffix": {
			httpserve.HTMLDocument{
				WriteBodyAttrs: func(w http.ResponseWriter) {
					_, _ = io.WriteString(w, ` class="x"`)
				},
				WriteBodySuffix: func(w http.ResponseWriter) {
					_, _ = io.WriteString(w, ` data-x`)
				},
			},
			c.HTMLPrefix() +
				`</head><body class="x"><template data-x></template></body></html>`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := writeHTML(t, c, tc.doc)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestWriteHTMLDocumentAttrs tests the attributes of the html element:
// sorted by name, an empty value writes an empty attribute and values are escaped.
func TestWriteHTMLDocumentAttrs(t *testing.T) {
	t.Parallel()

	c := builtCore(t)
	// The html start tag is the 1 part of HTMLHead that varies.
	rest := strings.TrimPrefix(c.HTMLHead(), "<!DOCTYPE html><html>")

	for name, tc := range map[string]struct {
		doc     datapages.DocumentAttrs
		wantTag string
	}{
		"nil":   {wantTag: "<html>"},
		"empty": {doc: datapages.DocumentAttrs{}, wantTag: "<html>"},
		"sorted": {
			doc: datapages.DocumentAttrs{
				"translate": "no", "lang": "ar", "dir": "rtl",
				"data-theme": "dark", "class": "dark",
			},
			wantTag: `<html class="dark" data-theme="dark" dir="rtl" lang="ar" translate="no">`,
		},
		"empty value": {
			doc:     datapages.DocumentAttrs{"itemscope": ""},
			wantTag: `<html itemscope="">`,
		},
		"escaped": {
			doc:     datapages.DocumentAttrs{"class": `a"><script>x`, "title": `'&`},
			wantTag: `<html class="a&#34;&gt;&lt;script&gt;x" title="&#39;&amp;">`,
		},
		"datastar attribute name": {
			doc: datapages.DocumentAttrs{
				"data-class:dark": "$dark", "data-on:keydown__window.debounce_1": "x",
			},
			wantTag: `<html data-class:dark="$dark" data-on:keydown__window.debounce_1="x">`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := writeHTML(t, c, httpserve.HTMLDocument{DocumentAttrs: tc.doc})
			require.NoError(t, err)
			require.Equal(t, "<!DOCTYPE html>"+tc.wantTag+rest+
				c.HTMLDatastarScript()+"</head><body></body></html>", got)
		})
	}
}

// TestWriteHTMLDocumentAttrsInvalid tests a name that is invalid or that the CSP
// nonce writes. Nothing of the document is written, and the response is a 500 instead,
// whatever status the document was meant to have.
func TestWriteHTMLDocumentAttrsInvalid(t *testing.T) {
	t.Parallel()

	c := builtCore(t)

	for name, tc := range map[string]struct {
		attr    string
		wantErr error
	}{
		"empty":          {"", httpserve.ErrDocumentAttrsInvalidName},
		"uppercase":      {"data-Theme", httpserve.ErrDocumentAttrsInvalidName},
		"leading digit":  {"1x", httpserve.ErrDocumentAttrsInvalidName},
		"leading dash":   {"-x", httpserve.ErrDocumentAttrsInvalidName},
		"space":          {"a b", httpserve.ErrDocumentAttrsInvalidName},
		"quote":          {`a"b`, httpserve.ErrDocumentAttrsInvalidName},
		"closes the tag": {"a><script", httpserve.ErrDocumentAttrsInvalidName},
		"equals":         {"a=b", httpserve.ErrDocumentAttrsInvalidName},
		"non-ASCII":      {"dätä", httpserve.ErrDocumentAttrsInvalidName},
		"data-nonce":     {"data-nonce", httpserve.ErrDocumentAttrsReserved},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			err := c.WriteHTML(w, httptest.NewRequest(http.MethodGet, "/", nil),
				httpserve.HTMLDocument{
					Status:        http.StatusNotFound,
					DocumentAttrs: datapages.DocumentAttrs{"lang": "en", tc.attr: "x"},
					Body:          renderer{s: "body"},
				})
			require.ErrorIs(t, err, tc.wantErr)
			if tc.wantErr == httpserve.ErrDocumentAttrsInvalidName {
				require.Contains(t, err.Error(), strconv.Quote(tc.attr))
			}
			require.Equal(t, http.StatusInternalServerError, w.Code)
			require.Equal(t, "Internal Server Error\n", w.Body.String())
		})
	}
}

// TestWriteHTMLDocumentAttrsCSPNonce tests that the nonce follows the attributes
// of the document on the html element.
func TestWriteHTMLDocumentAttrsCSPNonce(t *testing.T) {
	t.Parallel()

	c := mustCore(t, datapages.ServerConfig{
		DatastarJS: "/ds.js",
		CSPNonce:   func(*http.Request) string { return "n" },
	}, "")
	c.Build()

	got, err := writeHTML(t, c, httpserve.HTMLDocument{
		DocumentAttrs: datapages.DocumentAttrs{"lang": "en"},
	})
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(got,
		`<!DOCTYPE html><html lang="en" data-nonce="n"><head>`), got)
}

// TestWriteHTMLError tests a part that fails to render. Whichever part it is,
// the error reaches the caller instead of being swallowed into a half-written document.
func TestWriteHTMLError(t *testing.T) {
	t.Parallel()

	c := builtCore(t)
	errRender := errors.New("render failed")

	for name, doc := range map[string]httpserve.HTMLDocument{
		"generic head": {HeadGeneric: renderer{err: errRender}},
		"head":         {Head: renderer{err: errRender}},
		"body":         {Body: renderer{err: errRender}},
		"csrf":         {CSRF: csrfScript{err: errRender}},
		"prologue":     {WriteHeadPrologue: func(io.Writer) error { return errRender }},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := writeHTML(t, c, doc)
			require.ErrorIs(t, err, errRender)
		})
	}
}

// errGone is what every write fails with once the client is gone.
var errGone = errors.New("client gone")

// hangUpWriter is the response of a client that hangs up after n writes.
// Every later Write fails with errGone.
type hangUpWriter struct {
	http.ResponseWriter
	n, writes int
}

func (w *hangUpWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes > w.n {
		return 0, errGone
	}
	return w.ResponseWriter.Write(p)
}

// TestWriteHTMLClientGone tests a client that hangs up partway through
// the document. Whichever write it hangs up before, WriteHTML returns
// the write error instead of nil. The prologue and the body suffix write
// nothing. They are set for the writes WriteHTML makes around them.
func TestWriteHTMLClientGone(t *testing.T) {
	t.Parallel()

	c := builtCore(t)
	for name, doc := range map[string]httpserve.HTMLDocument{
		"prologue": {
			WriteHeadPrologue: func(io.Writer) error { return nil },
			WriteBodySuffix:   func(http.ResponseWriter) {},
		},
		"no prologue": {WriteBodySuffix: func(http.ResponseWriter) {}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			complete := &hangUpWriter{
				ResponseWriter: httptest.NewRecorder(),
				n:              math.MaxInt,
			}
			require.NoError(t, c.WriteHTML(complete, r, doc))

			for n := range complete.writes {
				w := &hangUpWriter{ResponseWriter: httptest.NewRecorder(), n: n}
				require.ErrorIs(t, c.WriteHTML(w, r, doc), errGone,
					"hung up after %d of %d writes", n, complete.writes)
			}
		})
	}
}

// TestWriteHTMLCacheControl tests the cache header and the status of a document.
// A document with a session token is private: a shared cache would serve the CSRF
// token in it to the next visitor. A Cache-Control the response already has stays,
// such as the no-store of a stateful page. The recorder snapshots the header when
// the status is written, which it has to come before.
func TestWriteHTMLCacheControl(t *testing.T) {
	t.Parallel()

	c := builtCore(t)
	for name, tc := range map[string]struct {
		token, before, want string
	}{
		"session":          {token: "tok", want: "private"},
		"no session":       {},
		"already no-store": {token: "tok", before: "no-store", want: "no-store"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			if tc.before != "" {
				w.Header().Set("Cache-Control", tc.before)
			}
			err := c.WriteHTML(w, httptest.NewRequest(http.MethodGet, "/", nil),
				httpserve.HTMLDocument{
					Status:       http.StatusNotFound,
					CSRF:         csrfScript{},
					SessionToken: tc.token,
				})
			require.NoError(t, err)
			res := w.Result()
			require.Equal(t, http.StatusNotFound, res.StatusCode)
			require.Equal(t, tc.want, res.Header.Get("Cache-Control"))
		})
	}
}

// TestCheckDatastarRequest tests the guard on the action endpoints. A request without
// the Datastar-Request header is answered 406 and the handler is skipped:
// those endpoints only ever produce SSE.
func TestCheckDatastarRequest(t *testing.T) {
	t.Parallel()

	c := builtCore(t)

	t.Run("datastar", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Datastar-Request", "true")
		require.True(t, c.CheckDatastarRequest(w, r))
		require.Empty(t, w.Body.String())
	})

	t.Run("plain", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		require.False(t, c.CheckDatastarRequest(w, r))
		require.Equal(t, http.StatusNotAcceptable, w.Code)
	})
}

// TestCheckSameOrigin tests the guard on an action that carries no Datastar check:
// a plain HTML form reaches it, and so does a form on another site.
// A client that reports no origin at all is allowed, which is what
// net/http.CrossOriginProtection does and what keeps non-browser clients working.
func TestCheckSameOrigin(t *testing.T) {
	t.Parallel()

	c := builtCore(t)

	for name, tc := range map[string]struct {
		method  string
		headers map[string]string
		wantOK  bool
	}{
		"no origin headers": {method: http.MethodPost, wantOK: true},
		"same origin": {
			method:  http.MethodPost,
			headers: map[string]string{"Sec-Fetch-Site": "same-origin"},
			wantOK:  true,
		},
		"cross site": {
			method:  http.MethodPost,
			headers: map[string]string{"Sec-Fetch-Site": "cross-site"},
			wantOK:  false,
		},
		"same site": {
			method:  http.MethodPost,
			headers: map[string]string{"Sec-Fetch-Site": "same-site"},
			wantOK:  false,
		},
		"foreign origin": {
			method:  http.MethodPost,
			headers: map[string]string{"Origin": "https://evil.example"},
			wantOK:  false,
		},
		"own origin": {
			method:  http.MethodPost,
			headers: map[string]string{"Origin": "http://example.com"},
			wantOK:  true,
		},
		"cross site GET": {
			method:  http.MethodGet,
			headers: map[string]string{"Sec-Fetch-Site": "cross-site"},
			wantOK:  true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			r := httptest.NewRequest(tc.method, "http://example.com/act/", nil)
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
			require.Equal(t, tc.wantOK, c.CheckSameOrigin(w, r))
			if tc.wantOK {
				require.Empty(t, w.Body.String())
				return
			}
			require.Equal(t, http.StatusForbidden, w.Code)
		})
	}
}

// TestHTTPErrBad tests what a 400 tells the client. The body carries the
// operation only, never the underlying error, which goes to the log.
func TestHTTPErrBad(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	builtCore(t).HTTPErrBad(w, "reading signals", errors.New("eof"))
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Equal(t, "reading signals", strings.TrimSpace(w.Body.String()))
}

// TestWriteHTMLCSPNonce tests that the configured nonce reaches the html element,
// the Datastar script tag, the CSRF script and ScriptTagOpen.
// Datastar reads it off the html element to compile its attribute expressions
// without script-src 'unsafe-eval'.
func TestWriteHTMLCSPNonce(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		nonce     func(*http.Request) string
		wantHas   []string
		wantLacks []string
		wantTag   string
	}{
		"unset": {
			nonce:     nil,
			wantHas:   []string{"<html>", `<script type="module" src="/ds.js">`},
			wantLacks: []string{"data-nonce", "nonce="},
			wantTag:   "<script>",
		},
		"set": {
			nonce: func(*http.Request) string { return "r4nd0m+val/ue=" },
			wantHas: []string{
				`<html data-nonce="r4nd0m+val/ue=">`,
				`<script type="module" nonce="r4nd0m+val/ue=" src="/ds.js">`,
				"<csrf tok r4nd0m+val/ue=>",
			},
			wantTag: `<script nonce="r4nd0m+val/ue=">`,
		},
		"empty writes no nonce": {
			nonce:     func(*http.Request) string { return "" },
			wantHas:   []string{"<html>"},
			wantLacks: []string{"data-nonce", "nonce="},
			wantTag:   "<script>",
		},
		"escaped": {
			nonce:   func(*http.Request) string { return `a"><script>x` },
			wantHas: []string{`<html data-nonce="a&#34;&gt;&lt;script&gt;x">`},
			wantTag: `<script nonce="a&#34;&gt;&lt;script&gt;x">`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := mustCore(t, datapages.ServerConfig{
				DatastarJS: "/ds.js", CSPNonce: tc.nonce,
			}, "")
			c.Build()

			body, err := writeHTML(t, c, httpserve.HTMLDocument{
				CSRF: csrfScript{}, SessionToken: "tok",
			})
			require.NoError(t, err)
			for _, want := range tc.wantHas {
				require.Contains(t, body, want)
			}
			for _, lacks := range tc.wantLacks {
				require.NotContains(t, body, lacks)
			}
			require.Equal(t, tc.wantTag,
				c.ScriptTagOpen(httptest.NewRequest(http.MethodGet, "/", nil)))
		})
	}
}
