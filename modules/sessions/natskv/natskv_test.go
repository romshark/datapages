package natskv_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"maps"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	natsctr "github.com/testcontainers/testcontainers-go/modules/nats"

	"github.com/romshark/datapages/modules/sessions"
	"github.com/romshark/datapages/modules/sessions/natskv"
)

type testSession struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

var tokGen = sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen}

func setupNATS(t *testing.T) *nats.Conn {
	t.Helper()
	ctx := context.Background()
	ctr, err := natsctr.Run(ctx, "nats:latest")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ctr.Terminate(ctx)) })

	url, err := ctr.ConnectionString(ctx)
	require.NoError(t, err)

	// The testcontainers NATS module only waits for the port to be open,
	// not for the server to be fully initialized. Use nats.RetryOnFailedConnect
	// so the client keeps retrying until NATS is ready.
	conn, err := nats.Connect(
		url,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(50),
		nats.ReconnectWait(200*time.Millisecond),
	)
	require.NoError(t, err)
	require.Eventually(t, conn.IsConnected, 10*time.Second, 100*time.Millisecond)
	t.Cleanup(conn.Close)
	return conn
}

func newManager(
	t *testing.T, conn *nats.Conn, conf natskv.Config,
) payloadManager {
	t.Helper()
	sm, err := natskv.New[testSession](conn, tokGen, conf)
	require.NoError(t, err)
	return payloadManager{sm}
}

// payloadManager adapts the record-based manager API to the payload-shaped calls
// these tests are written against.
type payloadManager struct {
	*natskv.SessionManager[testSession]
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
		return err
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

// kvFor returns a direct KV handle for the given bucket.
func kvFor(t *testing.T, conn *nats.Conn, bucket string) nats.KeyValue {
	t.Helper()
	js, err := conn.JetStream()
	require.NoError(t, err)
	kv, err := js.KeyValue(bucket)
	require.NoError(t, err)
	return kv
}

func validKey() []byte { return []byte("0123456789abcdef") }

// compositeKey builds a composite KV key matching the format used by natskv.
func compositeKey(userID, sessionID string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(userID)) + "." + sessionID
}

// TestNew tests the constructor against a live NATS server: the default bucket,
// a custom one and one that already exists all work, and an encryption key of the
// wrong length is refused up front rather than at the first write.
func TestNew(t *testing.T) {
	conn := setupNATS(t)

	t.Run("ok default bucket", func(t *testing.T) {
		sm, err := natskv.New[testSession](conn, tokGen, natskv.Config{
			EncryptionKey: validKey(),
		})
		require.NoError(t, err)
		require.NotNil(t, sm)
	})

	t.Run("ok custom bucket", func(t *testing.T) {
		sm, err := natskv.New[testSession](conn, tokGen, natskv.Config{
			EncryptionKey: validKey(),
			KVConfig:      nats.KeyValueConfig{Bucket: "CUSTOM"},
		})
		require.NoError(t, err)
		require.NotNil(t, sm)
	})

	t.Run("ok existing bucket", func(t *testing.T) {
		bucket := "EXISTING_BUCKET"
		js, err := conn.JetStream()
		require.NoError(t, err)
		_, err = js.CreateKeyValue(&nats.KeyValueConfig{Bucket: bucket})
		require.NoError(t, err)

		sm, err := natskv.New[testSession](conn, tokGen, natskv.Config{
			EncryptionKey: validKey(),
			KVConfig:      nats.KeyValueConfig{Bucket: bucket},
		})
		require.NoError(t, err)
		require.NotNil(t, sm)
	})

	t.Run("err primary key wrong length", func(t *testing.T) {
		_, err := natskv.New[testSession](conn, tokGen, natskv.Config{
			EncryptionKey: []byte("short"),
		})
		require.ErrorIs(t, err, natskv.ErrEncryptionKeyLen)
	})

	t.Run("err previous key wrong length", func(t *testing.T) {
		_, err := natskv.New[testSession](conn, tokGen, natskv.Config{
			EncryptionKey:          validKey(),
			PreviousEncryptionKeys: [][]byte{[]byte("bad")},
		})
		require.ErrorIs(t, err, natskv.ErrEncryptionKeyLen)
	})
}

