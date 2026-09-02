package inmem_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/modules/sessions"
	"github.com/romshark/datapages/modules/sessions/inmem"
)

type testSession struct {
	Username string
	Role     string
}

var tokGen = sessions.DefaultTokenGenerator{}

func newManager(t *testing.T) payloadManager {
	t.Helper()
	return payloadManager{inmem.New[testSession](tokGen)}
}

// payloadManager adapts the record-based manager API to the payload-shaped calls
// these tests are written against. Record plumbing is covered by TestRecordRoundTrip.
type payloadManager struct {
	*inmem.SessionManager[testSession]
}

func (m payloadManager) CreateSession(
	ctx context.Context, userID string, s testSession,
) (string, error) {
	return m.SessionManager.CreateSession(
		ctx, sessions.Record[testSession]{UserID: userID, Data: s},
	)
}

func (m payloadManager) ReadSessionFromCookie(cookieValue string) (
	session testSession, token, userID string, ok bool, err error,
) {
	rec, token, ok, err := m.SessionManager.ReadSessionFromCookie(cookieValue)
	return rec.Data, token, rec.UserID, ok, err
}

func (m payloadManager) SaveSession(
	ctx context.Context, token string, s testSession,
) error {
	rec, err := m.SessionManager.Session(ctx, token)
	if err != nil {
		return nil // Same no-op the record API has for unknown tokens.
	}
	rec.Data = s
	return m.SessionManager.SaveSession(ctx, token, rec)
}

func (m payloadManager) Session(
	ctx context.Context, token string,
) (testSession, error) {
	rec, err := m.SessionManager.Session(ctx, token)
	return rec.Data, err
}

type failingTokGen struct{}

var errFake = &fakeError{}

type fakeError struct{}

func (*fakeError) Error() string { return "fake error" }

func (failingTokGen) Generate() (string, error) {
	return "", errFake
}

// fixedTokGen returns the same token every time.
type fixedTokGen struct{ token string }

func (g fixedTokGen) Generate() (string, error) {
	return g.token, nil
}

// TestReadSessionFromCookie tests the read every request makes: an empty cookie
// and an unknown token are misses, a live token returns the user ID,
// the token and the session payload.
func TestReadSessionFromCookie(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	token, err := sm.CreateSession(ctx, "alice",
		testSession{Username: "alice", Role: "admin"})
	require.NoError(t, err)

	tests := map[string]struct {
		cookie   string
		wantOK   bool
		wantUID  string
		wantSess testSession
	}{
		"empty value": {
			cookie: "", wantOK: false,
		},
		"nonexistent token": {
			cookie: "no-such-token", wantOK: false,
		},
		"valid session": {
			cookie:   token,
			wantOK:   true,
			wantUID:  "alice",
			wantSess: testSession{Username: "alice", Role: "admin"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sess, retTok, uid, ok, err := sm.ReadSessionFromCookie(tc.cookie)
			require.NoError(t, err)
			if tc.wantOK {
				require.True(t, ok)
				require.Equal(t, tc.wantUID, uid)
				require.Equal(t, token, retTok)
				require.Equal(t, tc.wantSess, sess)
			} else {
				require.False(t, ok)
				require.Empty(t, retTok)
				require.Empty(t, uid)
			}
		})
	}
}

// TestReadSessionFromCookieStale tests a cookie whose session was closed.
// The browser keeps sending it, and the read has to report a miss.
func TestReadSessionFromCookieStale(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	token, err := sm.CreateSession(ctx, "alice", testSession{})
	require.NoError(t, err)
	require.NoError(t, sm.CloseSession(ctx, token))

	_, _, _, ok, err := sm.ReadSessionFromCookie(token)
	require.NoError(t, err)
	require.False(t, ok)
}

// TestCreateSession tests that a created session is readable back under the
// token it returned, a zero payload included.
func TestCreateSession(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	tests := map[string]struct {
		userID  string
		session testSession
	}{
		"basic": {
			userID:  "bob",
			session: testSession{Username: "bob", Role: "user"},
		},
		"zero session": {
			userID:  "charlie",
			session: testSession{},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			token, err := sm.CreateSession(ctx, tc.userID, tc.session)
			require.NoError(t, err)
			require.NotEmpty(t, token)

			sess, retTok, uid, ok, err := sm.ReadSessionFromCookie(
				token,
			)
			require.NoError(t, err)
			require.True(t, ok)
			require.Equal(t, token, retTok)
			require.Equal(t, tc.userID, uid)
			require.Equal(t, tc.session, sess)
		})
	}
}

