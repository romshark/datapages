package httpserve_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/runtime/httpserve"
)

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

func TestDevNoCache(t *testing.T) {
	t.Parallel()

	h := httpserve.DevNoCache(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("body"))
		}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	require.Equal(t, "no-store, max-age=0", w.Header().Get("Cache-Control"))
	require.Equal(t, "no-cache", w.Header().Get("Pragma"))
	require.Equal(t, "0", w.Header().Get("Expires"))
	require.Equal(t, "body", w.Body.String())
}

func TestWriteReloadOnVisibility(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	httpserve.WriteReloadOnVisibility(&b)
	require.Equal(t,
		`data-on:visibilitychange__window="`+
			`if (!document.hidden) window.location.reload()" `,
		b.String())
}