// TestSaveSession tests that an updated payload is re-encrypted and stored,
// and that the next read returns it.
func TestSaveSession(t *testing.T) {
	conn := setupNATS(t)
	sm := newManager(t, conn, natskv.Config{
		EncryptionKey: validKey(),
		KVConfig:      nats.KeyValueConfig{Bucket: "SAVE"},
	})
	ctx := context.Background()

	original := testSession{Username: "alice", Role: "viewer"}
	token, err := sm.CreateSession(ctx, "alice", original)
	require.NoError(t, err)

	updated := testSession{Username: "alice", Role: "admin"}
	require.NoError(t, sm.SaveSession(ctx, token, updated))

	got, err := sm.Session(ctx, token)
	require.NoError(t, err)
	require.Equal(t, updated, got)
}

// TestSaveSessionUserIDMismatch tests a record naming a different user than
// the key it would be stored under. Accepting it made ReadSessionFromCookie,
// Session and UserSessions report three different answers about who is signed in.
func TestSaveSessionUserIDMismatch(t *testing.T) {
	conn := setupNATS(t)
	sm := newManager(t, conn, natskv.Config{
		EncryptionKey: validKey(),
		KVConfig:      nats.KeyValueConfig{Bucket: "SAVE_MISMATCH"},
	})
	ctx := context.Background()

	token, err := sm.CreateSession(ctx, "alice", testSession{Username: "alice"})
	require.NoError(t, err)

	err = sm.SessionManager.SaveSession(ctx, token, sessions.Record[testSession]{
		UserID: "bob",
		Data:   testSession{Username: "bob"},
	})
	require.ErrorIs(t, err, natskv.ErrUserIDMismatch)

	rec, _, _, ok, err := sm.ReadSessionFromCookie(token)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, testSession{Username: "alice"}, rec)
}

// TestSaveSessionInvalidToken tests a token that does not decrypt.
// Unlike the in-memory manager this reports an error, since the token itself is
// malformed rather than merely unknown.
func TestSaveSessionInvalidToken(t *testing.T) {
	conn := setupNATS(t)
	sm := newManager(t, conn, natskv.Config{
		EncryptionKey: validKey(),
		KVConfig:      nats.KeyValueConfig{Bucket: "SAVE_BAD"},
	})

	err := sm.SaveSession(context.Background(), "!!!bad!!!", testSession{})
	require.Error(t, err)
}

// TestCreateSession tests that a created session is readable back under its token,
// and that an empty user ID is refused.
func TestCreateSession(t *testing.T) {
	conn := setupNATS(t)
	sm := newManager(t, conn, natskv.Config{
		EncryptionKey: validKey(),
		KVConfig:      nats.KeyValueConfig{Bucket: "CREATE"},
	})
	ctx := context.Background()

	t.Run("ok", func(t *testing.T) {
		token, err := sm.CreateSession(ctx, "bob", testSession{
			Username: "bob", Role: "user",
		})
		require.NoError(t, err)
		require.NotEmpty(t, token)

		sess, err := sm.Session(ctx, token)
		require.NoError(t, err)
		require.Equal(t, "bob", sess.Username)
	})

	t.Run("empty user ID", func(t *testing.T) {
		_, err := sm.CreateSession(ctx, "", testSession{})
		require.ErrorIs(t, err, natskv.ErrEmptyUserID)
	})
}

// TestCreateSessionErrTokenGenerator tests a failing token generator:
// the error reaches the caller and nothing is written to the bucket.
func TestCreateSessionErrTokenGenerator(t *testing.T) {
	conn := setupNATS(t)
	sm, err := natskv.New[testSession](conn, failingTokGen{}, natskv.Config{
		EncryptionKey: validKey(),
		KVConfig:      nats.KeyValueConfig{Bucket: "CREATE_ERR"},
	})
	require.NoError(t, err)

	_, err = payloadManager{sm}.CreateSession(
		context.Background(), "bob", testSession{},
	)
	require.Error(t, err)
}

type failingTokGen struct{}

func (failingTokGen) Generate() (string, error) {
	return "", errFake
}

var errFake = &fakeError{}

type fakeError struct{}

func (*fakeError) Error() string { return "fake error" }