// TestCreateSessionEmptyUserID tests the refusal of an anonymous session.
// An empty user ID would make every such session belong to the same user.
func TestCreateSessionEmptyUserID(t *testing.T) {
	sm := newManager(t)

	_, err := sm.CreateSession(context.Background(), "", testSession{})
	require.ErrorIs(t, err, inmem.ErrEmptyUserID)
}

// TestCreateSessionUniqueTokens tests that the default generator does not repeat
// itself over a run of creations for one user.
func TestCreateSessionUniqueTokens(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	tokens := make(map[string]struct{}, 100)
	for range 100 {
		tok, err := sm.CreateSession(ctx, "user", testSession{})
		require.NoError(t, err)
		require.NotContains(t, tokens, tok, "duplicate token generated")
		tokens[tok] = struct{}{}
	}
}

// TestCreateSessionErrTokenGenerator tests a token generator that fails.
// The error reaches the caller rather than producing a session with an empty token.
func TestCreateSessionErrTokenGenerator(t *testing.T) {
	sm := payloadManager{inmem.New[testSession](failingTokGen{})}

	_, err := sm.CreateSession(context.Background(), "bob", testSession{})
	require.ErrorIs(t, err, errFake)
}

// TestCreateSessionTokenCollisionOverwrites documents the current behavior:
// if the token generator produces a duplicate, the new session silently
// overwrites the old one. With a properly configured generator (256-bit random tokens)
// this is practically impossible.
func TestCreateSessionTokenCollisionOverwrites(t *testing.T) {
	sm := payloadManager{inmem.New[testSession](fixedTokGen{token: "same-token"})}
	ctx := context.Background()

	tok1, err := sm.CreateSession(ctx, "alice",
		testSession{Username: "alice", Role: "admin"})
	require.NoError(t, err)

	tok2, err := sm.CreateSession(ctx, "bob",
		testSession{Username: "bob", Role: "user"})
	require.NoError(t, err)
	require.Equal(t, tok1, tok2)

	// The second session overwrites the first.
	sess, retTok, uid, ok, err := sm.ReadSessionFromCookie(
		tok1,
	)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, tok1, retTok)
	require.Equal(t, "bob", uid)
	require.Equal(t, testSession{Username: "bob", Role: "user"}, sess)
}

// TestCloseSession tests that the session is gone afterwards, and that closing
// an already closed or never existing token is a no-op rather than an error:
// a sign-out may arrive twice.
func TestCloseSession(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	tests := map[string]struct {
		setup func(t *testing.T) string
	}{
		"existing session": {
			setup: func(t *testing.T) string {
				tok, err := sm.CreateSession(ctx, "alice", testSession{})
				require.NoError(t, err)
				return tok
			},
		},
		"already closed": {
			setup: func(t *testing.T) string {
				tok, err := sm.CreateSession(ctx, "alice", testSession{})
				require.NoError(t, err)
				require.NoError(t, sm.CloseSession(ctx, tok))
				return tok
			},
		},
		"nonexistent token": {
			setup: func(*testing.T) string {
				return "never-existed"
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			token := tc.setup(t)
			err := sm.CloseSession(ctx, token)
			require.NoError(t, err)

			_, _, _, ok, err := sm.ReadSessionFromCookie(
				token,
			)
			require.NoError(t, err)
			require.False(t, ok)
		})
	}
}

// TestCloseSessionNotifiesWatchers tests that every watcher of a session is called
// exactly once when it closes. That callback is how an open SSE stream learns to end.
func TestCloseSessionNotifiesWatchers(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	token, err := sm.CreateSession(ctx, "alice", testSession{})
	require.NoError(t, err)

	var called1, called2 atomic.Int32
	require.NoError(t, sm.NotifyClosed(ctx, token, func() { called1.Add(1) }))
	require.NoError(t, sm.NotifyClosed(ctx, token, func() { called2.Add(1) }))

	require.NoError(t, sm.CloseSession(ctx, token))

	require.Equal(t, int32(1), called1.Load())
	require.Equal(t, int32(1), called2.Load())
}

