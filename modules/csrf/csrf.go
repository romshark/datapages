// Package csrf defines the interface for
// CSRF (Cross-Site Request Forgery) token generation and validation.
//
// The built-in implementations live in the subpackages.
package csrf

type UnixSeconds = int64

// TokenGenerator issues CSRF tokens.
//
// Implementations are safe for concurrent use.
type TokenGenerator interface {
	// GenerateToken returns a CSRF token bound to the given userID
	// and session issuance time (unix seconds).
	// Returns an empty string if sessIssuedAtUnix is negative.
	GenerateToken(userID string, sessIssuedAtUnix UnixSeconds) string
}

// TokenValidator checks CSRF tokens against the session they belong to.
//
// Implementations are safe for concurrent use.
type TokenValidator interface {
	// ValidateToken checks whether token is valid for the given
	// userID and session issuance time (unix seconds).
	ValidateToken(userID string, sessIssuedAtUnix UnixSeconds, token string) bool
}
