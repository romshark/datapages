// Package auth reads and writes the session of a request: the cookie that
// names it, the record behind it and the CSRF token that goes with it.
//
// Application code must not import this package.
package auth

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/modules/csrf"
	"github.com/romshark/datapages/modules/sessions"
	"github.com/romshark/datapages/runtime/httpread"
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
	server       Server
	sessions     sessions.Manager[Data]
	conf         datapages.SessionsConfig
	csrf         datapages.CSRFConfig
	csrfDisabled bool
	metrics      Metrics
}

// NewManager returns a manager reading sessions from store.
// cfg may be zero, in which case the CSRF protection runs on the
// built-in tokens. metrics may be nil.
func NewManager[Data any](
	server Server,
	store sessions.Manager[Data],
	cfg datapages.ServerConfig,
	metrics Metrics,
) *Manager[Data] {
	conf := cfg.Sessions
	if conf.Cookie.Name == "" {
		conf.Cookie.Name = datapages.DefaultSessionCookieName
	}
	if conf.TokenGenerator == nil {
		conf.TokenGenerator = sessions.DefaultTokenGenerator{
			Length: sessions.DefaultTokenLen,
		}
	}
	var csrfConf datapages.CSRFConfig
	if cfg.CSRF != nil {
		csrfConf = *cfg.CSRF
	}
	if csrfConf.Tokens == nil {
		csrfConf.Tokens = csrf.Tokens{}
	}
	if csrfConf.Disabled {
		server.Logger().Warn("CSRF protection is disabled",
			slog.String("reason", "CSRFConfig.Disabled is set"),
			slog.String("effect", "state-changing actions are accepted on the "+
				"session cookie alone"))
	}
	return &Manager[Data]{
		server:       server,
		sessions:     store,
		conf:         conf,
		csrf:         csrfConf,
		csrfDisabled: csrfConf.Disabled,
		metrics:      metrics,
	}
}

// CookieName is the name of the cookie the session token is kept in.
func (m *Manager[Data]) CookieName() string { return m.conf.Cookie.Name }

// SessionTokenGenerator generates the token a new session is named by.
func (m *Manager[Data]) SessionTokenGenerator() sessions.TokenGenerator {
	return m.conf.TokenGenerator
}

// SessionManager is the store the sessions live in.
func (m *Manager[Data]) SessionManager() sessions.Manager[Data] {
	return m.sessions
}

// CSRFEnabled reports whether state-changing actions are CSRF protected.
func (m *Manager[Data]) CSRFEnabled() bool { return !m.csrfDisabled }

// WriteCSRFToken writes the token a signed-in visitor sends back on an action
// and reports how many bytes it wrote. It writes nothing for a guest and when
// CSRF protection is off.
func (m *Manager[Data]) WriteCSRFToken(
	w io.Writer, sessionToken string,
) (n int, err error) {
	if m.csrfDisabled {
		return 0, nil
	}
	return m.csrf.Tokens.WriteToken(w, sessionToken)
}

// csrfScriptPrefix and csrfScriptSuffix wrap the CSRF token in the module
// script that adds it to every state-changing Datastar fetch. Datastar issues
// those through globalThis.fetch, which is why overriding it reaches them all.
const (
	csrfScriptPrefix = `
	<script type="module">
		const o = globalThis.fetch.bind(globalThis)
		globalThis.fetch=(i,init={}) => {
			const isReq=i instanceof Request
			const r=isReq ? i:new Request(i,init)
			if (r.headers.get("Datastar-Request")!=="true" ||
				r.method=="GET"||r.method=="HEAD"||r.method=="OPTIONS"
			) return isReq ? o(r,init):o(r)
			const h=new Headers(r.headers)
			h.set("X-CSRF-Token",'`

	csrfScriptSuffix = `')
			return o(new Request(r,{...init,headers:h}))
		}
	</script>`
)

// WriteCSRFScript writes the script that adds the X-CSRF-Token header to every
// state-changing Datastar fetch of the page. It writes nothing for a guest
// (empty userID) and when CSRF protection is off.
func (m *Manager[Data]) WriteCSRFScript(
	w io.Writer, userID, sessionToken string,
) error {
	if userID == "" || m.csrfDisabled {
		return nil
	}
	if _, err := io.WriteString(w, csrfScriptPrefix); err != nil {
		return err
	}
	n, err := m.WriteCSRFToken(w, sessionToken)
	if err != nil {
		return err
	}
	if n == 0 {
		m.server.Logger().Warn("wrote empty CSRF token",
			slog.String("user-id", userID))
	}
	_, err = io.WriteString(w, csrfScriptSuffix)
	return err
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
	cookieVal, found := httpread.CookieValue(r, m.conf.Cookie.Name)
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
		// Nothing reads this record again; leaving it would waste store space.
		if err := m.sessions.CloseSession(r.Context(), token); err != nil {
			m.server.Logger().Error("closing an expired session",
				slog.Any("err", err))
		}
		return datapages.Session[Data]{}, "", true
	}
	m.sessionRead("valid")

	if !m.CheckCSRF(w, r, token) {
		return sess, token, false
	}

	return sess, token, true
}

// CheckCSRF answers r and returns false when the request carries no valid
// CSRF token. A read method, a guest and disabled protection all pass.
func (m *Manager[Data]) CheckCSRF(
	w http.ResponseWriter, r *http.Request, sessionToken string,
) (ok bool) {
	if sessionToken == "" ||
		r.Method == http.MethodGet ||
		r.Method == http.MethodOptions ||
		r.Method == http.MethodHead ||
		m.csrfDisabled {
		return true
	}
	t := r.Header.Get("X-CSRF-Token")
	if t == "" {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return false
	}
	if !m.csrf.Tokens.ValidateToken(sessionToken, t) {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return false
	}
	return true
}

// SetSessionCookie writes the session cookie. An empty value clears it.
func (m *Manager[Data]) SetSessionCookie(w http.ResponseWriter, value string) {
	cookie := http.Cookie{
		Name:     m.conf.Cookie.Name,
		Value:    value,
		Path:     "/",
		Domain:   m.conf.Cookie.Domain,
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
	if err := datapages.ValidateUserID(session.UserID); err != nil {
		return fmt.Errorf("user ID %q: %w", session.UserID, err)
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