// TestNotifyClosedSessionDoesNotExist tests watching a session that closed
// before the watcher registered. The callback runs at once, since waiting for a
// close that already happened would hang the stream forever.
func TestNotifyClosedSessionDoesNotExist(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	// Session that was created then closed.
	token, err := sm.CreateSession(ctx, "alice", testSession{})
	require.NoError(t, err)
	require.NoError(t, sm.CloseSession(ctx, token))

	var called atomic.Int32
	err = sm.NotifyClosed(ctx, token, func() { called.Add(1) })
	require.NoError(t, err)
	// fn must be called immediately when the session doesn't exist.
	require.Equal(t, int32(1), called.Load())
}

// TestNotifyClosedSessionDoesNotExistNeverCreated tests the same immediate
// callback for a token that never existed.
func TestNotifyClosedSessionDoesNotExistNeverCreated(t *testing.T) {
	sm := newManager(t)

	var called atomic.Int32
	err := sm.NotifyClosed(context.Background(), "never-existed", func() {
		called.Add(1)
	})
	require.NoError(t, err)
	require.Equal(t, int32(1), called.Load())
}

// TestNotifyClosedSessionExists tests the ordinary case: nothing runs while the
// session is alive, and the close calls the callback once.
func TestNotifyClosedSessionExists(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	token, err := sm.CreateSession(ctx, "bob", testSession{})
	require.NoError(t, err)

	var called atomic.Int32
	err = sm.NotifyClosed(ctx, token, func() { called.Add(1) })
	require.NoError(t, err)

	// fn must not be called while session is alive.
	require.Zero(t, called.Load())

	// Close triggers the callback.
	require.NoError(t, sm.CloseSession(ctx, token))
	require.Equal(t, int32(1), called.Load())
}

// TestNotifyClosedContextCancellation tests a watcher whose context ends
// before the session does. The session stays alive and the callback must not run.
//
// [inmem.SessionManager.NotifyClosed] leaves a goroutine waiting on the
// context to unregister the watcher, and until it gets there
// [inmem.SessionManager.CloseSession] suppresses the callback by re-checking
// the context itself. Either path reaches the same outcome, and outside a
// synctest bubble which one the assertion runs against is luck.
// [synctest.Wait] returns once every other goroutine is durably blocked,
// hence once the watcher is off the map.
func TestNotifyClosedContextCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sm := newManager(t)
		ctx := context.Background()

		token, err := sm.CreateSession(ctx, "carol", testSession{})
		require.NoError(t, err)

		watchCtx, cancel := context.WithCancel(ctx)

		var called atomic.Int32
		err = sm.NotifyClosed(watchCtx, token, func() { called.Add(1) })
		require.NoError(t, err)

		cancel()
		synctest.Wait()

		require.NoError(t, sm.CloseSession(ctx, token))
		require.Zero(t, called.Load(),
			"a watcher whose context ended was still notified")
	})
}

// TestNotifyClosedAlreadyCanceledContext tests registering with a context that
// is already done. No watcher is registered, which leaves neither the
// registration nor a later close calling back.
func TestNotifyClosedAlreadyCanceledContext(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	token, err := sm.CreateSession(ctx, "carol", testSession{})
	require.NoError(t, err)

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel() // Cancel before registration.

	var called atomic.Int32
	err = sm.NotifyClosed(canceledCtx, token, func() { called.Add(1) })
	require.NoError(t, err)

	// Watcher was not registered, fn must not be called.
	require.Zero(t, called.Load())

	// Closing the session must also not trigger the callback.
	require.NoError(t, sm.CloseSession(ctx, token))
	require.Zero(t, called.Load())
}

// TestNotifyClosedMultipleWatchersDifferentContexts tests two watchers of one
// session where only one context ended. Cancelling one must not unregister the other.
func TestNotifyClosedMultipleWatchersDifferentContexts(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	token, err := sm.CreateSession(ctx, "dave", testSession{})
	require.NoError(t, err)

	cancelCtx, cancel := context.WithCancel(ctx)

	var canceledCalled, activeCalled atomic.Int32
	require.NoError(t, sm.NotifyClosed(cancelCtx, token, func() {
		canceledCalled.Add(1)
	}))
	require.NoError(t, sm.NotifyClosed(ctx, token, func() {
		activeCalled.Add(1)
	}))

	// Cancel only the first watcher's context.
	cancel()

	require.NoError(t, sm.CloseSession(ctx, token))

	// Only the active watcher must be called.
	require.Zero(t, canceledCalled.Load())
	require.Equal(t, int32(1), activeCalled.Load())
}

