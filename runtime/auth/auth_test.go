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
			m.SetSessionCookie(w, "tok")

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