// TestSession tests the payload read by token: a live session, a closed one
// reported as not found, and a token that does not decrypt at all.
func TestSession(t *testing.T) {
	conn := setupNATS(t)
	sm := newManager(t, conn, natskv.Config{
		EncryptionKey: validKey(),
		KVConfig:      nats.KeyValueConfig{Bucket: "SESS"},
	})
	ctx := context.Background()

	tests := map[string]struct {
		setup   func(t *testing.T) string
		wantErr error
	}{
		"ok": {
			setup: func(t *testing.T) string {
				tok, err := sm.CreateSession(ctx, "alice",
					testSession{Username: "alice", Role: "admin"})
				require.NoError(t, err)
				return tok
			},
		},
		"not found": {
			setup: func(t *testing.T) string {
				tok, err := sm.CreateSession(ctx, "gone", testSession{})
				require.NoError(t, err)
				require.NoError(t, sm.CloseSession(ctx, tok))
				return tok
			},
			wantErr: natskv.ErrSessionNotFound,
		},
		"invalid token": {
			setup: func(*testing.T) string {
				return "not-valid-encrypted-token!!!"
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			token := tc.setup(t)
			sess, err := sm.Session(ctx, token)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else if name == "ok" {
				require.NoError(t, err)
				require.Equal(t, "alice", sess.Username)
				require.Equal(t, "admin", sess.Role)
			} else {
				require.Error(t, err)
			}
		})
	}
}

// TestSessionBadJSON tests a KV entry corrupted behind the manager's back.
// The read fails instead of returning a zero payload as though it were the session.
func TestSessionBadJSON(t *testing.T) {
	conn := setupNATS(t)
	bucket := "SESS_BADJSON"
	sm := newManager(t, conn, natskv.Config{
		EncryptionKey: validKey(),
		KVConfig:      nats.KeyValueConfig{Bucket: bucket},
	})
	ctx := context.Background()

	// Create a valid session to get a token, then overwrite the KV entry with bad JSON.
	token, err := sm.CreateSession(ctx, "alice", testSession{})
	require.NoError(t, err)

	kv := kvFor(t, conn, bucket)
	keys, err := kv.Keys()
	require.NoError(t, err)
	require.Len(t, keys, 1)
	_, err = kv.Put(keys[0], []byte("{invalid"))
	require.NoError(t, err)

	_, err = sm.Session(ctx, token)
	require.Error(t, err)
}

// TestReadSessionFromCookie tests every cookie the browser can send: empty, not base64,
// encrypted under a key this manager does not hold, naming a closed session, and valid.
// Only the last is a hit, and none of the others is an error:
// a bad cookie is a visitor without a session.
func TestReadSessionFromCookie(t *testing.T) {
	conn := setupNATS(t)
	sm := newManager(t, conn, natskv.Config{
		EncryptionKey: validKey(),
		KVConfig:      nats.KeyValueConfig{Bucket: "READ"},
	})
	ctx := context.Background()

	token, err := sm.CreateSession(ctx, "carol",
		testSession{Username: "carol", Role: "editor"})
	require.NoError(t, err)

	staleTok, err := sm.CreateSession(ctx, "old", testSession{})
	require.NoError(t, err)
	require.NoError(t, sm.CloseSession(ctx, staleTok))

	tests := map[string]struct {
		cookie  string
		wantOK  bool
		wantUID string
	}{
		"empty value": {
			cookie: "", wantOK: false,
		},
		"invalid base64": {
			cookie: "!!!bad!!!", wantOK: false,
		},
		"wrong encryption key": {
			cookie: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			wantOK: false,
		},
		"stale session": {
			cookie: staleTok, wantOK: false,
		},
		"valid session": {
			cookie: token, wantOK: true, wantUID: "carol",
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
				require.Equal(t, "carol", sess.Username)
			} else {
				require.False(t, ok)
				_ = sess
			}
		})
	}
}

