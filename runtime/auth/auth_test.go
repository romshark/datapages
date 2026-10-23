package auth_test

import (
	"cmp"
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/testlog"
	"github.com/romshark/datapages/modules/sessions"
	"github.com/romshark/datapages/modules/sessions/inmem"
	"github.com/romshark/datapages/runtime/auth"
)

// testServer logs to log, or to [slog.Default] when log is nil.
type testServer struct{ log *slog.Logger }

func (s testServer) Logger() *slog.Logger {
	if s.log == nil {
		return slog.Default()
	}
	return s.log
}

func newStore() *inmem.SessionManager[struct{}] {
	return inmem.New[struct{}](sessions.DefaultTokenGenerator{
		Length: sessions.DefaultTokenLen,
	})
}

func newManager(t *testing.T) (*auth.Manager[struct{}], *inmem.SessionManager[struct{}]) {
	t.Helper()
	store := newStore()
	return auth.NewManager(testServer{}, store, datapages.ServerConfig{}, nil), store
}

// newCountingManager returns a manager on a failingStore that
// counts into the returned metrics.
func newCountingManager(t *testing.T) (
	*auth.Manager[struct{}], *failingStore, *recordedMetrics,
) {
	t.Helper()
	store := &failingStore{SessionManager: newStore()}
	metrics := new(recordedMetrics)
	return auth.NewManager(
		testServer{}, store, datapages.ServerConfig{}, metrics,
	), store, metrics
}

var errStore = errors.New("store down")

// failingStore is the in-memory store with the operations
// a test sets failing with errStore.
type failingStore struct {
	*inmem.SessionManager[struct{}]
	failRead, failCreate, failClose bool
}

func (s *failingStore) ReadSessionFromCookie(cookieValue string) (
	sessions.Record[struct{}], string, bool, error,
) {
	if s.failRead {
		return sessions.Record[struct{}]{}, "", false, errStore
	}
	return s.SessionManager.ReadSessionFromCookie(cookieValue)
}

func (s *failingStore) CreateSession(
	ctx context.Context, rec sessions.Record[struct{}],
) (string, error) {
	if s.failCreate {
		return "", errStore
	}
	return s.SessionManager.CreateSession(ctx, rec)
}

func (s *failingStore) CloseSession(ctx context.Context, token string) error {
	if s.failClose {
		return errStore
	}
	return s.SessionManager.CloseSession(ctx, token)
}

// recordedMetrics lists the outcomes a manager counts, such as "read:valid".
type recordedMetrics []string

func (m *recordedMetrics) SessionRead(outcome string) {
	*m = append(*m, "read:"+outcome)
}

func (m *recordedMetrics) SessionCreated(outcome string) {
	*m = append(*m, "created:"+outcome)
}

func (m *recordedMetrics) SessionClosed(outcome string) {
	*m = append(*m, "closed:"+outcome)
}

