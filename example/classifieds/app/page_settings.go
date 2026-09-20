package app

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/classifieds/app/datapagesgen/href"
	"github.com/romshark/datapages/example/classifieds/app/domain"
)

// PageSettings is /settings
type PageSettings struct {
	App *App
	Base
}

func (p PageSettings) render(
	ctx context.Context, session Session,
) (templ.Component, error) {
	u, err := p.App.repo.UserByID(ctx, session.UserID())
	if err != nil {
		return nil, err
	}

	baseData, err := p.baseData(ctx, session)
	if err != nil {
		return nil, err
	}

	userSessions, err := p.App.sessions.UserSessions(ctx, session.UserID())
	if err != nil {
		return nil, err
	}
	sessions := make(map[string]SessionRecord)
	maps.Insert(sessions, userSessions)

	return pageSettings(session, sessions, u, baseData), nil
}

func (p PageSettings) GET(
	r *http.Request,
	session Session,
) (body datapages.Component, redirect datapages.Redirect, err error) {
	if session.IsGuest() {
		return nil, datapages.Redirect{URL: href.PageLogin()}, nil
	}

	body, err = p.render(r.Context(), session)
	return body, redirect, err
}

// POSTSave is /settings/save/{$}
//
// The user name is the session's user ID, hence the rename issues a new session.
// newSession rules out an sse parameter, so the page comes back
// through the redirect rather than a patch.
func (p PageSettings) POSTSave(
	r *http.Request,
	session Session,
	signals datapages.Signals[struct {
		Username string `json:"username"`
	}],
) (
	newSession datapages.NewSession[struct{}],
	redirect datapages.Redirect,
	err error,
) {
	if session.IsGuest() {
		return newSession, datapages.Redirect{URL: href.PageLogin()}, nil
	}

	name := strings.TrimSpace(signals.Values.Username)
	if name == session.UserID() {
		return newSession, redirect, nil
	}
	if err := datapages.ValidateUserID(name); err != nil {
		return newSession, redirect, fmt.Errorf("%w: %w", datapages.ErrBadRequest, err)
	}
	if err := p.App.repo.RenameUser(r.Context(), session.UserID(), name); err != nil {
		if errors.Is(err, domain.ErrUserNameReserved) {
			// A name someone else holds is the visitor's mistake, not a fault.
			return newSession, redirect,
				fmt.Errorf("%w: %w", datapages.ErrBadRequest, err)
		}
		return newSession, redirect, err
	}

	return datapages.NewSession[struct{}]{UserID: name},
		datapages.Redirect{URL: href.PageSettings()}, nil
}

// POSTCloseSession is /settings/close-session/{token}/{$}
func (p PageSettings) POSTCloseSession(
	r *http.Request,
	session Session,
	path datapages.Path[struct {
		Token string `path:"token"`
	}],
	sessionClosed datapages.Dispatcher[EventSessionClosed],
) (
	closeSession datapages.CloseSession,
	redirect datapages.Redirect,
	err error,
) {
	if session.IsGuest() {
		return false, redirect, errForbidden(domain.ErrUnauthorized)
	}
	sess, err := p.App.sessions.Session(r.Context(), path.Values.Token)
	if err != nil {
		return false, redirect, err
	}
	if sess.UserID != session.UserID() {
		return false, redirect, errForbidden(domain.ErrUnauthorized)
	}
	// Even though closeSession=true would close the sessions, let's close it
	// explicitly before we sessionClosed the event to make sure it's closed before
	// we claim it is.
	if err := p.App.sessions.CloseSession(r.Context(), path.Values.Token); err != nil {
		return false, redirect, err
	}
	_ = sessionClosed.Dispatch(EventSessionClosed{
		Recipient: datapages.SubjectUser(sess.UserID),
		Token:     path.Values.Token,
	})
	if session.Token() == path.Values.Token {
		// Closed current session
		return true, datapages.Redirect{URL: href.PageLogin()}, nil
	}
	// Closed another session.
	return false, redirect, nil
}

// POSTCloseAllSessions is /settings/close-all-sessions/{$}
func (p PageSettings) POSTCloseAllSessions(
	r *http.Request,
	session Session,
	sessionClosed datapages.Dispatcher[EventSessionClosed],
) (redirect datapages.Redirect, err error) {
	if session.IsGuest() {
		return redirect, errForbidden(domain.ErrUnauthorized)
	}
	closed, err := p.App.sessions.CloseAllUserSessions(r.Context(), nil, session.UserID())
	if err != nil {
		return redirect, err
	}
	recipient := datapages.SubjectUser(session.UserID())
	for _, token := range closed {
		_ = sessionClosed.Dispatch(EventSessionClosed{
			Recipient: recipient,
			Token:     token,
		})
	}
	return datapages.Redirect{URL: href.PageLogin()}, nil
}

func (p PageSettings) OnSessionClosed(
	event EventSessionClosed,
	sse datapages.SSE,
	session Session,
) error {
	if event.Token == session.Token() {
		// Current session was closed
		return sse.Redirect(href.PageLogin())
	}
	body, err := p.render(sse.Context(), session)
	if err != nil {
		return err
	}
	return sse.PatchElement(body)
}
