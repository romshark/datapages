package datapages_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
)

// TestValidateUserID covers what a user ID has to be. Any byte is accepted:
// one that cannot stand in a subject is escaped on the way in,
// which leaves only an empty ID and one too long to fit a subject to be refused.
func TestValidateUserID(t *testing.T) {
	for name, tt := range map[string]struct {
		userID string
		want   error
	}{
		"plain":        {"alice", nil},
		"email":        {"alice@example.com", nil},
		"dotted":       {"first.last@example.com", nil},
		"unicode":      {"жosé🔥", nil},
		"wildcard":     {"*", nil},
		"gt":           {">", nil},
		"space":        {"a b", nil},
		"percent":      {"100%", nil},
		"at the limit": {strings.Repeat("a", datapages.MaxUserIDEncodedLen), nil},
		"empty":        {"", datapages.ErrUserIDEmpty},
		"one over": {
			strings.Repeat("a", datapages.MaxUserIDEncodedLen+1),
			datapages.ErrUserIDTooLong,
		},
		// Escaping triples every byte, which is what the bound is measured on.
		"long once escaped": {
			strings.Repeat(".", datapages.MaxUserIDEncodedLen/3+1),
			datapages.ErrUserIDTooLong,
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := datapages.ValidateUserID(tt.userID)
			if tt.want == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.want)
		})
	}
}

// TestValidateUserIDDoesNotAllocate covers the cost of calling it per request.
func TestValidateUserIDDoesNotAllocate(t *testing.T) {
	n := testing.AllocsPerRun(100, func() {
		_ = datapages.ValidateUserID("first.last@example.com")
	})
	require.Zero(t, n)
}