// cleared reports whether the response clears the session cookie of m.
func cleared(t *testing.T, m *auth.Manager[struct{}], w *httptest.ResponseRecorder) bool {
	t.Helper()
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		return false
	}
	require.Len(t, cookies, 1)
	require.Equal(t, m.CookieName(), cookies[0].Name)
	require.Equal(t, -1, cookies[0].MaxAge, "the cookie is written, not cleared")
	return true
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
			m := auth.NewManager(testServer{}, newStore(),
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

// TestNewManagerDefaults tests what a zero configuration falls back to, and that
// a set field replaces it. Turning CSRF protection off is logged as a warning.
func TestNewManagerDefaults(t *testing.T) {
	t.Parallel()
	generator := sessions.DefaultTokenGenerator{Length: 64}
	for name, tt := range map[string]struct {
		conf          datapages.ServerConfig
		wantCookie    string
		wantGenerator sessions.TokenGenerator
		wantCSRF      bool
		wantLog       []string
	}{
		"zero": {
			wantCookie: datapages.DefaultSessionCookieName,
			wantGenerator: sessions.DefaultTokenGenerator{
				Length: sessions.DefaultTokenLen,
			},
			wantCSRF: true,
		},
		"set": {
			conf: datapages.ServerConfig{
				Sessions: datapages.SessionsConfig{
					Cookie:         datapages.AuthCookieConfig{Name: "sid"},
					TokenGenerator: generator,
				},
				CSRF: &datapages.CSRFConfig{Disabled: true},
			},
			wantCookie:    "sid",
			wantGenerator: generator,
			wantLog: []string{`level=WARN msg="CSRF protection is disabled" ` +
				`reason="CSRFConfig.Disabled is set" effect="state-changing ` +
				`actions are accepted on the session cookie alone"`},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var log testlog.Buffer
			m := auth.NewManager(testServer{log.Logger()}, newStore(), tt.conf, nil)
			require.Equal(t, tt.wantCookie, m.CookieName())
			require.Equal(t, tt.wantGenerator, m.SessionTokenGenerator())
			require.Equal(t, tt.wantCSRF, m.CSRFEnabled())
			log.RequireLines(t, tt.wantLog...)
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

// TestReadSession tests whether the handler runs and for whom, what becomes of the
// session cookie and which outcome is counted. A store failure keeps the cookie,
// and a cookie that names no live session is cleared.
func TestReadSession(t *testing.T) {
	live := sessions.Record[struct{}]{
		UserID: "alice", ExpiresAt: time.Now().Add(time.Hour),
	}
	expired := sessions.Record[struct{}]{
		UserID: "alice", ExpiresAt: time.Now().Add(-time.Hour),
	}
	for name, tt := range map[string]struct {
		method      string
		stored      *sessions.Record[struct{}] // Stored and its token sent.
		cookie      string                     // Sent when stored is nil.
		failRead    bool
		wantOK      bool
		wantUser    string
		wantCode    int
		wantCleared bool
		wantMetrics recordedMetrics
	}{
		"no cookie": {
			wantOK: true, wantCode: http.StatusOK,
			wantMetrics: recordedMetrics{"read:none"},
		},
		"store failure": {
			stored: &live, failRead: true,
			wantCode:    http.StatusServiceUnavailable,
			wantMetrics: recordedMetrics{"read:error"},
		},
		"unknown token": {
			cookie: "unknown",
			wantOK: true, wantCode: http.StatusOK, wantCleared: true,
			wantMetrics: recordedMetrics{"read:stale"},
		},
		"expired": {
			stored: &expired,
			wantOK: true, wantCode: http.StatusOK, wantCleared: true,
			wantMetrics: recordedMetrics{"read:expired"},
		},
		"live": {
			stored: &live,
			wantOK: true, wantUser: "alice", wantCode: http.StatusOK,
			wantMetrics: recordedMetrics{"read:valid"},
		},
		"live, action without CSRF token": {
			method: http.MethodPost, stored: &live,
			wantUser: "alice", wantCode: http.StatusForbidden,
			wantMetrics: recordedMetrics{"read:valid"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			m, store, metrics := newCountingManager(t)
			cookie := tt.cookie
			if tt.stored != nil {
				var err error
				cookie, err = store.CreateSession(t.Context(), *tt.stored)
				require.NoError(t, err)
			}
			store.failRead = tt.failRead

			r := httptest.NewRequest(cmp.Or(tt.method, http.MethodGet), "/", nil)
			if cookie != "" {
				r.AddCookie(&http.Cookie{Name: m.CookieName(), Value: cookie})
			}
			w := httptest.NewRecorder()
			sess, _, ok := m.ReadSession(w, r)
			require.Equal(t, tt.wantOK, ok, "the handler runs")
			require.Equal(t, tt.wantUser, sess.UserID())
			require.Equal(t, tt.wantCode, w.Code)
			require.Equal(t, tt.wantCleared, cleared(t, m, w), "the cookie is cleared")
			require.Equal(t, tt.wantMetrics, *metrics)
		})
	}
}

// TestReadSessionLogsAFailedDrop tests an expired session the store fails to
// delete. The visitor is signed out all the same, and the failure is logged.
func TestReadSessionLogsAFailedDrop(t *testing.T) {
	var log testlog.Buffer
	store := &failingStore{SessionManager: newStore(), failClose: true}
	m := auth.NewManager(testServer{log.Logger()}, store, datapages.ServerConfig{}, nil)
	token, err := store.CreateSession(t.Context(), sessions.Record[struct{}]{
		UserID: "alice", ExpiresAt: time.Now().Add(-time.Hour),
	})
	require.NoError(t, err)

	w, r := get(m, token)
	sess, _, ok := m.ReadSession(w, r)
	require.True(t, ok, "the request was answered instead of continuing")
	require.True(t, sess.IsGuest(), "an expired session authenticated")
	require.True(t, cleared(t, m, w), "the cookie is not cleared")
	log.RequireLines(t,
		`level=ERROR msg="closing an expired session" err="store down"`)
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

// TestCreateSessionClosesThePreviousSession tests signing in on a request that
// carries a session cookie. The previous token stops working, which keeps a
// token planted in the browser before the sign-in from riding on it.
func TestCreateSessionClosesThePreviousSession(t *testing.T) {
	m, store, metrics := newCountingManager(t)
	previous, err := store.CreateSession(t.Context(),
		sessions.Record[struct{}]{UserID: "mallory"})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.AddCookie(&http.Cookie{Name: m.CookieName(), Value: previous})
	sess, err := m.CreateSession(w, r,
		datapages.NewSession[struct{}]{UserID: "alice"})
	require.NoError(t, err)

	_, _, found, err := store.ReadSessionFromCookie(previous)
	require.NoError(t, err)
	require.False(t, found, "the previous session is still in the store")

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	require.Equal(t, sess.Token(), cookies[0].Value)
	rec, _, found, err := store.ReadSessionFromCookie(sess.Token())
	require.NoError(t, err)
	require.True(t, found, "the new session is not in the store")
	require.Equal(t, "alice", rec.UserID)
	require.Equal(t, recordedMetrics{"closed:success", "created:success"}, *metrics)
}

// TestCreateSessionFailure tests a sign-in that fails. The error comes back and
// no cookie is written. An invalid user ID fails before the previous session is closed,
// which keeps the visitor signed in as before. A store failure is counted.
func TestCreateSessionFailure(t *testing.T) {
	for name, tt := range map[string]struct {
		userID       string
		failClose    bool
		failCreate   bool
		wantErr      error
		wantPrevious bool // The session the request arrived with survives.
		wantMetrics  recordedMetrics
	}{
		"empty user ID": {wantErr: datapages.ErrUserIDEmpty, wantPrevious: true},
		"user ID too long": {
			userID:       strings.Repeat("a", datapages.MaxUserIDEncodedLen+1),
			wantErr:      datapages.ErrUserIDTooLong,
			wantPrevious: true,
		},
		"previous session not closed": {
			userID: "alice", failClose: true,
			wantErr: errStore, wantPrevious: true,
			wantMetrics: recordedMetrics{"closed:error"},
		},
		"not stored": {
			userID: "alice", failCreate: true,
			wantErr:     errStore,
			wantMetrics: recordedMetrics{"closed:success", "created:error"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			m, store, metrics := newCountingManager(t)
			previous, err := store.CreateSession(t.Context(),
				sessions.Record[struct{}]{UserID: "bob"})
			require.NoError(t, err)
			store.failClose, store.failCreate = tt.failClose, tt.failCreate

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/", nil)
			r.AddCookie(&http.Cookie{Name: m.CookieName(), Value: previous})
			sess, err := m.CreateSession(w, r,
				datapages.NewSession[struct{}]{UserID: tt.userID})
			require.ErrorIs(t, err, tt.wantErr)
			require.True(t, sess.IsGuest())
			require.Empty(t, w.Result().Cookies(), "a cookie is written")

			_, _, found, err := store.ReadSessionFromCookie(previous)
			require.NoError(t, err)
			require.Equal(t, tt.wantPrevious, found, "the previous session survives")
			require.Equal(t, tt.wantMetrics, *metrics)
		})
	}
}

// TestCloseSession tests signing out. The session is deleted and the cookie cleared.
// A store failure keeps the cookie, which still names a valid session.
func TestCloseSession(t *testing.T) {
	for name, tt := range map[string]struct {
		failClose   bool
		wantErr     error
		wantMetrics recordedMetrics
	}{
		"closed": {wantMetrics: recordedMetrics{"closed:success"}},
		"store failure": {
			failClose: true, wantErr: errStore,
			wantMetrics: recordedMetrics{"closed:error"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			m, store, metrics := newCountingManager(t)
			token, err := store.CreateSession(t.Context(),
				sessions.Record[struct{}]{UserID: "alice"})
			require.NoError(t, err)
			store.failClose = tt.failClose

			w, r := get(m, token)
			sess, err := m.CloseSession(w, r, token)
			require.ErrorIs(t, err, tt.wantErr)
			require.True(t, sess.IsGuest())

			_, _, found, err := store.ReadSessionFromCookie(token)
			require.NoError(t, err)
			require.Equal(t, tt.failClose, found, "the session is still in the store")
			require.Equal(t, !tt.failClose, cleared(t, m, w), "the cookie is cleared")
			require.Equal(t, tt.wantMetrics, *metrics)
		})
	}
}

var csrfTokenInScript = regexp.MustCompile(`"X-CSRF-Token",'([^']+)'`)

// TestWriteCSRFScriptFromSessionCookie tests that a document handler can write
// a CSRF script from the request cookie without reading the session store.
// [auth.Manager.CheckCSRFOnly] accepts the script's token with the same cookie.
// An absent cookie or disabled CSRF protection produces no script.
func TestWriteCSRFScriptFromSessionCookie(t *testing.T) {
	t.Parallel()
	for name, tt := range map[string]struct {
		cookie   string
		disabled bool
		script   bool
	}{
		"session cookie": {cookie: "tok", script: true},
		"no cookie":      {},
		"disabled":       {cookie: "tok", disabled: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := auth.NewManager(testServer{}, newStore(), datapages.ServerConfig{
				CSRF: &datapages.CSRFConfig{Disabled: tt.disabled},
			}, nil)

			page := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.cookie != "" {
				page.AddCookie(&http.Cookie{Name: m.CookieName(), Value: tt.cookie})
			}
			var b strings.Builder
			require.NoError(t, m.WriteCSRFScript(&b, m.SessionCookie(page), ""))
			if !tt.script {
				require.Empty(t, b.String())
				return
			}

			match := csrfTokenInScript.FindStringSubmatch(b.String())
			require.NotNil(t, match, "the script contains no token:\n%s", b.String())

			action := httptest.NewRequest(http.MethodPost, "/", nil)
			action.AddCookie(&http.Cookie{Name: m.CookieName(), Value: tt.cookie})
			action.Header.Set("X-CSRF-Token", match[1])
			require.True(t, m.CheckCSRFOnly(httptest.NewRecorder(), action),
				"the script token does not match the session cookie")
		})
	}
}

// TestCSRFScriptOrigin tests that the script of [auth.Manager.WriteCSRFScript]
// adds the token to the state-changing Datastar fetches to the page's origin only.
// testdata/csrf_script_test.mjs runs the script in Node.
func TestCSRFScriptOrigin(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	m := auth.NewManager(testServer{}, newStore(), datapages.ServerConfig{}, nil)
	var b strings.Builder
	require.NoError(t, m.WriteCSRFScript(&b, "tok", ""))
	script := filepath.Join(t.TempDir(), "csrf.html")
	require.NoError(t, os.WriteFile(script, []byte(b.String()), 0o644))

	out, err := exec.CommandContext(t.Context(),
		node, "testdata/csrf_script_test.mjs", script).CombinedOutput()
	require.NoError(t, err, "%s", out)
}

// TestCheckCSRFMethods tests which methods need the token of a signed-in visitor.
// The read methods pass without one. Every other method is answered with 403.
func TestCheckCSRFMethods(t *testing.T) {
	t.Parallel()
	for method, pass := range map[string]bool{
		http.MethodGet:     true,
		http.MethodHead:    true,
		http.MethodOptions: true,
		"QUERY":            true,
		http.MethodPost:    false,
		http.MethodPut:     false,
		http.MethodPatch:   false,
		http.MethodDelete:  false,
	} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			m, _ := newManager(t)
			w := httptest.NewRecorder()
			r := httptest.NewRequest(method, "/", nil)
			r.AddCookie(&http.Cookie{Name: m.CookieName(), Value: "tok"})
			require.Equal(t, pass, m.CheckCSRFOnly(w, r))
			if !pass {
				require.Equal(t, http.StatusForbidden, w.Code)
			}
		})
	}
}

// TestCheckCSRFToken tests the token on an action of a signed-in visitor.
// Only the one derived from their own session passes, any other is answered with 403.
func TestCheckCSRFToken(t *testing.T) {
	t.Parallel()
	m, _ := newManager(t)
	tokenOf := func(sessionToken string) string {
		var b strings.Builder
		_, err := m.WriteCSRFToken(&b, sessionToken)
		require.NoError(t, err)
		return b.String()
	}
	for name, tt := range map[string]struct {
		token    string
		wantCode int
	}{
		"own session":   {token: tokenOf("tok"), wantCode: http.StatusOK},
		"other session": {token: tokenOf("other"), wantCode: http.StatusForbidden},
		"malformed":     {token: "x", wantCode: http.StatusForbidden},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/", nil)
			r.AddCookie(&http.Cookie{Name: m.CookieName(), Value: "tok"})
			r.Header.Set("X-CSRF-Token", tt.token)
			require.Equal(t, tt.wantCode == http.StatusOK, m.CheckCSRFOnly(w, r))
			require.Equal(t, tt.wantCode, w.Code)
		})
	}
}

