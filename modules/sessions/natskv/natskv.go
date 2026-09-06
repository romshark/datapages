// Package natskv provides a built-in implementation
// of the SessionManager based on the NATS Key-Value Store.
//
// Sessions are stored in NATS KV with composite keys
// ({encodedUserID}.{encodedSessionID}) to enable efficient per-user prefix lookups.
// The cookie value is the composite key encrypted with AES-128-GCM,
// such that the userID is never exposed to the client.
//
// The bucket holds the session data and nothing the client sends:
// a token is rebuilt from the key when one is asked for,
// so read access to the bucket does not yield a working cookie.
package natskv

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/romshark/datapages/modules/sessions"
)

var (
	_ sessions.UserSessionIterator[struct{}] = (*SessionManager[struct{}])(nil)
	_ sessions.UserSessionCloser             = (*SessionManager[struct{}])(nil)
)

// DefaultBucket is the default bucket name for the NATS KV Store based session manager.
const DefaultBucket = "SESSIONS"

// SessionTokenGenerator generates cryptographically secure unique session tokens.
// A token may carry any character: it is encoded into the KV key.
type SessionTokenGenerator interface {
	Generate() (string, error)
}

var (
	ErrEmptyUserID     = sessions.ErrEmptyUserID
	ErrEmptySessionID  = sessions.ErrEmptyToken
	ErrSessionNotFound = sessions.ErrSessionNotFound

	ErrEncryptionKeyLen        = errors.New("encryption key must be exactly 16 bytes")
	ErrCiphertextTooShort      = errors.New("ciphertext too short")
	ErrMalformedCompositeKey   = errors.New("malformed composite key")
	ErrAllDecryptionKeysFailed = errors.New("all keys failed")

	// ErrUserIDMismatch is returned when a saved record names another user
	// than the key it is stored under, which decides who a session belongs to.
	ErrUserIDMismatch = errors.New("record user ID contradicts the session key")
)

// New creates a new NATS Key-Value store backed session manager.
func New[Data any](
	conn *nats.Conn,
	sessionTokenGenerator SessionTokenGenerator,
	conf Config,
) (*SessionManager[Data], error) {
	js, err := conn.JetStream()
	if err != nil {
		return nil, fmt.Errorf("creating JetStream context: %w", err)
	}

	keys := make([][]byte, 0, 1+len(conf.PreviousEncryptionKeys))
	keys = append(keys, conf.EncryptionKey)
	keys = append(keys, conf.PreviousEncryptionKeys...)

	aeads := make([]cipher.AEAD, len(keys))
	for i, key := range keys {
		if len(key) != 16 {
			return nil, ErrEncryptionKeyLen
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, fmt.Errorf("creating AES cipher: %w", err)
		}
		aeads[i], err = cipher.NewGCM(block)
		if err != nil {
			return nil, fmt.Errorf("creating GCM: %w", err)
		}
	}

	kvConfig := conf.KVConfig
	if kvConfig.Bucket == "" {
		kvConfig.Bucket = DefaultBucket
	}

	// Try to get existing bucket first, create if not found.
	// This avoids relying on specific error types from
	// CreateKeyValue which can vary across NATS versions.
	kv, err := js.KeyValue(kvConfig.Bucket)
	switch {
	case errors.Is(err, nats.ErrBucketNotFound):
		kv, err = js.CreateKeyValue(&kvConfig)
		if err != nil {
			return nil, fmt.Errorf("creating new KV bucket: %w", err)
		}
	case err != nil:
		return nil, fmt.Errorf("opening KV bucket: %w", err)
	}

	return &SessionManager[Data]{
		conf:                  conf,
		kv:                    kv,
		aeads:                 aeads,
		sessionTokenGenerator: sessionTokenGenerator,
	}, nil
}

// Config configures the session manager.
type Config struct {
	// EncryptionKey is the 16-byte AES-128 key used to
	// encrypt session tokens stored in cookies. Required.
	EncryptionKey []byte

	// PreviousEncryptionKeys is a list of previous 16-byte AES-128 keys used only for
	// decrypting existing cookies during key rotation.
	// New cookies are always encrypted with EncryptionKey.
	PreviousEncryptionKeys [][]byte

	KVConfig nats.KeyValueConfig
}

// SessionManager manages sessions backed by NATS KV.
type SessionManager[Data any] struct {
	conf                  Config
	kv                    nats.KeyValue
	aeads                 []cipher.AEAD // [0] is primary
	sessionTokenGenerator SessionTokenGenerator
}

// kvRecord wraps session data for storage.
//
// It deliberately holds no token: the token is the client's credential, and
// the encrypted key it is built from can be rebuilt from the key the record is
// stored under. Keeping it here would make read access to the bucket enough to
// impersonate every live session.
type kvRecord struct {
	Data json.RawMessage `json:"data"`
}