// TestReadSessionFromCookieBadJSON tests a corrupted KV entry on the request path.
// The request continues as a visitor without a session rather than failing.
func TestReadSessionFromCookieBadJSON(t *testing.T) {
	conn := setupNATS(t)
	bucket := "READ_BADJSON"
	sm := newManager(t, conn, natskv.Config{
		EncryptionKey: validKey(),
		KVConfig:      nats.KeyValueConfig{Bucket: bucket},
	})
	ctx := context.Background()

	// Create to get a valid token, then corrupt the stored JSON.
	token, err := sm.CreateSession(ctx, "corrupt", testSession{})
	require.NoError(t, err)

	kv := kvFor(t, conn, bucket)
	keys, err := kv.Keys()
	require.NoError(t, err)
	require.Len(t, keys, 1)
	_, err = kv.Put(keys[0], []byte("not-json"))
	require.NoError(t, err)

	_, _, _, ok, err := sm.ReadSessionFromCookie(token)
	require.NoError(t, err)
	require.False(t, ok)
}

// TestCloseSession tests that a closed session is gone from the bucket,
// and how the manager answers a token that is already closed or malformed.
func TestCloseSession(t *testing.T) {
	conn := setupNATS(t)
	sm := newManager(t, conn, natskv.Config{
		EncryptionKey: validKey(),
		KVConfig:      nats.KeyValueConfig{Bucket: "CLOSE"},
	})
	ctx := context.Background()

	tests := map[string]struct {
		setup   func(t *testing.T) string
		wantErr bool
	}{
		"ok": {
			setup: func(t *testing.T) string {
				tok, err := sm.CreateSession(ctx, "alice", testSession{})
				require.NoError(t, err)
				return tok
			},
		},
		"already deleted": {
			setup: func(t *testing.T) string {
				tok, err := sm.CreateSession(ctx, "alice", testSession{})
				require.NoError(t, err)
				require.NoError(t, sm.CloseSession(ctx, tok))
				m := maps.Collect(sm.UserSessions(ctx, "alice"))
				require.Len(t, m, 0)
				return tok
			},
		},
		"nonexistent session": {
			setup: func(t *testing.T) string {
				// Create a token via a different bucket so the KV key
				// was never written to sm's bucket.
				other := newManager(t, conn, natskv.Config{
					EncryptionKey: validKey(),
					KVConfig:      nats.KeyValueConfig{Bucket: "CLOSE_OTHER"},
				})
				tok, err := other.CreateSession(ctx, "alice", testSession{})
				require.NoError(t, err)
				return tok
			},
		},
		"invalid token": {
			setup:   func(*testing.T) string { return "!!!bad!!!" },
			wantErr: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			token := tc.setup(t)
			err := sm.CloseSession(ctx, token)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				// Verify session is gone.
				_, err = sm.Session(ctx, token)
				require.ErrorIs(t, err, natskv.ErrSessionNotFound)
				_, _, _, ok, err := sm.ReadSessionFromCookie(token)
				require.NoError(t, err)
				require.False(t, ok)
			}
		})
	}
}

// TestCloseAllUserSessions tests signing a user out everywhere. The closed
// tokens land in the caller's buffer, a nil buffer means the caller wants only
// the effect, and an empty user ID is refused.
func TestCloseAllUserSessions(t *testing.T) {
	conn := setupNATS(t)
	sm := newManager(t, conn, natskv.Config{
		EncryptionKey: validKey(),
		KVConfig:      nats.KeyValueConfig{Bucket: "CLOSEALL"},
	})
	ctx := context.Background()

	tests := map[string]struct {
		setup   func(t *testing.T) []string // returns expected tokens
		userID  string
		buffer  []string
		wantErr error
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
		"empty user ID": {
			setup:   func(*testing.T) []string { return nil },
			userID:  "",
			wantErr: natskv.ErrEmptyUserID,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			wantTokens := tc.setup(t)
			result, err := sm.CloseAllUserSessions(ctx, tc.buffer, tc.userID)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			if tc.buffer != nil {
				// The tokens are rebuilt from the keys, not read back from the bucket,
				// so they carry a fresh nonce and are not the bytes
				// CreateSession returned. What has to match is the count.
				require.Len(t, result, len(wantTokens))
				require.Len(t, slices.Compact(slices.Sorted(slices.Values(result))),
					len(wantTokens), "duplicate tokens")
			}
			if tc.userID != "" {
				m := maps.Collect(sm.UserSessions(ctx, tc.userID))
				require.Len(t, m, 0)
			}
		})
	}
}