// TestWriteCSRFScriptNonce tests the nonce attribute of the script tag, without
// which a Content-Security-Policy that allows scripts by nonce blocks the script.
func TestWriteCSRFScriptNonce(t *testing.T) {
	t.Parallel()
	for name, tt := range map[string]struct{ nonce, wantTag string }{
		"nonce":    {nonce: "abc", wantTag: `<script type="module" nonce="abc">`},
		"no nonce": {wantTag: `<script type="module">`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m, _ := newManager(t)
			var b strings.Builder
			require.NoError(t, m.WriteCSRFScript(&b, "tok", tt.nonce))
			tag, _, _ := strings.Cut(strings.TrimSpace(b.String()), ">")
			require.Equal(t, tt.wantTag, tag+">")
		})
	}
}

// TestWriteCSRFScriptWriteError tests a writer that fails at each write of the
// script in turn. Its error comes back, and no write follows the one that failed.
func TestWriteCSRFScriptWriteError(t *testing.T) {
	t.Parallel()
	m, _ := newManager(t)
	all := &failingWriter{failAt: math.MaxInt}
	require.NoError(t, m.WriteCSRFScript(all, "tok", "abc"))
	require.NotZero(t, all.writes)
	for failAt := range all.writes {
		w := &failingWriter{failAt: failAt}
		err := m.WriteCSRFScript(w, "tok", "abc")
		require.ErrorIs(t, err, errWrite, "write %d fails", failAt)
		require.Equal(t, failAt+1, w.writes, "writes when write %d fails", failAt)
	}
}

