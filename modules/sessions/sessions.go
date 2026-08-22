package sessions

import (
	"context"
	"time"
)

// TokenGenerator generates cryptographically random unique session identifiers.
type TokenGenerator interface {
	// Generates a cryptographically random session token.
	Generate() (string, error)
}

// Record is what a session manager stores and restores.
// Data is the application-defined payload,
// the rest is what datapages needs to resolve a session.
// The token isn't part of it, it identifies the record.
type Record[Data any] struct {
	// UserID identifies the authenticated user. It's never empty.
	UserID string

	// IssuedAt is the time the session was created at.
	IssuedAt time.Time

	// ExpiresAt is the time the session becomes invalid at.
	// The zero value never expires.
	ExpiresAt time.Time

	// Data is the application-defined payload of the session.
	Data Data
}

// Reader restores a session from the value of the session cookie.
type Reader[Data any] interface {
	// ReadSessionFromCookie returns the stored record and the raw authentication
	// token for the value of the session cookie.
	// Returns ok=false, err=nil if the value is empty, malformed,
	// or the session no longer exists; the caller should remove the cookie.
	// Returns (ok=false,err!=nil) on transient backend failures,
	// in which case the caller should keep the cookie and fail the request.
	ReadSessionFromCookie(cookieValue string) (
		rec Record[Data], token string, ok bool, err error,
	)
}

// Creator creates new sessions.
type Creator[Data any] interface {
	// CreateSession creates a new session identified by a unique token.
	// The returned token will be put into HTTP-only cookies.
	CreateSession(
		ctx context.Context, rec Record[Data],
	) (token string, err error)
}

// Closer closes existing sessions.
type Closer interface {
	// CloseSession closes a session identified by token.
	// No-op and no error if that session doesn't exist.
	CloseSession(ctx context.Context, token string) error
}

// CloseNotifier reports session closure to interested listeners.
type CloseNotifier interface {
	// NotifyClosed sets up a listener that calls fn when session with token is closed.
	// The listener shall be stopped once ctx is canceled.
	// If the session manager implementation doesn't support
	// dynamic closure notification then NotifyClosed is a no-op.
	NotifyClosed(ctx context.Context, token string, fn func()) error
}

// Manager stores and restores sessions.
type Manager[Data any] interface {
	Reader[Data]
	Creator[Data]
	Closer
	CloseNotifier
}
