package guard_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/example/desktop-multiwindow/guard"
)

const token = "TOKEN"

var listener = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5000}

// TestHandler tests which requests reach the next handler,
// and that the token in the URL becomes a cookie.
func TestHandler(t *testing.T) {
	for name, tc := range map[string]struct {
		host   string
		target string
		cookie string

		status   int
		location string
		setsTok  bool
	}{
		"cookie": {
			host: "127.0.0.1:5000", target: "/", cookie: token,
			status: http.StatusOK,
		},
		"no cookie": {
			host: "127.0.0.1:5000", target: "/",
			status: http.StatusForbidden,
		},
		"wrong cookie": {
			host: "127.0.0.1:5000", target: "/", cookie: "GUESS",
			status: http.StatusForbidden,
		},
		"other host": {
			host: "rebound.example:5000", target: "/", cookie: token,
			status: http.StatusForbidden,
		},
		"other port": {
			host: "127.0.0.1:5001", target: "/", cookie: token,
			status: http.StatusForbidden,
		},
		"token in url": {
			host: "127.0.0.1:5000", target: "/?launch-token=" + token + "&x=1",
			status: http.StatusSeeOther, location: "/?x=1", setsTok: true,
		},
		"wrong token in url": {
			host: "127.0.0.1:5000", target: "/?launch-token=GUESS",
			status: http.StatusForbidden,
		},
	} {
		t.Run(name, func(t *testing.T) {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			r := httptest.NewRequestWithContext(
				context.WithValue(t.Context(), http.LocalAddrContextKey, listener),
				http.MethodGet, tc.target, nil,
			)
			r.Host = tc.host
			if tc.cookie != "" {
				r.AddCookie(&http.Cookie{Name: "launch-token", Value: tc.cookie})
			}
			w := httptest.NewRecorder()
			guard.Handler(token, next).ServeHTTP(w, r)

			require.Equal(t, tc.status, w.Code)
			require.Equal(t, tc.location, w.Header().Get("Location"))
			var set *http.Cookie
			for _, c := range w.Result().Cookies() {
				if c.Name == "launch-token" {
					set = c
				}
			}
			if !tc.setsTok {
				require.Nil(t, set)
				return
			}
			require.NotNil(t, set)
			require.Equal(t, token, set.Value)
			require.True(t, set.HttpOnly)
			require.Equal(t, http.SameSiteStrictMode, set.SameSite)
		})
	}
}

// TestURL tests that the URL of a window carries the token to the listener.
func TestURL(t *testing.T) {
	require.Equal(t,
		"http://127.0.0.1:5000/?launch-token=A%2BB",
		guard.URL("127.0.0.1:5000", "A+B"))
}
