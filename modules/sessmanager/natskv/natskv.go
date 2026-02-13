// Package natskv provides a built-in implementation
// of the SessionManager based on the NATS Key-Value Store.
//
// Sessions are stored in NATS KV with composite keys
// ({encodedUserID}.{uniqueSessionID}) to enable efficient per-user prefix lookups.
// The cookie value is the composite key encrypted with AES-128-GCM,
// such that the userID is never exposed to the client.
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
	"net/http"
	"strings"

	"github.com/nats-io/nats.go"
)

// DefaultBucket is the default bucket name for the NATS KV Store based session manager.
const DefaultBucket = "SESSIONS"

// SessionTokenGenerator generates cryptographically secure unique session tokens.
// Tokens must not contain NATS subject characters ('.', '*', '>').
type SessionTokenGenerator interface {
	Generate() (string, error)
}

var (
	ErrUnsafeSessionID = errors.New("uniqueSessionID contains NATS-unsafe characters")

	ErrEncryptionKeyLen      = errors.New("encryption key must be exactly 16 bytes")
	ErrEmptyUserID           = errors.New("userID must not be empty")
	ErrEmptySessionID        = errors.New("uniqueSessionID must not be empty")
	ErrCiphertextTooShort    = errors.New("ciphertext too short")
	ErrMalformedCompositeKey = errors.New("malformed composite key")

	// ErrSessionNotFound is returned when a session is not found in the KV store.
	ErrSessionNotFound = errors.New("session not found")

	ErrAllDecryptionKeysFailed = errors.New("all keys failed")
)

// New creates a new NATS Key-Value store backed session manager.
func New[S any](
	conn *nats.Conn,
	sessionTokenGenerator SessionTokenGenerator,
	conf Config,
) (*SessionManager[S], error) {
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

	return &SessionManager[S]{
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
	// decrypting existing cookies during key rotation. New cookies are always encrypted
	// with EncryptionKey.
	PreviousEncryptionKeys [][]byte

	KVConfig nats.KeyValueConfig
}

// SessionManager manages sessions backed by NATS KV.
type SessionManager[S any] struct {
	conf                  Config
	kv                    nats.KeyValue
	aeads                 []cipher.AEAD // [0] is primary
	sessionTokenGenerator SessionTokenGenerator
}

// ReadSessionFromCookie decrypts the cookie value to
// recover the composite KV key and retrieves the session.
// Returns ok=false, err=nil if the cookie is nil, empty,
// malformed, or the session is not found (caller should
// remove the cookie). Returns ok=false, err!=nil on
// transient backend failures (caller should keep the
// cookie and fail the request).
func (s *SessionManager[S]) ReadSessionFromCookie(
	c *http.Cookie,
) (session S, token, userID string, ok bool, err error) {
	if c == nil || c.Value == "" {
		return session, "", "", false, nil
	}

	kvKey, err := decrypt(s.aeads, c.Value)
	if err != nil {
		return session, "", "", false, nil
	}

	uid, err := parseCompositeKeyUserID(kvKey)
	if err != nil {
		return session, "", "", false, nil
	}

	entry, err := s.kv.Get(kvKey)
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return session, "", "", false, nil
		}
		return session, "", "", false, fmt.Errorf("reading session from KV: %w", err)
	}

	if err := json.Unmarshal(entry.Value(), &session); err != nil {
		return session, "", "", false, nil
	}

	return session, c.Value, uid, true, nil
}