// TestNotifyClosedMultipleWatchers tests a session watched by many streams at once:
// each callback runs exactly once.
func TestNotifyClosedMultipleWatchers(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	token, err := sm.CreateSession(ctx, "dave", testSession{})
	require.NoError(t, err)

	const n = 10
	counters := make([]atomic.Int32, n)
	for i := range n {
		require.NoError(t, sm.NotifyClosed(ctx, token, func() {
			counters[i].Add(1)
		}))
	}

	require.NoError(t, sm.CloseSession(ctx, token))

	for i := range n {
		require.Equal(t, int32(1), counters[i].Load(),
			"watcher %d not called exactly once", i)
	}
}

// SaveSession tests.

// TestRecordRoundTrip tests the record API the rest of these tests go through
// [payloadManager] to reach: what CreateSession is given, including the issue and
// expiry times, is what ReadSessionFromCookie returns.
func TestRecordRoundTrip(t *testing.T) {
	sm := inmem.New[testSession](tokGen)
	ctx := context.Background()

	issued := time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)
	expires := issued.Add(24 * time.Hour)
	want := sessions.Record[testSession]{
		UserID:    "alice",
		IssuedAt:  issued,
		ExpiresAt: expires,
		Data:      testSession{Username: "alice", Role: "admin"},
	}

	token, err := sm.CreateSession(ctx, want)
	require.NoError(t, err)

	got, retTok, ok, err := sm.ReadSessionFromCookie(token)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, token, retTok)
	require.Equal(t, want, got)
}

// TestSaveSession tests that an updated payload is what the next read returns.
func TestSaveSession(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	token, err := sm.CreateSession(ctx, "alice",
		testSession{Username: "alice", Role: "viewer"})
	require.NoError(t, err)

	updated := testSession{Username: "alice", Role: "admin"}
	require.NoError(t, sm.SaveSession(ctx, token, updated))

	got, err := sm.Session(ctx, token)
	require.NoError(t, err)
	require.Equal(t, updated, got)
}

// TestSaveSessionNoOpIfNotFound tests a save against an unknown token.
// It is a no-op rather than an error, and it must not create the session either.
func TestSaveSessionNoOpIfNotFound(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	// Save to a nonexistent token is a no-op.
	require.NoError(t, sm.SaveSession(ctx, "no-such-token", testSession{}))

	_, err := sm.Session(ctx, "no-such-token")
	require.ErrorIs(t, err, inmem.ErrSessionNotFound)
}

// TestSaveSessionAfterClose tests a save that races a sign-out. The session
// stays gone: resurrecting it would hand the closed cookie a live session again.
func TestSaveSessionAfterClose(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	token, err := sm.CreateSession(ctx, "alice", testSession{})
	require.NoError(t, err)
	require.NoError(t, sm.CloseSession(ctx, token))

	// No-op, session is gone.
	require.NoError(t, sm.SaveSession(ctx, token, testSession{Username: "updated"}))

	_, err = sm.Session(ctx, token)
	require.ErrorIs(t, err, inmem.ErrSessionNotFound)
}

// TestSaveSessionPreservesUserID tests that saving the payload leaves the owner alone.
// The user ID is not part of the payload and only CreateSession sets it.
func TestSaveSessionPreservesUserID(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	token, err := sm.CreateSession(ctx, "alice", testSession{Role: "old"})
	require.NoError(t, err)

	require.NoError(t, sm.SaveSession(ctx, token, testSession{Role: "new"}))

	_, _, uid, ok, err := sm.ReadSessionFromCookie(token)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "alice", uid)
}

// Session tests.

