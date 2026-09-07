package httpserve_test

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/runtime/httpserve"
)

// TestIsDatastarRequest tests the header match, which is exact:
// Datastar sends the lowercase "true" and nothing else counts.
func TestIsDatastarRequest(t *testing.T) {
	t.Parallel()

	for header, want := range map[string]bool{
		"true":  true,
		"TRUE":  false,
		"false": false,
		"":      false,
	} {
		t.Run(header, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if header != "" {
				r.Header.Set("Datastar-Request", header)
			}
			require.Equal(t, want, httpserve.IsDatastarRequest(r))
		})
	}
}

// TestRedirect tests what a redirect turns into. A plain request gets a Location
// header and a 3xx, with a non-redirect status corrected to 302. A Datastar
// request instead gets JavaScript that navigates, since the browser would follow
// a 3xx inside the fetch and hand the page to the SSE reader.
func TestRedirect(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		redirect   datapages.Redirect
		datastar   bool
		wantExit   bool
		wantStatus int
		wantBody   string
		wantHeader string
	}{
		"no url": {
			redirect: datapages.Redirect{}, wantExit: false,
			wantStatus: http.StatusOK,
		},
		"default status": {
			redirect: datapages.Redirect{URL: "/next/"},
			wantExit: true, wantStatus: http.StatusFound,
		},
		"kept status": {
			redirect:   datapages.Redirect{URL: "/next/", Status: http.StatusSeeOther},
			wantExit:   true,
			wantStatus: http.StatusSeeOther,
		},
		"status that is no redirect": {
			redirect:   datapages.Redirect{URL: "/next/", Status: http.StatusTeapot},
			wantExit:   true,
			wantStatus: http.StatusFound,
		},
		"datastar navigates client side": {
			redirect: datapages.Redirect{URL: "/next/", Status: http.StatusSeeOther},
			datastar: true, wantExit: true, wantStatus: http.StatusOK,
			wantBody:   `window.location = "/next/";`,
			wantHeader: "text/javascript; charset=utf-8",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(http.MethodPost, "/", nil)
			if tc.datastar {
				r.Header.Set("Datastar-Request", "true")
			}
			w := httptest.NewRecorder()

			require.Equal(t, tc.wantExit, httpserve.Redirect(w, r, tc.redirect))
			require.Equal(t, tc.wantStatus, w.Code)
			if tc.wantBody != "" {
				require.Contains(t, w.Body.String(), tc.wantBody)
			}
			if tc.wantHeader != "" {
				require.Equal(t, tc.wantHeader, w.Header().Get("Content-Type"))
			}
		})
	}
}

// TestDevNoCache tests the headers dev mode adds so an edited page is reloaded rather
// than served from the browser cache, and that they do not disturb the response body.
func TestDevNoCache(t *testing.T) {
	t.Parallel()

	h := httpserve.DevNoCache(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("body"))
		},
	))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	require.Equal(t, "no-store, max-age=0", w.Header().Get("Cache-Control"))
	require.Equal(t, "no-cache", w.Header().Get("Pragma"))
	require.Equal(t, "0", w.Header().Get("Expires"))
	require.Equal(t, "body", w.Body.String())
}

// TestWriteReloadOnVisibility tests the dev-mode attribute that reloads a tab once it
// becomes visible again, which is how a tab left in the background picks up a restart.
func TestWriteReloadOnVisibility(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	httpserve.WriteReloadOnVisibility(&b)
	require.Equal(t,
		`data-on:visibilitychange__window="`+
			`if (!document.hidden) window.location.reload()" `,
		b.String())
}

// TestWriteErrStatus tests the mapping from the error sentinels to status codes,
// including a wrapped sentinel. Anything else, nil included, is a 500.
func TestWriteErrStatus(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		err  error
		want int
	}{
		"nil":       {nil, http.StatusInternalServerError},
		"unknown":   {errors.New("boom"), http.StatusInternalServerError},
		"bad":       {datapages.ErrBadRequest, http.StatusBadRequest},
		"forbidden": {datapages.ErrForbidden, http.StatusForbidden},
		"notfound":  {datapages.ErrNotFound, http.StatusNotFound},
		"conflict":  {datapages.ErrConflict, http.StatusConflict},
		"wrapped": {
			fmt.Errorf("reading user: %w", datapages.ErrNotFound),
			http.StatusNotFound,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			httpserve.WriteErrStatus(rec, tc.err)
			require.Equal(t, tc.want, rec.Code)
			require.Equal(t,
				http.StatusText(tc.want), strings.TrimSpace(rec.Body.String()))
		})
	}
}

//go:embed testdata/static/hello.txt
var testAssets embed.FS

// TestAssetsFileSystem tests which of the assets options wins: an explicit file
// system beats an embedded one, an embedded one is opened at the subdirectory the
// generated code names, and no option at all serves nothing.
func TestAssetsFileSystem(t *testing.T) {
	t.Parallel()

	t.Run("explicit fs wins", func(t *testing.T) {
		t.Parallel()
		want := http.Dir("/somewhere")
		got, err := httpserve.AssetsFileSystem(datapages.ServerConfig{
			AssetsFS:    want,
			AssetsEmbed: &testAssets,
		}, "app/static", "static")
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("no option", func(t *testing.T) {
		t.Parallel()
		got, err := httpserve.AssetsFileSystem(
			datapages.ServerConfig{}, "app/static", "static",
		)
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("app declares none", func(t *testing.T) {
		t.Parallel()
		_, err := httpserve.AssetsFileSystem(
			datapages.ServerConfig{AssetsEmbed: &testAssets}, "", "",
		)
		require.ErrorContains(t, err, "the app package declares no assets")
	})

	t.Run("embed subdirectory", func(t *testing.T) {
		t.Parallel()
		fsys, err := httpserve.AssetsFileSystem(
			datapages.ServerConfig{AssetsEmbed: &testAssets},
			"testdata/static", "testdata/static",
		)
		require.NoError(t, err)
		f, err := fsys.Open("hello.txt")
		require.NoError(t, err)
		defer func() { _ = f.Close() }()
		b, err := io.ReadAll(f)
		require.NoError(t, err)
		require.Equal(t, "hello", strings.TrimSpace(string(b)))
	})
}
