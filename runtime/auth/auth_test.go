package auth_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/modules/sessions"
	"github.com/romshark/datapages/modules/sessions/inmem"
	"github.com/romshark/datapages/runtime/auth"
)

type testServer struct{}

func (testServer) Logger() *slog.Logger { return slog.Default() }

func newManager(t *testing.T) (
	*auth.Manager[struct{}], *inmem.SessionManager[struct{}],
) {
	t.Helper()
	store := inmem.New[struct{}](sessions.DefaultTokenGenerator{
		Length: sessions.DefaultTokenLen,
	})
	return auth.NewManager[struct{}](
		testServer{}, store, datapages.ServerConfig{}, nil,
	), store
}

// get is a request carrying the session cookie of token.
func get(m *auth.Manager[struct{}], token string) (
	*httptest.ResponseRecorder, *http.Request,
) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: m.CookieName(), Value: token})
	return httptest.NewRecorder(), r
}

// TestSecureCookie tests the Secure flag of the session cookie, which is set
// however this process is reached: TLS is as likely to end at a proxy in front of it.
func TestSecureCookie(t *testing.T) {
	for name, disable := range map[string]bool{
		"default":  false,
		"disabled": true,
	} {
		t.Run(name, func(t *testing.T) {
			store := inmem.New[struct{}](sessions.DefaultTokenGenerator{
				Length: sessions.DefaultTokenLen,
			})
			m := auth.NewManager(testServer{}, store,
				datapages.ServerConfig{
					Sessions: datapages.SessionsConfig{
						DisableSecureCookie: disable,
					},
				}, nil)

			w := httptest.NewRecorder()
			m.SetSessionCookie(w, "tok", time.Time{})

			cookies := w.Result().Cookies()
			require.Len(t, cookies, 1)
			require.Equal(t, !disable, cookies[0].Secure)
		})
	}
}

// TestReadSessionDropsAnExpiredSession tests what reading an expired session
// leaves behind. The client is signed out either way, and the record it named
// is of no use to anyone from that point on.
func TestReadSessionDropsAnExpiredSession(t *testing.T) {
	m, store := newManager(t)

	token, err := store.CreateSession(t.Context(), sessions.Record[struct{}]{
		UserID:    "alice",
		IssuedAt:  time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-time.Hour),
	})
	require.NoError(t, err)

	w, r := get(m, token)
	sess, _, ok := m.ReadSession(w, r)
	require.True(t, ok, "the request was answered instead of continuing")
	require.True(t, sess.IsGuest(), "an expired session authenticated")

	_, _, found, err := store.ReadSessionFromCookie(token)
	require.NoError(t, err)
	require.False(t, found, "the expired session is still in the store")
}

// TestReadSessionKeepsALiveSession tests the session the drop must not touch.
func TestReadSessionKeepsALiveSession(t *testing.T) {
	m, store := newManager(t)

	for name, expiresAt := range map[string]time.Time{
		"expires later": time.Now().Add(time.Hour),
		"never expires": {},
	} {
		t.Run(name, func(t *testing.T) {
			token, err := store.CreateSession(t.Context(),
				sessions.Record[struct{}]{UserID: "alice", ExpiresAt: expiresAt})
			require.NoError(t, err)

			w, r := get(m, token)
			sess, _, ok := m.ReadSession(w, r)
			require.True(t, ok)
			require.Equal(t, "alice", sess.UserID())

			_, _, found, err := store.ReadSessionFromCookie(token)
			require.NoError(t, err)
			require.True(t, found, "a live session was dropped")
		})
	}
}

// TestSessionCookieLifetime tests that the session cookie carries Max-Age and
// Expires from the session expiry, and neither when the session never expires.
func TestSessionCookieLifetime(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour)

	for name, td := range map[string]struct {
		value      string
		expiresAt  time.Time
		wantMaxAge int
	}{
		"browser session": {value: "tok", wantMaxAge: 0},
		"persistent":      {value: "tok", expiresAt: expiresAt, wantMaxAge: 3600},
		// An expiry in the past must not write a Max-Age below 1,
		// which deletes the cookie.
		"already expired": {
			value:      "tok",
			expiresAt:  time.Now().Add(-time.Hour),
			wantMaxAge: 1,
		},
		"cleared": {value: "", expiresAt: expiresAt, wantMaxAge: -1},
	} {
		t.Run(name, func(t *testing.T) {
			m, _ := newManager(t)
			w := httptest.NewRecorder()
			m.SetSessionCookie(w, td.value, td.expiresAt)

			cookies := w.Result().Cookies()
			require.Len(t, cookies, 1)
			c := cookies[0]
			require.InDelta(t, td.wantMaxAge, c.MaxAge, 1)
			switch {
			case td.value == "":
				require.Equal(t, time.Unix(0, 0).UTC(), c.Expires.UTC())
			case td.expiresAt.IsZero():
				require.True(t, c.Expires.IsZero(),
					"a browser-session cookie carries an expiry")
			default:
				require.WithinDuration(t, td.expiresAt, c.Expires, time.Second)
			}
		})
	}
}

// TestCreateSessionCookieExpiry tests that CreateSession puts the expiry of the
// new session into the cookie, not only into the store record.
func TestCreateSessionCookieExpiry(t *testing.T) {
	m, _ := newManager(t)
	expiresAt := time.Now().Add(30 * 24 * time.Hour)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	sess, err := m.CreateSession(w, r, datapages.NewSession[struct{}]{
		UserID:    "alice",
		ExpiresAt: expiresAt,
	})
	require.NoError(t, err)
	require.Equal(t, "alice", sess.UserID())

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	require.WithinDuration(t, expiresAt, cookies[0].Expires, time.Second)
	require.InDelta(t, int(30*24*time.Hour/time.Second), cookies[0].MaxAge, 1)
}
