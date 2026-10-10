package app

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// TestValidateRegisterPassword tests that registration refuses a password
// outside 8 to 72 bytes with a message, and that bcrypt hashes every password
// it accepts. A password bcrypt refuses fails registration with a 500.
func TestValidateRegisterPassword(t *testing.T) {
	const (
		tooShort = "Password must be at least 8 characters"
		tooLong  = "Password must be at most 72 bytes"
	)
	for name, tt := range map[string]struct {
		password string
		want     string
	}{
		"shortest":  {password: strings.Repeat("a", 8)},
		"too short": {password: strings.Repeat("a", 7), want: tooShort},
		"longest":   {password: strings.Repeat("a", 72)},
		"too long":  {password: strings.Repeat("a", 73), want: tooLong},
		// 37 characters in 74 bytes.
		"multibyte": {password: strings.Repeat("ü", 37), want: tooLong},
	} {
		t.Run(name, func(t *testing.T) {
			got := validateRegister("Ada", "ada@example.com", tt.password)
			require.Equal(t, tt.want, got, "%d-byte password", len(tt.password))
			if got != "" {
				return
			}
			_, err := bcrypt.GenerateFromPassword([]byte(tt.password), bcrypt.MinCost)
			require.NoError(t, err,
				"bcrypt refuses a %d-byte password that validation accepts",
				len(tt.password))
		})
	}
}