// ReadSessionFromCookie decrypts the cookie value to
// recover the composite KV key and retrieves the session.
// Returns ok=false, err=nil if the value is empty, malformed,
// or the session is not found (caller should remove the cookie).
// Returns ok=false, err!=nil on transient backend failures
// (caller should keep the cookie and fail the request).
func (s *SessionManager[Data]) ReadSessionFromCookie(
	cookieValue string,
) (rec sessions.Record[Data], token string, ok bool, err error) {
	if cookieValue == "" {
		return rec, "", false, nil
	}

	kvKey, err := decrypt(s.aeads, cookieValue)
	if err != nil {
		return rec, "", false, nil
	}

	uid, err := parseCompositeKeyUserID(kvKey)
	if err != nil {
		return rec, "", false, nil
	}

	entry, err := s.kv.Get(kvKey)
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return rec, "", false, nil
		}
		return rec, "", false, fmt.Errorf("reading session from KV: %w", err)
	}

	var kvRec kvRecord
	if err := json.Unmarshal(entry.Value(), &kvRec); err != nil {
		return rec, "", false, nil
	}
	if err := json.Unmarshal(kvRec.Data, &rec); err != nil {
		return rec, "", false, nil
	}
	// The user id in the key is authoritative.
	rec.UserID = uid

	return rec, cookieValue, true, nil
}

// NotifyClosed watches for deletion of the session
// identified by the encrypted token and calls fn.
// If the session is already deleted, fn is called immediately.
func (s *SessionManager[Data]) NotifyClosed(
	ctx context.Context, token string, fn func(),
) error {
	kvKey, err := decrypt(s.aeads, token)
	if err != nil {
		// A token that doesn't decrypt names no session, so that session is
		// closed as far as the caller is concerned. inmem answers the same,
		// and a stream whose token no longer decrypts otherwise fails to open
		// here where it opens there.
		fn()
		return nil
	}

	// Already deleted: notify immediately. Fall through to Watch on other errors.
	if _, err := s.kv.Get(kvKey); errors.Is(err, nats.ErrKeyNotFound) {
		fn()
		return nil
	}

	watcher, err := s.kv.Watch(kvKey, nats.Context(ctx))
	if err != nil {
		return fmt.Errorf("setting up watcher: %w", err)
	}

	go func() {
		defer func() { _ = watcher.Stop() }()

		updates := watcher.Updates()
		for {
			select {
			case <-ctx.Done():
				return
			case entry, open := <-updates:
				if !open {
					// The subscription ended, which happens when the NATS
					// connection is lost. No delete event can arrive anymore and
					// whether the session still exists is unknowable from here.
					return
				}
				if entry == nil {
					// Initial replay ended without a delete event. Re-check in case
					// the key was deleted between our Get and Watch setup.
					_, err := s.kv.Get(kvKey)
					if errors.Is(err, nats.ErrKeyNotFound) {
						fn()
						return
					}
					continue
				}
				op := entry.Operation()
				if op == nats.KeyValueDelete || op == nats.KeyValuePurge {
					fn()
					return
				}
			}
		}
	}()
	return nil
}

// SaveSession overwrites the session data for an existing token.
// The record must name the user the token belongs to.
func (s *SessionManager[Data]) SaveSession(
	_ context.Context, token string, rec sessions.Record[Data],
) error {
	kvKey, err := decrypt(s.aeads, token)
	if err != nil {
		return fmt.Errorf("decrypting token: %w", err)
	}
	uid, err := parseCompositeKeyUserID(kvKey)
	if err != nil {
		return fmt.Errorf("parsing composite key: %w", err)
	}
	if rec.UserID != uid {
		return fmt.Errorf("%w: %q under %q", ErrUserIDMismatch, rec.UserID, uid)
	}
	return s.putSession(kvKey, rec)
}

// putSession stores rec under kvKey.
func (s *SessionManager[Data]) putSession(
	kvKey string, rec sessions.Record[Data],
) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshaling session data: %w", err)
	}

	kvRec, err := json.Marshal(kvRecord{Data: data})
	if err != nil {
		return fmt.Errorf("marshaling KV record: %w", err)
	}

	if _, err := s.kv.Put(kvKey, kvRec); err != nil {
		return fmt.Errorf("storing session in KV: %w", err)
	}
	return nil
}

