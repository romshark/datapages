// Package csrf defines the interface for CSRF token generation and validation.
package csrf

// TokenManager generates and validates CSRF tokens.
//
// Implementations must be safe for concurrent use.
type TokenManager interface {
	// GenerateToken returns a CSRF token bound to the given userID
	// and session issuance time (unix seconds).
	// Returns an empty string if sessIssuedAtUnix is negative.
	GenerateToken(userID string, sessIssuedAtUnix int64) string

	// ValidateToken checks whether token is valid for the given
	// userID and session issuance time (unix seconds).
	ValidateToken(userID string, sessIssuedAtUnix int64, token string) bool
}
