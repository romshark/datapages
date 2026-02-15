package natskv_test

import (
	"context"
	"encoding/base64"
	"maps"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	natsctr "github.com/testcontainers/testcontainers-go/modules/nats"

	"github.com/romshark/datapages/modules/sessmanager/natskv"
	"github.com/romshark/datapages/modules/sesstokgen"
)

type testSession struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

var tokGen = sesstokgen.Generator{Length: sesstokgen.DefaultLength}

func setupNATS(t *testing.T) *nats.Conn {
	t.Helper()
	ctx := context.Background()
	ctr, err := natsctr.Run(ctx, "nats:latest")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ctr.Terminate(ctx)) })

	url, err := ctr.ConnectionString(ctx)
	require.NoError(t, err)
	conn, err := nats.Connect(url)
	require.NoError(t, err)
	t.Cleanup(conn.Close)
	return conn
}

func newManager(
	t *testing.T, conn *nats.Conn, conf natskv.Config,
) *natskv.SessionManager[testSession] {
	t.Helper()
	sm, err := natskv.New[testSession](conn, tokGen, conf)
	require.NoError(t, err)
	return sm
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

func TestSaveSessionInvalidToken(t *testing.T) {
	conn := setupNATS(t)
	sm := newManager(t, conn, natskv.Config{
		EncryptionKey: validKey(),
		KVConfig:      nats.KeyValueConfig{Bucket: "SAVE_BAD"},
	})

	err := sm.SaveSession(context.Background(), "!!!bad!!!", testSession{})
	require.Error(t, err)
}

func TestCreateSession(t *testing.T) {
	conn := setupNATS(t)
	sm := newManager(t, conn, natskv.Config{
		EncryptionKey: validKey(),
		KVConfig:      nats.KeyValueConfig{Bucket: "CREATE"},
	})
	ctx := context.Background()

	t.Run("ok", func(t *testing.T) {
		token, err := sm.CreateSession(ctx, "bob", testSession{Username: "bob", Role: "user"})
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

func TestCreateSessionErrTokenGenerator(t *testing.T) {
	conn := setupNATS(t)
	sm, err := natskv.New[testSession](conn, failingTokGen{}, natskv.Config{
		EncryptionKey: validKey(),
		KVConfig:      nats.KeyValueConfig{Bucket: "CREATE_ERR"},
	})
	require.NoError(t, err)

	_, err = sm.CreateSession(context.Background(), "bob", testSession{})
	require.Error(t, err)
}

type failingTokGen struct{}

func (failingTokGen) Generate() (string, error) {
	return "", errFake
}

var errFake = &fakeError{}

type fakeError struct{}

func (*fakeError) Error() string { return "fake error" }

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
		cookie  *http.Cookie
		wantOK  bool
		wantUID string
	}{
		"nil cookie": {
			cookie: nil, wantOK: false,
		},
		"empty value": {
			cookie: &http.Cookie{Value: ""}, wantOK: false,
		},
		"invalid base64": {
			cookie: &http.Cookie{Value: "!!!bad!!!"}, wantOK: false,
		},
		"wrong encryption key": {
			cookie: &http.Cookie{Value: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
			wantOK: false,
		},
		"stale session": {
			cookie: &http.Cookie{Value: staleTok}, wantOK: false,
		},
		"valid session": {
			cookie: &http.Cookie{Value: token}, wantOK: true, wantUID: "carol",
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

	_, _, _, ok, err := sm.ReadSessionFromCookie(&http.Cookie{Value: token})
	require.NoError(t, err)
	require.False(t, ok)
}

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
				_, _, _, ok, err := sm.ReadSessionFromCookie(&http.Cookie{Value: token})
				require.NoError(t, err)
				require.False(t, ok)
			}
		})
	}
}

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
				require.ElementsMatch(t, wantTokens, result)
			}
			if tc.userID != "" {
				m := maps.Collect(sm.UserSessions(ctx, tc.userID))
				require.Len(t, m, 0)
			}
		})
	}
}

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

func TestIterateAndCloseSessions(t *testing.T) {
	conn := setupNATS(t)
	sm := newManager(t, conn, natskv.Config{
		EncryptionKey: validKey(),
		KVConfig:      nats.KeyValueConfig{Bucket: "ITER_AND_CLOSE"},
	})
	ctx := context.Background()

	want := testSession{Username: "alice", Role: "admin"}
	_, err := sm.CreateSession(ctx, "alice", want)
	require.NoError(t, err)

	// Token from UserSessions must be usable with Session and CloseSession.
	for tok, sess := range sm.UserSessions(ctx, "alice") {
		require.Equal(t, want, sess)

		got, err := sm.Session(ctx, tok)
		require.NoError(t, err)
		require.Equal(t, want, got)

		require.NoError(t, sm.CloseSession(ctx, tok))
	}

	// Session should be gone.
	m := maps.Collect(sm.UserSessions(ctx, "alice"))
	require.Len(t, m, 0)
}

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

	cookie := &http.Cookie{Value: token}
	sess, _, uid, ok, err := smNew.ReadSessionFromCookie(cookie)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "alice", uid)
	require.Equal(t, "admin", sess.Role)
}

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

		// Session still exists — fn must not have been called.
		m := maps.Collect(sm.UserSessions(ctx, userID))
		require.Len(t, m, 1)
		for _, s := range m {
			require.Equal(t, sess, s)
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

		// Barrier: by the time this NATS round-trip completes,
		// the watcher goroutine has seen the delete event.
		_ = maps.Collect(sm.UserSessions(ctx, "alice"))

		require.Equal(t, int32(1), called.Load())
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
