// Package inmem provides an in-memory session manager.
//
// WARNING: Do not use this in production deployments;
// sessions are not persisted and are lost on restart.
// In production deployments prefer using a persistent store
// (e.g. github.com/romshark/datapages/modules/sessmanager/natskv).
package inmem

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/romshark/datapages/modules/sessmanager"
)

var (
	// ErrSessionNotFound is returned when a session is not found.
	ErrSessionNotFound = errors.New("session not found")

	// ErrEmptyUserID is returned when a userID is empty.
	ErrEmptyUserID = errors.New("userID must not be empty")
)

var _ sessmanager.SessionManager[struct{}] = (*SessionManager[struct{}])(nil)

type entry[S any] struct {
	userID  string
	session S
}

type watcher struct {
	ctx context.Context
	fn  func()
}

// SessionManager is an in-memory session manager.
type SessionManager[S any] struct {
	lock     sync.Mutex
	sessions map[string]entry[S]           // token -> entry
	watchers map[string]map[uint64]watcher // token -> watcherID -> watcher
	nextID   uint64
	tokenGen sessmanager.TokenGenerator
}

// New creates a new in-memory session manager.
func New[S any](tokenGen sessmanager.TokenGenerator) *SessionManager[S] {
	return &SessionManager[S]{
		sessions: make(map[string]entry[S]),
		watchers: make(map[string]map[uint64]watcher),
		tokenGen: tokenGen,
	}
}

// ReadSessionFromCookie returns the session associated with the cookie value.
// The cookie value is the raw session token.
func (m *SessionManager[S]) ReadSessionFromCookie(c *http.Cookie) (
	session S, token, userID string, ok bool, err error,
) {
	if c == nil || c.Value == "" {
		return session, "", "", false, nil
	}

	m.lock.Lock()
	e, exists := m.sessions[c.Value]
	m.lock.Unlock()

	if !exists {
		return session, "", "", false, nil
	}

	return e.session, c.Value, e.userID, true, nil
}

// CreateSession stores a new session and returns a token to be used as a cookie value.
func (m *SessionManager[S]) CreateSession(
	_ context.Context, userID string, session S,
) (string, error) {
	if userID == "" {
		return "", ErrEmptyUserID
	}
	token, err := m.tokenGen.Generate()
	if err != nil {
		return "", err
	}

	m.lock.Lock()
	m.sessions[token] = entry[S]{userID: userID, session: session}
	m.lock.Unlock()

	return token, nil
}

// NotifyClosed registers fn to be called when the session identified by token is closed.
// If the session doesn't exist, fn is called immediately.
// If ctx is already canceled, the watcher is not registered.
// The watcher is automatically removed when ctx is canceled.
func (m *SessionManager[S]) NotifyClosed(
	ctx context.Context, token string, fn func(),
) error {
	m.lock.Lock()
	if _, exists := m.sessions[token]; !exists {
		m.lock.Unlock()
		fn()
		return nil
	}
	if ctx.Err() != nil {
		m.lock.Unlock()
		return nil
	}
	id := m.nextID
	m.nextID++
	ws := m.watchers[token]
	if ws == nil {
		ws = make(map[uint64]watcher)
		m.watchers[token] = ws
	}
	ws[id] = watcher{ctx: ctx, fn: fn}
	m.lock.Unlock()

	go func() {
		<-ctx.Done()
		m.lock.Lock()
		defer m.lock.Unlock()
		if ws := m.watchers[token]; ws != nil {
			delete(ws, id)
			if len(ws) == 0 {
				delete(m.watchers, token)
			}
		}
	}()

	return nil
}

// CloseSession removes a session and notifies all registered watchers.
func (m *SessionManager[S]) CloseSession(_ context.Context, token string) error {
	m.lock.Lock()
	delete(m.sessions, token)
	ws := m.watchers[token]
	delete(m.watchers, token)
	m.lock.Unlock()

	for _, w := range ws {
		if w.ctx.Err() == nil {
			w.fn()
		}
	}

	return nil
}

// SaveSession overwrites the session data for an existing token.
// No-op if the session doesn't exist.
func (m *SessionManager[S]) SaveSession(
	_ context.Context, token string, session S,
) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	e, exists := m.sessions[token]
	if !exists {
		return nil
	}
	e.session = session
	m.sessions[token] = e
	return nil
}

// Session retrieves a session by its token.
func (m *SessionManager[S]) Session(_ context.Context, token string) (S, error) {
	m.lock.Lock()
	e, exists := m.sessions[token]
	m.lock.Unlock()

	if !exists {
		var zero S
		return zero, ErrSessionNotFound
	}
	return e.session, nil
}

// CloseAllUserSessions closes all sessions for a user.
// If buffer is non-nil, appends tokens of closed sessions to it.
func (m *SessionManager[S]) CloseAllUserSessions(
	_ context.Context, buffer []string, userID string,
) ([]string, error) {
	if userID == "" {
		return buffer, ErrEmptyUserID
	}
	m.lock.Lock()
	var tokens []string
	for tok, e := range m.sessions {
		if e.userID == userID {
			tokens = append(tokens, tok)
		}
	}
	var allWs []watcher
	for _, tok := range tokens {
		delete(m.sessions, tok)
		for _, w := range m.watchers[tok] {
			allWs = append(allWs, w)
		}
		delete(m.watchers, tok)
		if buffer != nil {
			buffer = append(buffer, tok)
		}
	}
	m.lock.Unlock()

	for _, w := range allWs {
		if w.ctx.Err() == nil {
			w.fn()
		}
	}
	return buffer, nil
}

// UserSession is a token–session pair.
type UserSession[S any] struct {
	Token   string
	Session S
}

// UserSessions returns all current sessions for a user.
func (m *SessionManager[S]) UserSessions(
	_ context.Context, userID string,
) []UserSession[S] {
	if userID == "" {
		return nil
	}
	m.lock.Lock()
	defer m.lock.Unlock()

	var result []UserSession[S]
	for tok, e := range m.sessions {
		if e.userID == userID {
			result = append(result, UserSession[S]{Token: tok, Session: e.session})
		}
	}
	return result
}