// NotifyClosed watches for deletion of the session
// identified by the encrypted token and calls fn.
// If the session is already deleted, fn is called
// immediately.
func (s *SessionManager[S]) NotifyClosed(
	ctx context.Context, token string, fn func(),
) error {
	kvKey, err := decrypt(s.aeads, token)
	if err != nil {
		return fmt.Errorf("decrypting token: %w", err)
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

		for {
			select {
			case <-ctx.Done():
				return
			case entry := <-watcher.Updates():
				if entry == nil {
					// Initial replay ended without a delete event.  Re-check in case
					// the key was deleted between our Get and Watch setup.
					_, err := s.kv.Get(kvKey)
					if errors.Is(err, nats.ErrKeyNotFound) {
						fn()
					}
					return
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

// SaveSession stores a session in NATS KV under the
// composite key {encodedUserID}.{uniqueSessionID}.
// Returns the encrypted token for use as a cookie value.
func (s *SessionManager[S]) SaveSession(
	_ context.Context, userID, uniqueSessionID string, session S,
) (token string, err error) {
	switch {
	case userID == "":
		return "", ErrEmptyUserID
	case uniqueSessionID == "":
		return "", ErrEmptySessionID
	case strings.ContainsAny(uniqueSessionID, ".*>"):
		return "", ErrUnsafeSessionID
	}

	payload, err := json.Marshal(session)
	if err != nil {
		return "", fmt.Errorf("marshaling session data JSON: %w", err)
	}

	kvKey := compositeKey(userID, uniqueSessionID)
	if _, err := s.kv.Put(kvKey, payload); err != nil {
		return "", fmt.Errorf("storing session in KV: %w", err)
	}

	token, err = encrypt(s.aeads[0], kvKey)
	if err != nil {
		return "", fmt.Errorf("encrypting session token: %w", err)
	}
	return token, nil
}

// CreateSession creates a new session in NATS KV.
// Returns an encrypted token suitable for use as a
// cookie value.
func (s *SessionManager[S]) CreateSession(
	ctx context.Context, userID string, session S,
) (token string, err error) {
	uniqueSessionID, err := s.sessionTokenGenerator.Generate()
	if err != nil {
		return "", err
	}
	return s.SaveSession(ctx, userID, uniqueSessionID, session)
}

// CloseSession deletes a session from NATS KV.
// No-op and no error if the session doesn't exist.
func (s *SessionManager[S]) CloseSession(
	_ context.Context, token string,
) error {
	kvKey, err := decrypt(s.aeads, token)
	if err != nil {
		return fmt.Errorf("decrypting session token: %w", err)
	}
	if err := s.kv.Delete(kvKey); err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return nil
		}
		return fmt.Errorf("deleting session: %w", err)
	}
	return nil
}

// CloseAllUserSessions closes all sessions for a user.
// Only sees sessions that exist at call time;
// sessions created during iteration are not closed.
// If buffer is non-nil, appends encrypted tokens of closed sessions to it.
func (s *SessionManager[S]) CloseAllUserSessions(
	ctx context.Context, buffer []string, userID string,
) ([]string, error) {
	if userID == "" {
		return buffer, ErrEmptyUserID
	}
	prefix := encodeUserID(userID) + ".*"
	watcher, err := s.kv.Watch(prefix,
		nats.IgnoreDeletes(), nats.MetaOnly(), nats.Context(ctx))
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
			if !errors.Is(err, nats.ErrKeyNotFound) {
				errs = append(errs, fmt.Errorf("deleting session %q: %w", kvKey, err))
			}
			continue
		}
		encrypted, err := encrypt(s.aeads[0], kvKey)
		if err != nil {
			errs = append(errs, fmt.Errorf("encrypting token for %q: %w", kvKey, err))
			continue
		}
		if buffer != nil {
			buffer = append(buffer, encrypted)
		}
	}

	return buffer, errors.Join(errs...)
}

// Session retrieves a session by its encrypted token.
func (s *SessionManager[S]) Session(
	_ context.Context, token string,
) (session S, err error) {
	kvKey, err := decrypt(s.aeads, token)
	if err != nil {
		return session, fmt.Errorf("decrypting session token: %w", err)
	}

	entry, err := s.kv.Get(kvKey)
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return session, ErrSessionNotFound
		}
		return session, fmt.Errorf("getting session: %w", err)
	}

	if err := json.Unmarshal(entry.Value(), &session); err != nil {
		return session, fmt.Errorf("unmarshaling session data JSON: %w", err)
	}

	return session, nil
}

// UserSessions returns an iterator over all current
// sessions for a given user (snapshot, not streaming).
// Yields (encryptedToken, session) pairs.
func (s *SessionManager[S]) UserSessions(
	ctx context.Context, userID string,
) iter.Seq2[string, S] {
	return func(yield func(string, S) bool) {
		if userID == "" {
			return
		}
		prefix := encodeUserID(userID) + ".*"
		watcher, err := s.kv.Watch(prefix, nats.IgnoreDeletes(), nats.Context(ctx))
		if err != nil {
			return
		}
		defer func() { _ = watcher.Stop() }()

		for entry := range watcher.Updates() {
			if entry == nil {
				break
			}

			var session S
			if err := json.Unmarshal(entry.Value(), &session); err != nil {
				continue
			}

			encrypted, err := encrypt(s.aeads[0], entry.Key())
			if err != nil {
				continue
			}

			if !yield(encrypted, session) {
				return
			}
		}
	}
}

// encodeUserID encodes a userID into a base64url string
// safe for use in NATS KV keys and subject patterns.
func encodeUserID(userID string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(userID))
}

// compositeKey builds the NATS KV key: {base64url(userID)}.{token}.
func compositeKey(userID, token string) string {
	encoded := encodeUserID(userID)
	var b strings.Builder
	b.Grow(len(encoded) + len(".") + len(token))
	b.WriteString(encoded)
	b.WriteByte('.')
	b.WriteString(token)
	return b.String()
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
func encrypt(aead cipher.AEAD, plaintext string) (string, error) {
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}
	ciphertext := aead.Seal(nonce, nonce, []byte(plaintext), nil)
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