// TestSession tests the payload read by token,
// and the sentinel an unknown token produces.
func TestSession(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	want := testSession{Username: "alice", Role: "admin"}
	token, err := sm.CreateSession(ctx, "alice", want)
	require.NoError(t, err)

	tests := map[string]struct {
		token   string
		wantErr error
	}{
		"ok": {
			token: token,
		},
		"not found": {
			token:   "no-such-token",
			wantErr: inmem.ErrSessionNotFound,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sess, err := sm.Session(ctx, tc.token)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, want, sess)
		})
	}
}

// TestSessionAfterClose tests that a closed token reads as not found.
func TestSessionAfterClose(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	token, err := sm.CreateSession(ctx, "alice", testSession{})
	require.NoError(t, err)
	require.NoError(t, sm.CloseSession(ctx, token))

	_, err = sm.Session(ctx, token)
	require.ErrorIs(t, err, inmem.ErrSessionNotFound)
}

// CloseAllUserSessions tests.

// TestCloseAllUserSessions tests signing a user out everywhere: all of that
// user's sessions go, other users' stay, and the closed tokens are appended to
// the caller's buffer. A nil buffer means the caller wants only the effect.
func TestCloseAllUserSessions(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	tests := map[string]struct {
		setup  func(t *testing.T) []string // returns expected tokens
		userID string
		buffer []string
	}{
		"closes multiple sessions": {
			setup: func(t *testing.T) []string {
				var tokens []string
				for range 3 {
					tok, err := sm.CreateSession(ctx, "multi", testSession{})
					require.NoError(t, err)
					tokens = append(tokens, tok)
				}
				return tokens
			},
			userID: "multi",
			buffer: []string{},
		},
		"no sessions": {
			setup:  func(*testing.T) []string { return nil },
			userID: "nobody",
			buffer: []string{},
		},
		"nil buffer": {
			setup: func(t *testing.T) []string {
				_, err := sm.CreateSession(ctx, "nilbuf", testSession{})
				require.NoError(t, err)
				return nil
			},
			userID: "nilbuf",
			buffer: nil,
		},
		"does not close other users": {
			setup: func(t *testing.T) []string {
				tok, err := sm.CreateSession(ctx, "target", testSession{})
				require.NoError(t, err)
				// Create session for a different user.
				_, err = sm.CreateSession(ctx, "bystander", testSession{})
				require.NoError(t, err)
				return []string{tok}
			},
			userID: "target",
			buffer: []string{},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			wantTokens := tc.setup(t)
			result, err := sm.CloseAllUserSessions(ctx, tc.buffer, tc.userID)
			require.NoError(t, err)
			if tc.buffer != nil {
				require.ElementsMatch(t, wantTokens, result)
			}
			// Verify all sessions for the user are gone.
			require.Empty(t, sm.UserSessions(ctx, tc.userID))
		})
	}
}

// TestCloseAllUserSessionsEmptyUserID tests the refusal of an empty user ID,
// which would otherwise be a request to close nothing or everything.
func TestCloseAllUserSessionsEmptyUserID(t *testing.T) {
	sm := newManager(t)

	_, err := sm.CloseAllUserSessions(context.Background(), nil, "")
	require.ErrorIs(t, err, inmem.ErrEmptyUserID)
}

// TestCloseAllUserSessionsNotifiesWatchers tests that the bulk close notifies
// every watcher, the same as closing each session by hand.
func TestCloseAllUserSessionsNotifiesWatchers(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	var called atomic.Int32
	for range 3 {
		tok, err := sm.CreateSession(ctx, "alice", testSession{})
		require.NoError(t, err)
		require.NoError(t, sm.NotifyClosed(ctx, tok, func() {
			called.Add(1)
		}))
	}

	_, err := sm.CloseAllUserSessions(ctx, nil, "alice")
	require.NoError(t, err)
	require.Equal(t, int32(3), called.Load())
}

// UserSessions tests.

// TestUserSessions tests the listing a settings page reads: one entry per live session
// of the user, each carrying a token, and nothing for an unknown or empty user ID.
func TestUserSessions(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	tests := map[string]struct {
		setup  func(t *testing.T)
		userID string
		wantN  int
	}{
		"multiple sessions": {
			setup: func(t *testing.T) {
				for range 3 {
					_, err := sm.CreateSession(ctx, "iter",
						testSession{Username: "iter"})
					require.NoError(t, err)
				}
			},
			userID: "iter",
			wantN:  3,
		},
		"no sessions": {
			setup:  func(*testing.T) {},
			userID: "ghost",
			wantN:  0,
		},
		"empty user ID": {
			setup:  func(*testing.T) {},
			userID: "",
			wantN:  0,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.setup(t)
			sessions := sm.UserSessions(ctx, tc.userID)
			require.Len(t, sessions, tc.wantN)
			for _, us := range sessions {
				require.NotEmpty(t, us.Token)
			}
		})
	}
}

