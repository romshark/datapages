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

type entry[Data any] struct {
	rec sessmanager.Record[Data]
}

type watcher struct {
	ctx context.Context
	fn  func()
}

// SessionManager is an in-memory session manager.
type SessionManager[Data any] struct {
	lock     sync.Mutex
	sessions map[string]entry[Data]        // token -> entry
	watchers map[string]map[uint64]watcher // token -> watcherID -> watcher
	nextID   uint64
	tokenGen sessmanager.TokenGenerator
}

// New creates a new in-memory session manager.
func New[Data any](tokenGen sessmanager.TokenGenerator) *SessionManager[Data] {
	return &SessionManager[Data]{
		sessions: make(map[string]entry[Data]),
		watchers: make(map[string]map[uint64]watcher),
		tokenGen: tokenGen,
	}
}

// ReadSessionFromCookie returns the record associated with the cookie value.
// The cookie value is the raw session token.
func (m *SessionManager[Data]) ReadSessionFromCookie(c *http.Cookie) (
	rec sessmanager.Record[Data], token string, ok bool, err error,
) {
	if c == nil || c.Value == "" {
		return rec, "", false, nil
	}

	m.lock.Lock()
	e, exists := m.sessions[c.Value]
	m.lock.Unlock()

	if !exists {
		return rec, "", false, nil
	}

	return e.rec, c.Value, true, nil
}

// CreateSession stores a new session and returns a token to be used as a cookie value.
func (m *SessionManager[Data]) CreateSession(
	_ context.Context, rec sessmanager.Record[Data],
) (string, error) {
	if rec.UserID == "" {
		return "", ErrEmptyUserID
	}
	token, err := m.tokenGen.Generate()
	if err != nil {
		return "", err
	}

	m.lock.Lock()
	m.sessions[token] = entry[Data]{rec: rec}
	m.lock.Unlock()

	return token, nil
}

// NotifyClosed registers fn to be called when the session identified by token is closed.
// If the session doesn't exist, fn is called immediately.
// If ctx is already canceled, the watcher is not registered.
// The watcher is automatically removed when ctx is canceled.
func (m *SessionManager[Data]) NotifyClosed(
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
func (m *SessionManager[Data]) CloseSession(_ context.Context, token string) error {
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

// SaveSession overwrites the record for an existing token.
// No-op if the session doesn't exist.
func (m *SessionManager[Data]) SaveSession(
	_ context.Context, token string, rec sessmanager.Record[Data],
) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	e, exists := m.sessions[token]
	if !exists {
		return nil
	}
	e.rec = rec
	m.sessions[token] = e
	return nil
}

// Session retrieves a session record by its token.
func (m *SessionManager[Data]) Session(
	_ context.Context, token string,
) (sessmanager.Record[Data], error) {
	m.lock.Lock()
	e, exists := m.sessions[token]
	m.lock.Unlock()

	if !exists {
		var zero sessmanager.Record[Data]
		return zero, ErrSessionNotFound
	}
	return e.rec, nil
}

// CloseAllUserSessions closes all sessions for a user.
// If buffer is non-nil, appends tokens of closed sessions to it.
func (m *SessionManager[Data]) CloseAllUserSessions(
	_ context.Context, buffer []string, userID string,
) ([]string, error) {
	if userID == "" {
		return buffer, ErrEmptyUserID
	}
	m.lock.Lock()
	var tokens []string
	for tok, e := range m.sessions {
		if e.rec.UserID == userID {
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

// UserSession is a token and record pair.
type UserSession[Data any] struct {
	Token  string
	Record sessmanager.Record[Data]
}

// UserSessions returns all current sessions for a user.
func (m *SessionManager[Data]) UserSessions(
	_ context.Context, userID string,
) []UserSession[Data] {
	if userID == "" {
		return nil
	}
	m.lock.Lock()
	defer m.lock.Unlock()

	var result []UserSession[Data]
	for tok, e := range m.sessions {
		if e.rec.UserID == userID {
			result = append(result, UserSession[Data]{Token: tok, Record: e.rec})
		}
	}
	return result
}
