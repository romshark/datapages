package sessmanager

import (
	"context"
	"net/http"
)

// TokenGenerator generates cryptographically random unique session identifiers.
type TokenGenerator interface {
	// Generates a cryptographically random session token.
	Generate() (string, error)
}

type SessionManager[Session any] interface {
	// ReadSessionFromCookie returns the resolved session and
	// the raw authentication token. Returns ok=false, err=nil if auth information is
	// malformed and therefore the cookie must be removed.
	// Returns (ok=false,err!=nil) on transient backend failures, in which case the
	// caller should keep the cookie and fail the request.
	ReadSessionFromCookie(c *http.Cookie) (
		session Session, token, userID string, ok bool, err error,
	)

	// CreateSession creates a new session identified by a unique token.
	// The returned token will be put into HTTP-only cookies.
	CreateSession(
		ctx context.Context, userID string, session Session,
	) (token string, err error)

	// NotifyClosed sets up a listener that calls fn when session with token is closed.
	// The listener shall be stopped once ctx is canceled.
	// If the session manager implementation doesn't support dynamic closure notification
	// then NotifyClosed is a no-op.
	NotifyClosed(ctx context.Context, token string, fn func()) error

	// CloseSession closes a session identified by token.
	// No-op and no error if that session doesn't exist.
	CloseSession(ctx context.Context, token string) error
}
