package inmem_test

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/modules/sessmanager/inmem"
	"github.com/romshark/datapages/modules/sesstokgen"
)

type testSession struct {
	Username string
	Role     string
}

var tokGen = sesstokgen.Generator{}

func newManager(t *testing.T) *inmem.SessionManager[testSession] {
	t.Helper()
	return inmem.New[testSession](tokGen)
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

func TestReadSessionFromCookie(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	token, err := sm.CreateSession(ctx, "alice",
		testSession{Username: "alice", Role: "admin"})
	require.NoError(t, err)

	tests := map[string]struct {
		cookie   *http.Cookie
		wantOK   bool
		wantUID  string
		wantSess testSession
	}{
		"nil cookie": {
			cookie: nil, wantOK: false,
		},
		"empty value": {
			cookie: &http.Cookie{Value: ""}, wantOK: false,
		},
		"nonexistent token": {
			cookie: &http.Cookie{Value: "no-such-token"}, wantOK: false,
		},
		"valid session": {
			cookie:   &http.Cookie{Value: token},
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

func TestReadSessionFromCookieStale(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	token, err := sm.CreateSession(ctx, "alice", testSession{})
	require.NoError(t, err)
	require.NoError(t, sm.CloseSession(ctx, token))

	_, _, _, ok, err := sm.ReadSessionFromCookie(&http.Cookie{Value: token})
	require.NoError(t, err)
	require.False(t, ok)
}

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
				&http.Cookie{Value: token})
			require.NoError(t, err)
			require.True(t, ok)
			require.Equal(t, token, retTok)
			require.Equal(t, tc.userID, uid)
			require.Equal(t, tc.session, sess)
		})
	}
}

func TestCreateSessionEmptyUserID(t *testing.T) {
	sm := newManager(t)

	_, err := sm.CreateSession(context.Background(), "", testSession{})
	require.ErrorIs(t, err, inmem.ErrEmptyUserID)
}

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

func TestCreateSessionErrTokenGenerator(t *testing.T) {
	sm := inmem.New[testSession](failingTokGen{})

	_, err := sm.CreateSession(context.Background(), "bob", testSession{})
	require.ErrorIs(t, err, errFake)
}

// TestCreateSessionTokenCollisionOverwrites documents the current behavior:
// if the token generator produces a duplicate, the new session silently
// overwrites the old one. With a properly configured generator (256-bit
// random tokens) this is practically impossible.
func TestCreateSessionTokenCollisionOverwrites(t *testing.T) {
	sm := inmem.New[testSession](fixedTokGen{token: "same-token"})
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
		&http.Cookie{Value: tok1})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, tok1, retTok)
	require.Equal(t, "bob", uid)
	require.Equal(t, testSession{Username: "bob", Role: "user"}, sess)
}

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
				&http.Cookie{Value: token})
			require.NoError(t, err)
			require.False(t, ok)
		})
	}
}

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

func TestNotifyClosedSessionDoesNotExistNeverCreated(t *testing.T) {
	sm := newManager(t)

	var called atomic.Int32
	err := sm.NotifyClosed(context.Background(), "never-existed", func() {
		called.Add(1)
	})
	require.NoError(t, err)
	require.Equal(t, int32(1), called.Load())
}

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

func TestNotifyClosedContextCancellation(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	token, err := sm.CreateSession(ctx, "carol", testSession{})
	require.NoError(t, err)

	watchCtx, cancel := context.WithCancel(ctx)

	var called atomic.Int32
	err = sm.NotifyClosed(watchCtx, token, func() { called.Add(1) })
	require.NoError(t, err)

	// Cancel the watcher context and wait for the cleanup goroutine
	// to remove it from the watchers map (session stays alive).
	cancel()
	require.Eventually(t, func() bool {
		// Close the session after cleanup has run — fn must NOT be called.
		_ = sm.CloseSession(ctx, token)
		return called.Load() == 0
	}, 2*time.Second, 10*time.Millisecond)
}

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

func TestNotifyClosedMultipleWatchers(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	token, err := sm.CreateSession(ctx, "dave", testSession{})
	require.NoError(t, err)

	const n = 10
	counters := make([]atomic.Int32, n)
	for i := range n {
		i := i
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

func TestSaveSessionNoOpIfNotFound(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	// Save to a nonexistent token is a no-op.
	require.NoError(t, sm.SaveSession(ctx, "no-such-token", testSession{}))

	_, err := sm.Session(ctx, "no-such-token")
	require.ErrorIs(t, err, inmem.ErrSessionNotFound)
}

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

func TestSaveSessionPreservesUserID(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	token, err := sm.CreateSession(ctx, "alice", testSession{Role: "old"})
	require.NoError(t, err)

	require.NoError(t, sm.SaveSession(ctx, token, testSession{Role: "new"}))

	_, _, uid, ok, err := sm.ReadSessionFromCookie(&http.Cookie{Value: token})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "alice", uid)
}

// Session tests.

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

func TestCloseAllUserSessionsEmptyUserID(t *testing.T) {
	sm := newManager(t)

	_, err := sm.CloseAllUserSessions(context.Background(), nil, "")
	require.ErrorIs(t, err, inmem.ErrEmptyUserID)
}

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

func TestUserSessionsDoesNotIncludeOtherUsers(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	_, err := sm.CreateSession(ctx, "alice", testSession{Username: "alice"})
	require.NoError(t, err)
	_, err = sm.CreateSession(ctx, "bob", testSession{Username: "bob"})
	require.NoError(t, err)

	sessions := sm.UserSessions(ctx, "alice")
	require.Len(t, sessions, 1)
	require.Equal(t, "alice", sessions[0].Session.Username)
}

func TestUserSessionsTokenUsableWithSessionAndClose(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	want := testSession{Username: "alice", Role: "admin"}
	_, err := sm.CreateSession(ctx, "alice", want)
	require.NoError(t, err)

	sessions := sm.UserSessions(ctx, "alice")
	require.Len(t, sessions, 1)
	require.Equal(t, want, sessions[0].Session)

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
				&http.Cookie{Value: tokens[i]})
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
			&http.Cookie{Value: tok})
		require.NoError(t, err)
		require.False(t, ok)
	}
}

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
			&http.Cookie{Value: token})
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
	// If we read after close, ok is false — both are valid outcomes.
}

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
	// immediately by NotifyClosed. This test validates no panics, no races,
	// and that at least one callback fires.
	require.GreaterOrEqual(t, totalCalls.Load(), int32(1))
}

func TestConcurrentSaveSession(t *testing.T) {
	sm := newManager(t)
	ctx := context.Background()

	token, err := sm.CreateSession(ctx, "alice", testSession{Role: "initial"})
	require.NoError(t, err)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			_ = sm.SaveSession(ctx, token, testSession{
				Username: "alice",
				Role:     "role",
			})
		}(i)
	}
	wg.Wait()

	// Session must still be readable.
	sess, err := sm.Session(ctx, token)
	require.NoError(t, err)
	require.Equal(t, "alice", sess.Username)
}

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
			require.Equal(t, "alice", us.Session.Username, "goroutine %d", i)
		}
	}
}
