package csrf_test

import (
	"io"
	"strings"
	"testing"

	"github.com/romshark/datapages/modules/csrf"
)

var (
	GI int
	GB bool
)

// BenchmarkWriteToken measures the token every rendered page pays for,
// with and without a session.
func BenchmarkWriteToken(b *testing.B) {
	var tokens csrf.Tokens

	b.Run("session", func(b *testing.B) {
		for b.Loop() {
			GI, _ = tokens.WriteToken(io.Discard, sessionToken)
		}
	})
	b.Run("guest", func(b *testing.B) {
		for b.Loop() {
			GI, _ = tokens.WriteToken(io.Discard, "")
		}
	})
}

// BenchmarkValidateToken measures the check every action request pays for,
// on a valid token and on each way one can be invalid: the early exits must not
// be the only fast paths.
func BenchmarkValidateToken(b *testing.B) {
	var tokens csrf.Tokens
	valid := generateBench(b)

	for name, tc := range map[string]struct {
		sessionToken string
		token        string
	}{
		"valid":        {sessionToken, valid},
		"flipped byte": {sessionToken, flipFirst(valid)},
		"not base64":   {sessionToken, "!" + valid[1:]},
		"wrong length": {sessionToken, valid[:len(valid)-1]},
		"guest":        {"", valid},
	} {
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				GB = tokens.ValidateToken(tc.sessionToken, tc.token)
			}
		})
	}
}

// generateBench returns a valid token for [sessionToken].
func generateBench(b *testing.B) string {
	b.Helper()
	var s strings.Builder
	if _, err := (csrf.Tokens{}).WriteToken(&s, sessionToken); err != nil {
		b.Fatal(err)
	}
	return s.String()
}