// TestUserSessions tests the iterator a settings page reads: one entry per live
// session of the user, and nothing for an unknown or empty user ID.
func TestUserSessions(t *testing.T) {
	conn := setupNATS(t)
	sm := newManager(t, conn, natskv.Config{
		EncryptionKey: validKey(),
		KVConfig:      nats.KeyValueConfig{Bucket: "USERSESS"},
	})
	ctx := context.Background()

	tests := map[string]struct {
		setup  func(t *testing.T)
		userID string
		wantN  int
	}{
		"multiple sessions": {
			setup: func(t *testing.T) {
				for range 2 {
					_, err := sm.CreateSession(ctx, "iter",
						testSession{Username: "iter", Role: "user"})
					require.NoError(t, err)
				}
			},
			userID: "iter",
			wantN:  2,
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
			var count int
			for tok, sess := range sm.UserSessions(ctx, tc.userID) {
				require.NotEmpty(t, tok)
				_ = sess
				count++
			}
			require.Equal(t, tc.wantN, count)
		})
	}
}

// TestIterateAndCloseSessions tests that a token the iterator yields works with
// the rest of the API. Closing a session while iterating must leave nothing behind.
func TestIterateAndCloseSessions(t *testing.T) {
	conn := setupNATS(t)
	sm := newManager(t, conn, natskv.Config{
		EncryptionKey: validKey(),
		KVConfig:      nats.KeyValueConfig{Bucket: "ITER_AND_CLOSE"},
	})
	ctx := context.Background()

	want := testSession{Username: "alice", Role: "admin"}
	created, err := sm.CreateSession(ctx, "alice", want)
	require.NoError(t, err)

	// Token from UserSessions must be usable with Session and CloseSession.
	for tok, rec := range sm.UserSessions(ctx, "alice") {
		require.Equal(t, want, rec.Data)
		require.NotEqual(t, created, tok,
			"the bucket handed back the cookie the client carries")

		got, err := sm.Session(ctx, tok)
		require.NoError(t, err)
		require.Equal(t, want, got)

		require.NoError(t, sm.CloseSession(ctx, tok))
	}

	// Session should be gone.
	m := maps.Collect(sm.UserSessions(ctx, "alice"))
	require.Len(t, m, 0)
}

// TestUserSessionsBreakEarly tests a caller that stops after the first entry.
// The iterator has to return rather than keep pulling from the KV watcher.
func TestUserSessionsBreakEarly(t *testing.T) {
	conn := setupNATS(t)
	sm := newManager(t, conn, natskv.Config{
		EncryptionKey: validKey(),
		KVConfig:      nats.KeyValueConfig{Bucket: "USERSESS_BREAK"},
	})
	ctx := context.Background()

	for range 3 {
		_, err := sm.CreateSession(ctx, "breakuser", testSession{})
		require.NoError(t, err)
	}

	var count int
	for range sm.UserSessions(ctx, "breakuser") {
		count++
		break
	}
	require.Equal(t, 1, count)
}

// TestUserSessionsBadJSON tests a corrupted entry among a user's sessions.
// The iterator skips it and yields the rest: one bad key must not hide the whole
// listing from the settings page.
func TestUserSessionsBadJSON(t *testing.T) {
	conn := setupNATS(t)
	bucket := "USERSESS_BADJSON"
	sm := newManager(t, conn, natskv.Config{
		EncryptionKey: validKey(),
		KVConfig:      nats.KeyValueConfig{Bucket: bucket},
	})
	ctx := context.Background()

	// Create a valid session plus one with bad JSON.
	_, err := sm.CreateSession(ctx, "badjson", testSession{Username: "ok"})
	require.NoError(t, err)

	kv := kvFor(t, conn, bucket)
	key := compositeKey("badjson", "bad")
	_, err = kv.Put(key, []byte("not-json"))
	require.NoError(t, err)

	// Iterator should skip the bad entry and yield the good one.
	m := maps.Collect(sm.UserSessions(ctx, "badjson"))
	require.Len(t, m, 1)
}

type callCounter struct{ atomic.Int32 }

func (c *callCounter) Inc() { c.Add(1) }

