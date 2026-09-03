package sessions

import (
	"context"
	"errors"
	"iter"
	"time"
)

var (
	// ErrSessionNotFound reports that no session exists for a token.
	// It is declared here rather than per store so that one errors.Is check
	// works against every implementation.
	ErrSessionNotFound = errors.New("session not found")

	// ErrEmptyUserID reports a record with no user.
	ErrEmptyUserID = errors.New("userID must not be empty")

	// ErrEmptyToken reports that [TokenGenerator.Generate] returned "".
	// Every store refuses it: an empty token names no session,
	// and the cookie carrying it cannot be told apart from no cookie at all.
	ErrEmptyToken = errors.New("token must not be empty")
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
	//
	// The token returned must be cookieValue itself.
	// The CSRF token is derived from it, and an action that takes
	// no session is checked against the cookie without reading the store.
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

// UserSessionIterator iterates the live sessions of one user.
//
// Optional: a store need not implement it, and an application that lists
// sessions needs it. It's declared here so that the built-in stores agree on the API,
// which lets an application swap one for another seamlessly.
type UserSessionIterator[Data any] interface {
	// UserSessions iterates the live sessions of userID as (token, record) pairs,
	// a snapshot rather than a stream. The token is the one CloseSession, Session and
	// NotifyClosed take. An empty userID yields nothing.
	//
	// The error reports that the store could not be read. It exists so that
	// an unreachable store is not the same answer as a user with no sessions.
	UserSessions(
		ctx context.Context, userID string,
	) (iter.Seq2[string, Record[Data]], error)
}

// UserSessionCloser closes every live session of one user.
//
// Optional in the same way as [UserSessionIterator]:
// a store that cannot look sessions up by user cannot implement it.
// It is declared here so that the built-in stores agree on the API.
type UserSessionCloser interface {
	// CloseAllUserSessions closes the sessions of userID that exist at
	// call time and, if buffer is non-nil, appends their tokens to it.
	// An empty userID is an error; which error to return is up to the store.
	CloseAllUserSessions(
		ctx context.Context, buffer []string, userID string,
	) ([]string, error)
}

// CloseNotifier reports session closure to interested listeners.
type CloseNotifier interface {
	// NotifyClosed sets up a listener that calls fn when session with token is closed.
	// The listener shall be stopped once ctx is canceled.
	// If the session manager implementation doesn't support
	// dynamic closure notification then NotifyClosed is a no-op.
	NotifyClosed(ctx context.Context, token string, fn func()) error
}

// ExpiredDeleter deletes the sessions that have expired.
//
// Reading a session reclaims only the ones a client comes back to.
// An abandoned session is never read again and needs to be garbage collected.
type ExpiredDeleter interface {
	// DeleteExpired deletes every session whose ExpiresAt has passed and
	// returns how many it deleted. One that never expires is left alone.
	//
	// The server never calls it. When to collect, how often and which
	// replica does it is the application's to schedule.
	DeleteExpired(ctx context.Context) (deleted int, err error)
}

// Manager stores and restores sessions.
type Manager[Data any] interface {
	Reader[Data]
	Creator[Data]
	Closer
	CloseNotifier
	ExpiredDeleter
}
