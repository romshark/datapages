// Package auth reads and writes the session of a request: the cookie that
// names it, the record behind it and the CSRF token that goes with it.
//
// Application code must not import this package.
package auth

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/modules/sessions"
	"github.com/romshark/datapages/runtime/httpread"
	"github.com/romshark/datapages/runtime/subject"
)

// Metrics counts what the manager does. A nil Metrics counts nothing.
type Metrics interface {
	// SessionRead counts a session lookup. outcome is "none", "error",
	// "stale", "expired" or "valid".
	SessionRead(outcome string)
	// SessionCreated counts a session issued. outcome is "success" or "error".
	SessionCreated(outcome string)
	// SessionClosed counts a session removed. outcome is "success" or "error".
	SessionClosed(outcome string)
}

// Server is what the manager needs of the server it belongs to.
type Server interface {
	Logger() *slog.Logger
	// TLSEnabled reports whether the session cookie may carry the Secure flag.
	TLSEnabled() bool
}

// Manager reads and writes the session of a request.
type Manager[Data any] struct {
	server   Server
	sessions sessions.Manager[Data]
	conf     datapages.SessionsConfig
	csrf     *datapages.CSRFConfig
	metrics  Metrics
}

// NewManager returns a manager reading sessions from store.
// conf and csrfConf may be zero, metrics may be nil.
func NewManager[Data any](
	server Server,
	store sessions.Manager[Data],
	conf datapages.SessionsConfig,
	csrfConf *datapages.CSRFConfig,
	metrics Metrics,
) *Manager[Data] {
	if conf.TokenCookie.Name == "" {
		conf.TokenCookie.Name = datapages.DefaultSessionCookieName
	}
	if conf.SessionTokenGenerator == nil {
		conf.SessionTokenGenerator = sessions.DefaultTokenGenerator{
			Length: sessions.DefaultTokenLen,
		}
	}
	return &Manager[Data]{
		server:   server,
		sessions: store,
		conf:     conf,
		csrf:     csrfConf,
		metrics:  metrics,
	}
}

// CookieName is the name of the cookie the session token is kept in.
func (m *Manager[Data]) CookieName() string { return m.conf.TokenCookie.Name }

// SessionTokenGenerator generates the token a new session is named by.
func (m *Manager[Data]) SessionTokenGenerator() sessions.TokenGenerator {
	return m.conf.SessionTokenGenerator
}

// SessionManager is the store the sessions live in.
func (m *Manager[Data]) SessionManager() sessions.Manager[Data] {
	return m.sessions
}

// CSRFToken returns the token a signed-in visitor sends back on an action.
// It is empty when CSRF protection is off.
func (m *Manager[Data]) CSRFToken(userID string, issuedAt time.Time) string {
	if m.csrf == nil {
		return ""
	}
	return m.csrf.Tokens.GenerateToken(userID, issuedAt.Unix())
}

func (m *Manager[Data]) sessionRead(outcome string) {
	if m.metrics != nil {
		m.metrics.SessionRead(outcome)
	}
}

// ReadSession reads the session the request carries.
// ok is false when the request was answered and the handler must not run.
func (m *Manager[Data]) ReadSession(w http.ResponseWriter, r *http.Request) (
	sess datapages.Session[Data], token string, ok bool,
) {
	cookieVal, found := httpread.CookieValue(r, m.conf.TokenCookie.Name)
	if !found {
		m.sessionRead("none")
		return sess, "", true
	}

	rec, token, ok, err := m.sessions.ReadSessionFromCookie(cookieVal)
	if err != nil {
		// Transient backend failure; keep the cookie, fail the request.
		m.sessionRead("error")
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return datapages.Session[Data]{}, "", false
	}
	if !ok {
		// Cookie is stale or malformed; clear it and continue as unauthenticated.
		m.sessionRead("stale")
		m.SetSessionCookie(w, "")
		return datapages.Session[Data]{}, "", true
	}
	sess = datapages.MakeSession(
		rec.UserID, token, rec.IssuedAt, rec.ExpiresAt, rec.Data,
	)

	if !sess.ExpiresAt().IsZero() && !time.Now().Before(sess.ExpiresAt()) {
		// Session has expired; clear the cookie and continue as unauthenticated.
		m.sessionRead("expired")
		m.SetSessionCookie(w, "")
		return datapages.Session[Data]{}, "", true
	}
	m.sessionRead("valid")

	if !m.CheckCSRF(w, r, sess.UserID(), sess.IssuedAt()) {
		return sess, token, false
	}

	return sess, token, true
}

// CheckCSRF answers r and returns false when the request carries no valid
// CSRF token. A read method, a guest and disabled protection all pass.
func (m *Manager[Data]) CheckCSRF(
	w http.ResponseWriter, r *http.Request, userID string, issuedAt time.Time,
) (ok bool) {
	if userID == "" ||
		r.Method == http.MethodGet ||
		r.Method == http.MethodOptions ||
		r.Method == http.MethodHead ||
		m.csrf == nil {
		return true
	}
	t := r.Header.Get("X-CSRF-Token")
	if t == "" {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return false
	}
	if m.csrf.DevBypassToken != "" && t == m.csrf.DevBypassToken {
		return true
	}
	if !m.csrf.Tokens.ValidateToken(userID, issuedAt.Unix(), t) {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return false
	}
	return true
}

// SetSessionCookie writes the session cookie. An empty value clears it.
func (m *Manager[Data]) SetSessionCookie(w http.ResponseWriter, value string) {
	cookie := http.Cookie{
		Name:     m.conf.TokenCookie.Name,
		Value:    value,
		Path:     "/",
		Domain:   m.conf.TokenCookie.Domain,
		HttpOnly: !m.conf.DisableHTTPOnly,
		Secure:   m.server.TLSEnabled(),
		SameSite: http.SameSiteLaxMode,
	}
	if value == "" {
		cookie.MaxAge = -1
		cookie.Expires = time.Unix(0, 0)
	}
	http.SetCookie(w, &cookie)
}

// CreateSession stores a new session and puts its token into the cookie.
func (m *Manager[Data]) CreateSession(
	w http.ResponseWriter, r *http.Request, session datapages.NewSession[Data],
) error {
	if !subject.IsToken(session.UserID) {
		return fmt.Errorf(
			"user ID must be a non-empty subject token, received %q",
			session.UserID)
	}
	token, err := m.sessions.CreateSession(r.Context(), sessions.Record[Data]{
		UserID:    session.UserID,
		IssuedAt:  time.Now(),
		ExpiresAt: session.ExpiresAt,
		Data:      session.Data,
	})
	if err != nil {
		if m.metrics != nil {
			m.metrics.SessionCreated("error")
		}
		return err
	}
	if m.metrics != nil {
		m.metrics.SessionCreated("success")
	}
	m.SetSessionCookie(w, token)
	return nil
}

// CloseSession removes the session named by token and clears the cookie.
func (m *Manager[Data]) CloseSession(
	w http.ResponseWriter, r *http.Request, token string,
) error {
	if err := m.sessions.CloseSession(r.Context(), token); err != nil {
		if m.metrics != nil {
			m.metrics.SessionClosed("error")
		}
		return err
	}
	if m.metrics != nil {
		m.metrics.SessionClosed("success")
	}
	m.SetSessionCookie(w, "")
	return nil
}