// TestKeyRotation tests a deployment that changed its encryption key. A session written
// under the old key still reads, since the old key is kept in PreviousEncryptionKeys,
// which is what lets the key rotate without signing everyone out.
func TestKeyRotation(t *testing.T) {
	conn := setupNATS(t)
	veryOldKey := []byte("veryoldkey012345")
	oldKey := []byte("oldkey0123456789")
	newKey := []byte("newkey0123456789")

	smOld := newManager(t, conn, natskv.Config{
		EncryptionKey:          oldKey,
		PreviousEncryptionKeys: [][]byte{veryOldKey},
		KVConfig:               nats.KeyValueConfig{Bucket: "ROTATE"},
	})
	ctx := context.Background()
	token, err := smOld.CreateSession(ctx, "alice",
		testSession{Username: "alice", Role: "admin"})
	require.NoError(t, err)

	smNew := newManager(t, conn, natskv.Config{
		EncryptionKey:          newKey,
		PreviousEncryptionKeys: [][]byte{veryOldKey, oldKey},
		KVConfig:               nats.KeyValueConfig{Bucket: "ROTATE"},
	})

	sess, err := smNew.Session(ctx, token)
	require.NoError(t, err)
	require.Equal(t, "alice", sess.Username)

	cookie := token
	sess, _, uid, ok, err := smNew.ReadSessionFromCookie(cookie)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "alice", uid)
	require.Equal(t, "admin", sess.Role)
}

// TestNotifyClosed tests the callback an open SSE stream waits on.
// A session already gone calls back at once, a live one does not, a close reaches the
// watcher through the KV watch, a cancelled context stops the watcher,
// and a malformed token is an error rather than a watcher that never fires.
func TestNotifyClosed(t *testing.T) {
	conn := setupNATS(t)

	t.Run("already deleted", func(t *testing.T) {
		sm := newManager(t, conn, natskv.Config{
			EncryptionKey: validKey(),
			KVConfig:      nats.KeyValueConfig{Bucket: "NOTIFY_DEL"},
		})
		ctx := context.Background()

		token, err := sm.CreateSession(ctx, "alice", testSession{})
		require.NoError(t, err)
		require.NoError(t, sm.CloseSession(ctx, token))

		var called callCounter
		err = sm.NotifyClosed(ctx, token, called.Inc)
		require.NoError(t, err)
		require.Equal(t, int32(1), called.Load())
	})

	t.Run("session exists fn not called", func(t *testing.T) {
		sm := newManager(t, conn, natskv.Config{
			EncryptionKey: validKey(),
			KVConfig:      nats.KeyValueConfig{Bucket: "NOTIFY_EXISTS"},
		})
		ctx := context.Background()

		userID, sess := "bob", testSession{Username: "bobby", Role: "bar"}
		token, err := sm.CreateSession(ctx, userID, sess)
		require.NoError(t, err)

		var called callCounter
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		err = sm.NotifyClosed(ctx, token, called.Inc)
		require.NoError(t, err)

		// The session still exists: fn must not have been called.
		m := maps.Collect(sm.UserSessions(ctx, userID))
		require.Len(t, m, 1)
		for _, rec := range m {
			require.Equal(t, sess, rec.Data)
		}

		require.Zero(t, called.Load())
	})

	t.Run("delete after setup", func(t *testing.T) {
		sm := newManager(t, conn, natskv.Config{
			EncryptionKey: validKey(),
			KVConfig:      nats.KeyValueConfig{Bucket: "NOTIFY_LIVE"},
		})
		ctx := context.Background()

		token, err := sm.CreateSession(ctx, "alice", testSession{})
		require.NoError(t, err)

		var called callCounter
		err = sm.NotifyClosed(ctx, token, called.Inc)
		require.NoError(t, err)

		// Barrier: wait for the watcher goroutine to finish initial replay.
		_ = maps.Collect(sm.UserSessions(ctx, "alice"))

		require.NoError(t, sm.CloseSession(ctx, token))

		require.Eventually(t, func() bool {
			return called.Load() == 1
		}, 5*time.Second, 10*time.Millisecond)
	})

	t.Run("context cancellation stops watcher", func(t *testing.T) {
		sm := newManager(t, conn, natskv.Config{
			EncryptionKey: validKey(),
			KVConfig:      nats.KeyValueConfig{Bucket: "NOTIFY_CTX"},
		})
		ctx, cancel := context.WithCancel(context.Background())

		token, err := sm.CreateSession(ctx, "ctx", testSession{})
		require.NoError(t, err)

		var called callCounter
		err = sm.NotifyClosed(ctx, token, called.Inc)
		require.NoError(t, err)

		m := maps.Collect(sm.UserSessions(context.Background(), "ctx"))
		require.Len(t, m, 1)

		cancel()

		// A fresh-context NATS round-trip acts as a barrier: by the time
		// it returns, the cancelled goroutine has had time to observe
		// ctx.Done() and exit.
		m = maps.Collect(sm.UserSessions(context.Background(), "ctx"))
		require.Len(t, m, 1)
		require.Zero(t, called.Load())
	})

	t.Run("invalid token", func(t *testing.T) {
		sm := newManager(t, conn, natskv.Config{
			EncryptionKey: validKey(),
			KVConfig:      nats.KeyValueConfig{Bucket: "NOTIFY_BAD"},
		})
		err := sm.NotifyClosed(context.Background(), "!!!bad!!!", func() {})
		require.Error(t, err)
	})
}

