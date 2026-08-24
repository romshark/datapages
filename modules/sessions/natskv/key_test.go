package natskv

import (
	"crypto/aes"
	"crypto/cipher"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCompositeKeyRoundTrip covers the key a session is stored under and the
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

			token, err := encrypt(aead, key)
			require.NoError(t, err)
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