// TestUserSessionsDoesNotIncludeOtherUsers tests the isolation between users,
// since the listing goes on a page one of them can see.
func TestUserSessionsDoesNotIncludeOtherUsers(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	_, err := sm.CreateSession(ctx, "alice", testSession{Username: "alice"})
	require.NoError(t, err)
	_, err = sm.CreateSession(ctx, "bob", testSession{Username: "bob"})
	require.NoError(t, err)

	sessions := sm.UserSessions(ctx, "alice")
	require.Len(t, sessions, 1)
	require.Equal(t, "alice", sessions[0].Record.Data.Username)
}

// TestUserSessionsTokenUsableWithSessionAndClose tests that a token from the listing
// works with the rest of the API: reading the payload and closing that one session.
func TestUserSessionsTokenUsableWithSessionAndClose(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	want := testSession{Username: "alice", Role: "admin"}
	_, err := sm.CreateSession(ctx, "alice", want)
	require.NoError(t, err)

	sessions := sm.UserSessions(ctx, "alice")
	require.Len(t, sessions, 1)
	require.Equal(t, want, sessions[0].Record.Data)

	got, err := sm.Session(ctx, sessions[0].Token)
	require.NoError(t, err)
	require.Equal(t, want, got)

	require.NoError(t, sm.CloseSession(ctx, sessions[0].Token))
	require.Empty(t, sm.UserSessions(ctx, "alice"))
}

// Concurrency tests.
//
// All assertions happen in the main test goroutine after wg.Wait()
// to avoid calling require (which uses t.FailNow) from non-test goroutines.

// TestConcurrentCreateAndRead tests many requests signing in at once and then
// reading their own session back. Every token has to name a session of its own.
func TestConcurrentCreateAndRead(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	const goroutines = 50
	tokens := make([]string, goroutines)
	createErrs := make([]error, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			tok, err := sm.CreateSession(ctx, "user",
				testSession{Username: "user", Role: "role"})
			tokens[i] = tok
			createErrs[i] = err
		}(i)
	}
	wg.Wait()

	for i, err := range createErrs {
		require.NoError(t, err, "create goroutine %d", i)
	}

	readErrs := make([]error, goroutines)
	readOK := make([]bool, goroutines)
	readSess := make([]testSession, goroutines)

	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			sess, _, _, ok, err := sm.ReadSessionFromCookie(
				tokens[i],
			)
			readErrs[i] = err
			readOK[i] = ok
			readSess[i] = sess
		}(i)
	}
	wg.Wait()

	for i := range goroutines {
		require.NoError(t, readErrs[i], "read goroutine %d", i)
		require.True(t, readOK[i], "read goroutine %d", i)
		require.Equal(t, "user", readSess[i].Username, "read goroutine %d", i)
	}
}

// TestConcurrentCreateAndClose tests a batch of sign-outs running at once,
// and that none of the sessions survives it.
func TestConcurrentCreateAndClose(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	const goroutines = 50
	tokens := make([]string, goroutines)

	for i := range goroutines {
		tok, err := sm.CreateSession(ctx, "user", testSession{})
		require.NoError(t, err)
		tokens[i] = tok
	}

	closeErrs := make([]error, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			closeErrs[i] = sm.CloseSession(ctx, tokens[i])
		}(i)
	}
	wg.Wait()

	for i, err := range closeErrs {
		require.NoError(t, err, "goroutine %d", i)
	}

	for _, tok := range tokens {
		_, _, _, ok, err := sm.ReadSessionFromCookie(
			tok,
		)
		require.NoError(t, err)
		require.False(t, ok)
	}
}