// TestNotifyClosedConnectionLost tests that the watcher goroutine ends when the
// NATS connection goes away, rather than spinning on the closed updates channel
// for as long as the context lives.
func TestNotifyClosedConnectionLost(t *testing.T) {
	conn := setupNATS(t)
	sm := newManager(t, conn, natskv.Config{
		EncryptionKey: validKey(),
		KVConfig:      nats.KeyValueConfig{Bucket: "NOTIFY_CONN_LOST"},
	})
	ctx := t.Context()

	token, err := sm.CreateSession(ctx, "alice", testSession{})
	require.NoError(t, err)

	var called callCounter
	require.NoError(t, sm.NotifyClosed(ctx, token, called.Inc))

	// Barrier: wait for the watcher goroutine to finish initial replay.
	_ = maps.Collect(sm.UserSessions(ctx, "alice"))

	conn.Close()

	// Everything after conn.Close() is in-process: the subscription loop ends,
	// the updates channel closes and the goroutine returns.
	// A second is scheduling headroom, not a round trip to NATS.
	require.Eventually(t, func() bool {
		return !goroutineRunning(".NotifyClosed")
	}, time.Second, 10*time.Millisecond, "watcher goroutine still running")
	require.Zero(t, called.Load())
}

// goroutineRunning reports whether any goroutine has
// a frame whose function name contains fn.
//
// [runtime.GoroutineProfile], [runtime.StackRecord], [runtime.CallersFrames] and
// [runtime.Frame.Function] are all covered by the Go 1 compatibility promise,
// https://go.dev/doc/go1compat. The spelling of the name a closure gets is not:
// only the enclosing method name is worth passing, since the ".funcN" suffix and
// the "[...]" a generic receiver renders as are compiler conventions.
//
// [runtime.GoroutineProfile] is used over [runtime.Stack] because a buffer too
// small for the text dump truncates it silently, which would report a spinning
// goroutine as gone.
func goroutineRunning(fn string) bool {
	recs := make([]runtime.StackRecord, runtime.NumGoroutine()+8)
	for {
		n, ok := runtime.GoroutineProfile(recs)
		if ok {
			recs = recs[:n]
			break
		}
		recs = make([]runtime.StackRecord, n+8)
	}
	for i := range recs {
		frames := runtime.CallersFrames(recs[i].Stack())
		for {
			f, more := frames.Next()
			if strings.Contains(f.Function, fn) {
				return true
			}
			if !more {
				break
			}
		}
	}
	return false
}

// TestDecryptShortCiphertext verifies that tokens whose base64-decoded
// payload is shorter than the AES-GCM nonce (12 bytes) are rejected
// gracefully instead of causing an out-of-bounds slice access.
func TestDecryptShortCiphertext(t *testing.T) {
	conn := setupNATS(t)
	sm := newManager(t, conn, natskv.Config{
		EncryptionKey: validKey(),
		KVConfig:      nats.KeyValueConfig{Bucket: "SHORT_CT"},
	})

	ctx := context.Background()

	shortToken := base64.RawURLEncoding.EncodeToString([]byte("short"))

	_, err := sm.Session(ctx, shortToken)
	require.ErrorIs(t, err, natskv.ErrCiphertextTooShort)

	var calls callCounter
	err = sm.NotifyClosed(ctx, shortToken, calls.Inc)
	require.ErrorIs(t, err, natskv.ErrCiphertextTooShort)
	require.Zero(t, calls.Load())

	err = sm.CloseSession(ctx, shortToken)
	require.ErrorIs(t, err, natskv.ErrCiphertextTooShort)
}