var errWrite = errors.New("write failed")

// failingWriter fails write number failAt, counting from 0, and each one after it.
type failingWriter struct{ failAt, writes int }

func (w *failingWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes > w.failAt {
		return 0, errWrite
	}
	return len(p), nil
}

// TestWriteCSRFScriptWarnsOfAnEmptyToken tests a token writer that writes nothing
// for a signed-in visitor, which is logged.
func TestWriteCSRFScriptWarnsOfAnEmptyToken(t *testing.T) {
	var log testlog.Buffer
	m := auth.NewManager(testServer{log.Logger()}, newStore(), datapages.ServerConfig{
		CSRF: &datapages.CSRFConfig{Tokens: emptyTokens{}},
	}, nil)
	var b strings.Builder
	require.NoError(t, m.WriteCSRFScript(&b, "tok", ""))
	log.RequireLines(t, `level=WARN msg="wrote empty CSRF token"`)
}

// emptyTokens writes no CSRF token and accepts none.
type emptyTokens struct{}

func (emptyTokens) WriteToken(io.Writer, string) (int, error) { return 0, nil }

func (emptyTokens) ValidateToken(string, string) bool { return false }

// TestWriteCSRFTokenWritesNothing tests that a guest gets no CSRF token,
// and that nobody does while the protection is off.
func TestWriteCSRFTokenWritesNothing(t *testing.T) {
	t.Parallel()
	for name, tt := range map[string]struct {
		sessionToken string
		disabled     bool
	}{
		"guest":    {},
		"disabled": {sessionToken: "tok", disabled: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var log testlog.Buffer
			m := auth.NewManager(testServer{log.Logger()}, newStore(),
				datapages.ServerConfig{
					CSRF: &datapages.CSRFConfig{Disabled: tt.disabled},
				}, nil)
			var b strings.Builder
			n, err := m.WriteCSRFToken(&b, tt.sessionToken)
			require.NoError(t, err)
			require.Zero(t, n)
			require.Empty(t, b.String())
		})
	}
}