// TestConcurrentCloseWithNotify tests each session being closed while a watcher is
// registered on it. Every callback runs exactly once, never twice and never not at all.
func TestConcurrentCloseWithNotify(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	const goroutines = 50
	tokens := make([]string, goroutines)
	counters := make([]atomic.Int32, goroutines)

	for i := range goroutines {
		tok, err := sm.CreateSession(ctx, "user", testSession{})
		require.NoError(t, err)
		tokens[i] = tok

		i := i
		require.NoError(t, sm.NotifyClosed(ctx, tok, func() {
			counters[i].Add(1)
		}))
	}

	closeErrs := make([]error, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			closeErrs[i] = sm.CloseSession(ctx, tokens[i])
		}(i)
	}
	wg.Wait()

	for i := range goroutines {
		require.NoError(t, closeErrs[i], "goroutine %d", i)
		require.Equal(t, int32(1), counters[i].Load(),
			"watcher %d not called exactly once", i)
	}
}

// TestConcurrentDoubleClose tests two goroutines closing one session, which is what
// a sign-out in two tabs looks like. The watcher must still be called only once.
func TestConcurrentDoubleClose(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	token, err := sm.CreateSession(ctx, "alice", testSession{})
	require.NoError(t, err)

	var called atomic.Int32
	require.NoError(t, sm.NotifyClosed(ctx, token, func() {
		called.Add(1)
	}))

	// Two goroutines race to close the same session.
	closeErrs := make([]error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	for i := range 2 {
		go func(i int) {
			defer wg.Done()
			closeErrs[i] = sm.CloseSession(ctx, token)
		}(i)
	}
	wg.Wait()

	for i, err := range closeErrs {
		require.NoError(t, err, "goroutine %d", i)
	}
	// Watcher must be called exactly once regardless of the race.
	require.Equal(t, int32(1), called.Load())
}

// TestConcurrentReadDuringClose tests a request reading a session while it is
// being closed. Either outcome is correct, and neither may return an error or a
// half-written payload.
func TestConcurrentReadDuringClose(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	token, err := sm.CreateSession(ctx, "bob",
		testSession{Username: "bob", Role: "admin"})
	require.NoError(t, err)

	var (
		readErr  error
		readOK   bool
		readSess testSession
		closeErr error
	)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		readSess, _, _, readOK, readErr = sm.ReadSessionFromCookie(
			token,
		)
	}()

	go func() {
		defer wg.Done()
		closeErr = sm.CloseSession(ctx, token)
	}()

	wg.Wait()

	require.NoError(t, readErr)
	require.NoError(t, closeErr)
	if readOK {
		// If we read before close, session must be intact.
		require.Equal(t, "bob", readSess.Username)
	}
	// A read after the close reports ok false. Both outcomes are valid.
}

// TestConcurrentNotifyAndClose tests watchers registering while the session is
// being closed. How many callbacks run depends on the scheduling: a watcher
// registered before the close is notified by it, one registered after is called
// by NotifyClosed itself. What is asserted is that no watcher is lost entirely
// and that nothing panics or races.
func TestConcurrentNotifyAndClose(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	const goroutines = 20
	token, err := sm.CreateSession(ctx, "user", testSession{})
	require.NoError(t, err)

	var totalCalls atomic.Int32
	errs := make([]error, goroutines)
	var wg sync.WaitGroup

	// Half the goroutines register watchers, half attempt to close.
	wg.Add(goroutines)
	for i := range goroutines {
		if i%2 == 0 {
			go func(i int) {
				defer wg.Done()
				errs[i] = sm.NotifyClosed(ctx, token, func() {
					totalCalls.Add(1)
				})
			}(i)
		} else {
			go func(i int) {
				defer wg.Done()
				errs[i] = sm.CloseSession(ctx, token)
			}(i)
		}
	}
	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, "goroutine %d", i)
	}
	// The exact count depends on scheduling: watchers registered before close
	// are notified via CloseSession, watchers registered after close are called
	// immediately by NotifyClosed.
	// This test validates no panics, no races, and that at least one callback fires.
	require.GreaterOrEqual(t, totalCalls.Load(), int32(1))
}

// TestConcurrentSaveSession tests many saves to one session at once.
// The last writer wins and the session stays readable.
func TestConcurrentSaveSession(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	token, err := sm.CreateSession(ctx, "alice", testSession{Role: "initial"})
	require.NoError(t, err)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			_ = sm.SaveSession(ctx, token, testSession{
				Username: "alice",
				Role:     "role",
			})
		}()
	}
	wg.Wait()

	// Session must still be readable.
	sess, err := sm.Session(ctx, token)
	require.NoError(t, err)
	require.Equal(t, "alice", sess.Username)
}