// unsafeGen returns session IDs carrying the NATS KV syntax,
// which [natskv.SessionTokenGenerator] allows an application to return.
type unsafeGen struct {
	prefix string
	n      int
}

func (g *unsafeGen) Generate() (string, error) {
	g.n++
	return fmt.Sprintf("%s%d", g.prefix, g.n), nil
}

// TestUnsafeSessionIDIsRevocable tests a session ID carrying the syntax the
// KV key is built from. A revocation watches "{user}.*", which matches one token,
// and an unencoded separator would hide the session from it.
func TestUnsafeSessionIDIsRevocable(t *testing.T) {
	conn := setupNATS(t)
	ctx := context.Background()

	for name, prefix := range map[string]string{
		"separator": "a.b.",
		"star":      "*",
		"gt":        ">",
		"plain":     "plain",
	} {
		t.Run(name, func(t *testing.T) {
			sm, err := natskv.New[testSession](conn, &unsafeGen{prefix: prefix},
				natskv.Config{
					EncryptionKey: validKey(),
					KVConfig:      nats.KeyValueConfig{Bucket: "UNSAFE" + name},
				})
			require.NoError(t, err)

			const count = 3
			cookies := make([]string, 0, count)
			for range count {
				c, err := sm.CreateSession(ctx, sessions.Record[testSession]{
					UserID: "alice",
				})
				require.NoError(t, err)
				cookies = append(cookies, c)
			}

			closed, err := sm.CloseAllUserSessions(ctx, []string{}, "alice")
			require.NoError(t, err)
			require.Len(t, closed, count,
				"closing every session of the user reached %d of %d",
				len(closed), count)

			for _, c := range cookies {
				_, _, ok, err := sm.ReadSessionFromCookie(c)
				require.NoError(t, err)
				require.False(t, ok, "a closed session still authenticates")
			}
		})
	}
}

// TestEmptySessionIDRefused tests the one session ID encoding cannot save.
// An empty ID names no key.
func TestEmptySessionIDRefused(t *testing.T) {
	conn := setupNATS(t)
	sm, err := natskv.New[testSession](conn, &emptyGen{}, natskv.Config{
		EncryptionKey: validKey(),
		KVConfig:      nats.KeyValueConfig{Bucket: "EMPTYSID"},
	})
	require.NoError(t, err)

	_, err = sm.CreateSession(context.Background(),
		sessions.Record[testSession]{UserID: "alice"})
	require.ErrorIs(t, err, natskv.ErrEmptySessionID)
}

// emptyGen returns no session ID at all.
type emptyGen struct{}

func (emptyGen) Generate() (string, error) { return "", nil }

// TestDeleteExpired tests the sweep. NATS expires keys at one age for the whole bucket,
// which is not the ExpiresAt a session carries.
func TestDeleteExpired(t *testing.T) {
	conn := setupNATS(t)
	sm, err := natskv.New[testSession](conn, tokGen, natskv.Config{
		EncryptionKey: validKey(),
		KVConfig:      nats.KeyValueConfig{Bucket: "DELETEEXPIRED"},
	})
	require.NoError(t, err)
	ctx := context.Background()

	past, err := sm.CreateSession(ctx, sessions.Record[testSession]{
		UserID: "alice", ExpiresAt: time.Now().Add(-time.Hour),
	})
	require.NoError(t, err)
	future, err := sm.CreateSession(ctx, sessions.Record[testSession]{
		UserID: "bob", ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	never, err := sm.CreateSession(ctx, sessions.Record[testSession]{
		UserID: "carol",
	})
	require.NoError(t, err)

	deleted, err := sm.DeleteExpired(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, deleted, "the sweep took the wrong number of sessions")

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

	deleted, err = sm.DeleteExpired(ctx)
	require.NoError(t, err)
	require.Zero(t, deleted)
}
