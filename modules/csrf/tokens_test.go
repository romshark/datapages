package csrf_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/modules/csrf"
)

const sessionToken = "yLZ6q0Nn7wQ0m0hE1s0DQZ0N1r6C9mJ8gS3lM7pX2tA"

// generate returns what [csrf.Tokens.WriteToken] writes for sessionToken.
func generate(t *testing.T, sessionToken string) string {
	t.Helper()
	var b strings.Builder
	n, err := csrf.Tokens{}.WriteToken(&b, sessionToken)
	require.NoError(t, err)
	require.Equal(t, n, b.Len(), "n must be what was written")
	return b.String()
}

// TestGenerateValidate tests the round trip:
// a token written for a session validates against that session.
func TestGenerateValidate(t *testing.T) {
	var tokens csrf.Tokens

	token := generate(t, sessionToken)
	require.NotEmpty(t, token)
	require.True(t, tokens.ValidateToken(sessionToken, token))
}

// TestGenerateMasksEveryToken tests the per-render mask. Two tokens for one
// session differ on the wire and both validate, which is what keeps a BREACH
// attacker from learning the token from compressed responses.
func TestGenerateMasksEveryToken(t *testing.T) {
	var tokens csrf.Tokens

	first := generate(t, sessionToken)
	second := generate(t, sessionToken)
	require.NotEqual(t, first, second,
		"the value on the wire must differ on every render")
	require.True(t, tokens.ValidateToken(sessionToken, first))
	require.True(t, tokens.ValidateToken(sessionToken, second))
}

// TestGenerateNeverLeaksTheSessionToken tests that the token the page carries
// reveals nothing of the session token it is derived from.
func TestGenerateNeverLeaksTheSessionToken(t *testing.T) {
	require.NotContains(t, generate(t, sessionToken), sessionToken)
}

// TestGenerateGuest tests a request without a session. There is nothing to
// derive from: nothing is written and the page carries no token.
func TestGenerateGuest(t *testing.T) {
	require.Empty(t, generate(t, ""),
		"a guest has no session token to derive from")
}

// TestValidate tests every token validation must refuse: one from another session,
// one for a guest, and one damaged in length, alphabet or a single byte.
func TestValidate(t *testing.T) {
	var tokens csrf.Tokens
	valid := generate(t, sessionToken)

	for name, tc := range map[string]struct {
		sessionToken string
		token        string
	}{
		"another session": {"5nQ2wT8xR1bV4kL9pD6zY3jH7mC0fG5sN8aE2uI4oW1", valid},
		"guest":           {"", valid},
		"empty token":     {sessionToken, ""},
		"too short":       {sessionToken, valid[:len(valid)-1]},
		"too long":        {sessionToken, valid + "A"},
		"not base64":      {sessionToken, "!" + valid[1:]},
		"flipped byte":    {sessionToken, flipFirst(valid)},
	} {
		t.Run(name, func(t *testing.T) {
			require.False(t, tokens.ValidateToken(tc.sessionToken, tc.token))
		})
	}
}

// flipFirst replaces the first character of token, which changes the mask it
// carries and leaves the length alone.
//
// The last character is the wrong one to change: it carries mostly bits the
// encoding doesn't use, which lets it decode to the same bytes and stay valid.
func flipFirst(token string) string {
	first := token[0]
	replacement := byte('A')
	if first == replacement {
		replacement = 'B'
	}
	return string(replacement) + token[1:]
}