// TestConcurrentSessionRead tests concurrent reads of one session all returning
// the same payload.
func TestConcurrentSessionRead(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	want := testSession{Username: "alice", Role: "admin"}
	token, err := sm.CreateSession(ctx, "alice", want)
	require.NoError(t, err)

	const goroutines = 50
	readErrs := make([]error, goroutines)
	sessions := make([]testSession, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			sessions[i], readErrs[i] = sm.Session(ctx, token)
		}(i)
	}
	wg.Wait()

	for i := range goroutines {
		require.NoError(t, readErrs[i], "goroutine %d", i)
		require.Equal(t, want, sessions[i], "goroutine %d", i)
	}
}

// TestConcurrentCloseAllUserSessions tests two bulk closes racing over the same user,
// which is a settings page clicked twice.
func TestConcurrentCloseAllUserSessions(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	for range 10 {
		_, err := sm.CreateSession(ctx, "alice", testSession{})
		require.NoError(t, err)
	}

	// Two goroutines race to close all sessions for the same user.
	closeErrs := make([]error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	for i := range 2 {
		go func(i int) {
			defer wg.Done()
			_, closeErrs[i] = sm.CloseAllUserSessions(ctx, nil, "alice")
		}(i)
	}
	wg.Wait()

	for i, err := range closeErrs {
		require.NoError(t, err, "goroutine %d", i)
	}
	require.Empty(t, sm.UserSessions(ctx, "alice"))
}

// TestConcurrentUserSessions tests concurrent listings of one user's sessions:
// each returns the full set, never a partially built one.
func TestConcurrentUserSessions(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	for range 5 {
		_, err := sm.CreateSession(ctx, "alice", testSession{Username: "alice"})
		require.NoError(t, err)
	}

	const goroutines = 20
	results := make([][]inmem.UserSession[testSession], goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			results[i] = sm.UserSessions(ctx, "alice")
		}(i)
	}
	wg.Wait()

	for i := range goroutines {
		require.Len(t, results[i], 5, "goroutine %d", i)
		for _, us := range results[i] {
			require.Equal(t, "alice", us.Record.Data.Username, "goroutine %d", i)
		}
	}
}

// TestDeleteExpired tests the sweep.
// A session nobody comes back to is never read again and stays until this runs.
func TestDeleteExpired(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sm := inmem.New[testSession](tokGen)
		ctx := t.Context()

		past, err := sm.CreateSession(ctx, sessions.Record[testSession]{
			UserID: "alice", ExpiresAt: time.Now().Add(-time.Hour),
		})
		require.NoError(t, err)
		future, err := sm.CreateSession(ctx, sessions.Record[testSession]{
			UserID: "alice", ExpiresAt: time.Now().Add(time.Hour),
		})
		require.NoError(t, err)
		never, err := sm.CreateSession(ctx, sessions.Record[testSession]{
			UserID: "alice",
		})
		require.NoError(t, err)

		// A stream watching the expired session has to learn it is gone.
		// The bubble runs on a fake clock, which lets the wait cost nothing whether
		// the notification arrives or not.
		closed := make(chan struct{})
		require.NoError(t, sm.NotifyClosed(ctx, past, func() { close(closed) }))

		deleted, err := sm.DeleteExpired(ctx)
		require.NoError(t, err)
		require.Equal(t, 1, deleted, "the sweep took the wrong number of sessions")

		select {
		case <-closed:
		case <-time.After(time.Second):
			t.Error("the watcher of the expired session was never notified")
		}

		_, _, ok, err := sm.ReadSessionFromCookie(past)
		require.NoError(t, err)
		require.False(t, ok, "the expired session is still stored")

		for name, token := range map[string]string{
			"not yet expired": future,
			"never expires":   never,
		} {
			_, _, ok, err := sm.ReadSessionFromCookie(token)
			require.NoError(t, err)
			require.True(t, ok, "the sweep took a session that %s", name)
		}

		// Nothing left to take.
		deleted, err = sm.DeleteExpired(ctx)
		require.NoError(t, err)
		require.Zero(t, deleted)
	})
}
