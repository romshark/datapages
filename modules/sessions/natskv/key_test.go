package natskv

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCompositeKeyRoundTrip tests the key a session is stored under and the
// token that carries it. CreateSession keeps the key it built,
// SaveSession recovers it from the token, and both must name one session.
func TestCompositeKeyRoundTrip(t *testing.T) {
	block, err := aes.NewCipher(make([]byte, 16))
	require.NoError(t, err)
	aead, err := cipher.NewGCM(block)
	require.NoError(t, err)
	aeads := []cipher.AEAD{aead}

	for name, tt := range map[string]struct{ userID, sessionID string }{
		"plain":            {"alice", "s1"},
		"email":            {"alice@example.com", "s1"},
		"dotted session":   {"alice", "a.b.c"},
		"wildcard session": {"alice", "*"},
		"gt session":       {"alice", ">"},
		"dotted user":      {"first.last@example.com", "s1"},
		"unicode":          {"josé", "sé"},
	} {
		t.Run(name, func(t *testing.T) {
			key := compositeKey(tt.userID, tt.sessionID)

			require.Equal(t, 1, strings.Count(string(key), "."),
				"a key has one separator: %q", key)
			require.NotContains(t, string(key), "*")
			require.NotContains(t, string(key), ">")

			uid, err := parseCompositeKeyUserID(string(key))
			require.NoError(t, err)
			require.Equal(t, tt.userID, uid, "the user ID did not survive the key")

			token := encrypt(aead, make([]byte, 32), key)
			back, err := decrypt(aeads, token)
			require.NoError(t, err)
			require.Equal(t, string(key), back,
				"the key CreateSession keeps and the one SaveSession recovers differ")

			require.True(t, strings.HasPrefix(string(key),
				strings.TrimSuffix(userKeyPattern(tt.userID), "*")),
				"the key is not under the pattern a revocation watches")
		})
	}
}

// TestTokenIsStable tests the token a session key encrypts to. Every call gives
// the same one, which lets a listed session be compared with its cookie,
// and two keys get two nonces: GCM leaks its authentication key when one nonce
// encrypts two plaintexts. A token with a random nonce, the format of the
// cookies of earlier releases, still decrypts.
func TestTokenIsStable(t *testing.T) {
	block, err := aes.NewCipher(make([]byte, 16))
	require.NoError(t, err)
	aead, err := cipher.NewGCM(block)
	require.NoError(t, err)
	nonceKey := []byte("0123456789abcdef0123456789abcdef")
	first := compositeKey("alice", "s1")
	second := compositeKey("alice", "s2")

	require.Equal(t, encrypt(aead, nonceKey, first), encrypt(aead, nonceKey, first))

	// Comparing whole tokens would pass with one nonce for every key:
	// the ciphertexts differ with the plaintexts.
	firstRaw, err := base64.RawURLEncoding.DecodeString(encrypt(aead, nonceKey, first))
	require.NoError(t, err)
	secondRaw, err := base64.RawURLEncoding.DecodeString(encrypt(aead, nonceKey, second))
	require.NoError(t, err)
	n := aead.NonceSize()
	require.NotEqual(t, firstRaw[:n], secondRaw[:n], "two session keys share a nonce")

	nonce := make([]byte, aead.NonceSize())
	_, err = rand.Read(nonce)
	require.NoError(t, err)
	legacy := base64.RawURLEncoding.EncodeToString(aead.Seal(nonce, nonce, first, nil))
	back, err := decrypt([]cipher.AEAD{aead}, legacy)
	require.NoError(t, err)
	require.Equal(t, string(first), back)
}