// CreateSession creates a new session in NATS KV.
// Returns an encrypted token suitable for use as a cookie value.
func (s *SessionManager[Data]) CreateSession(
	ctx context.Context, rec sessions.Record[Data],
) (token string, err error) {
	if rec.UserID == "" {
		return "", ErrEmptyUserID
	}

	uniqueSessionID, err := s.sessionTokenGenerator.Generate()
	if err != nil {
		return "", err
	}
	if uniqueSessionID == "" {
		return "", ErrEmptySessionID
	}

	kvKey := compositeKey(rec.UserID, uniqueSessionID)
	token, err = encrypt(s.aeads[0], kvKey)
	if err != nil {
		return "", fmt.Errorf("encrypting session token: %w", err)
	}

	if err := s.putSession(string(kvKey), rec); err != nil {
		return "", err
	}
	return token, nil
}

// CloseSession deletes a session from NATS KV.
// No-op and no error if the session doesn't exist.
func (s *SessionManager[Data]) CloseSession(
	_ context.Context, token string,
) error {
	kvKey, err := decrypt(s.aeads, token)
	if err != nil {
		// A token that doesn't decrypt names no session, which is the no-op case
		// [github.com/romshark/datapages/modules/sessions.Closer] documents.
		// Reporting it answers 500 to a guest POST to a sign-out action and to
		// a cookie left over from a rotated key.
		return nil
	}
	// Delete publishes a tombstone and never returns ErrKeyNotFound,
	// so this is inherently a no-op for non-existent keys.
	if err := s.kv.Delete(kvKey); err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}
	return nil
}

// CloseAllUserSessions closes all sessions for a user.
// Only sees sessions that exist at call time;
// sessions created during iteration are not closed.
// If buffer is non-nil, appends encrypted tokens of closed sessions to it.
func (s *SessionManager[Data]) CloseAllUserSessions(
	ctx context.Context, buffer []string, userID string,
) ([]string, error) {
	if userID == "" {
		return buffer, ErrEmptyUserID
	}
	prefix := userKeyPattern(userID)
	// The token is rebuilt from the key, so the payload is never read here.
	opts := []nats.WatchOpt{
		nats.IgnoreDeletes(), nats.Context(ctx), nats.MetaOnly(),
	}
	watcher, err := s.kv.Watch(prefix, opts...)
	if err != nil {
		return buffer, fmt.Errorf("watching user sessions: %w", err)
	}
	defer func() { _ = watcher.Stop() }()

	var errs []error
	for entry := range watcher.Updates() {
		if entry == nil {
			break
		}
		kvKey := entry.Key()
		if err := s.kv.Delete(kvKey); err != nil {
			errs = append(errs, fmt.Errorf("deleting session %q: %w", kvKey, err))
			continue
		}
		if buffer != nil {
			token, err := encrypt(s.aeads[0], []byte(kvKey))
			if err != nil {
				errs = append(errs, fmt.Errorf(
					"encrypting token for session %q: %w", kvKey, err,
				))
				continue
			}
			buffer = append(buffer, token)
		}
	}

	return buffer, errors.Join(errs...)
}

// Session retrieves a session record by its encrypted token.
func (s *SessionManager[Data]) Session(
	_ context.Context, token string,
) (rec sessions.Record[Data], err error) {
	kvKey, err := decrypt(s.aeads, token)
	if err != nil {
		return rec, fmt.Errorf("decrypting session token: %w", err)
	}
	uid, err := parseCompositeKeyUserID(kvKey)
	if err != nil {
		return rec, fmt.Errorf("parsing composite key: %w", err)
	}

	entry, err := s.kv.Get(kvKey)
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return rec, ErrSessionNotFound
		}
		return rec, fmt.Errorf("getting session: %w", err)
	}

	var kvRec kvRecord
	if err := json.Unmarshal(entry.Value(), &kvRec); err != nil {
		return rec, fmt.Errorf("unmarshaling KV record: %w", err)
	}
	if err := json.Unmarshal(kvRec.Data, &rec); err != nil {
		return rec, fmt.Errorf("unmarshaling session data: %w", err)
	}
	// The user id in the key is authoritative,
	// the same way ReadSessionFromCookie reads it.
	rec.UserID = uid

	return rec, nil
}

// UserSessions returns an iterator over all current
// sessions for a given user (snapshot, not streaming).
// Yields (token, session) pairs where token is the encrypted
// session token usable with CloseSession, Session, and NotifyClosed.
//
// The watch is set up before the iterator so that an unreachable store is an
// error rather than a user with no sessions.
func (s *SessionManager[Data]) UserSessions(
	ctx context.Context, userID string,
) (iter.Seq2[string, sessions.Record[Data]], error) {
	if userID == "" {
		return func(func(string, sessions.Record[Data]) bool) {}, nil
	}
	prefix := userKeyPattern(userID)
	watcher, err := s.kv.Watch(prefix, nats.IgnoreDeletes(), nats.Context(ctx))
	if err != nil {
		return nil, fmt.Errorf("watching user sessions: %w", err)
	}

	return func(yield func(string, sessions.Record[Data]) bool) {
		defer func() { _ = watcher.Stop() }()

		for entry := range watcher.Updates() {
			if entry == nil {
				break
			}

			var kvRec kvRecord
			if err := json.Unmarshal(entry.Value(), &kvRec); err != nil {
				continue
			}

			var rec sessions.Record[Data]
			if err := json.Unmarshal(kvRec.Data, &rec); err != nil {
				continue
			}
			// The prefix this scan runs over is the user, so the payload
			// cannot name another one without the list contradicting itself.
			rec.UserID = userID

			// A fresh nonce per call gives a different ciphertext that
			// decrypts back to the same key, which is all a token is.
			token, err := encrypt(s.aeads[0], []byte(entry.Key()))
			if err != nil {
				continue
			}

			if !yield(token, rec) {
				return
			}
		}
	}, nil
}

// keyEncoding has no '.', '*' or '>' in its alphabet. A '.' in either half of
// a key would put it below the "{user}.*" pattern a revocation watches.
var keyEncoding = base64.RawURLEncoding

// compositeKey builds the NATS KV key:
// {base64url(userID)}.{base64url(sessionID)}.
//
// It returns the bytes it built. [encrypt] reads them and the KV store wants a string,
// which is one conversion either way.
func compositeKey(userID, sessionID string) []byte {
	b := make([]byte, 0, keyEncoding.EncodedLen(len(userID))+
		len(".")+keyEncoding.EncodedLen(len(sessionID)))
	b = keyEncoding.AppendEncode(b, []byte(userID))
	b = append(b, '.')
	return keyEncoding.AppendEncode(b, []byte(sessionID))
}

// userKeyPattern matches every key a session of userID is stored under.
func userKeyPattern(userID string) string {
	b := make([]byte, 0, keyEncoding.EncodedLen(len(userID))+len(".*"))
	b = keyEncoding.AppendEncode(b, []byte(userID))
	return string(append(b, ".*"...))
}

// parseCompositeKeyUserID extracts and decodes the userID from a composite KV key.
func parseCompositeKeyUserID(kvKey string) (string, error) {
	encoded, sid, ok := strings.Cut(kvKey, ".")
	if !ok || encoded == "" || sid == "" {
		return "", ErrMalformedCompositeKey
	}
	uid, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decoding userID from key: %w", err)
	}
	return string(uid), nil
}

// encrypt encrypts plaintext using AES-128-GCM and returns a base64url-encoded string.
func encrypt(aead cipher.AEAD, plaintext []byte) (string, error) {
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}
	ciphertext := aead.Seal(nonce, nonce, plaintext, nil)
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

// decrypt decodes a base64url string and decrypts it using AES-128-GCM,
// trying each AEAD in order (supports key rotation).
// aeads[0] is the primary key, subsequent entries are previous keys.
func decrypt(aeads []cipher.AEAD, encrypted string) (string, error) {
	data, err := base64.RawURLEncoding.DecodeString(encrypted)
	if err != nil {
		return "", fmt.Errorf("decoding base64: %w", err)
	}

	for _, aead := range aeads {
		nonceSize := aead.NonceSize()
		if len(data) < nonceSize {
			return "", ErrCiphertextTooShort
		}
		pt, err := aead.Open(nil, data[:nonceSize], data[nonceSize:], nil)
		if err == nil {
			return string(pt), nil
		}
	}
	return "", ErrAllDecryptionKeysFailed
}

// DeleteExpired deletes every session whose ExpiresAt has passed.
// It reads the bucket key by key: the expiry is inside each record,
// and a bucket TTL would be one age for every key.
func (s *SessionManager[Data]) DeleteExpired(ctx context.Context) (int, error) {
	watcher, err := s.kv.WatchAll(nats.IgnoreDeletes(), nats.Context(ctx))
	if err != nil {
		return 0, fmt.Errorf("watching sessions: %w", err)
	}
	defer func() { _ = watcher.Stop() }()

	now := time.Now()
	deleted := 0
	var errs []error
	for entry := range watcher.Updates() {
		if entry == nil {
			break // The replay of what the bucket holds ended.
		}

		var kvRec kvRecord
		if err := json.Unmarshal(entry.Value(), &kvRec); err != nil {
			continue
		}
		var rec sessions.Record[Data]
		if err := json.Unmarshal(kvRec.Data, &rec); err != nil {
			continue
		}
		if rec.ExpiresAt.IsZero() || now.Before(rec.ExpiresAt) {
			continue
		}

		if err := s.kv.Delete(entry.Key()); err != nil {
			errs = append(errs,
				fmt.Errorf("deleting session %q: %w", entry.Key(), err))
			continue
		}
		deleted++
	}
	return deleted, errors.Join(errs...)
}
