package app

import (
	"context"
	"maps"
	"net/http"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/classifieds/app/domain"
	"github.com/romshark/datapages/example/classifieds/datapagesgen/href"
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

	sessions := make(map[string]SessionRecord)
	maps.Insert(sessions, p.App.sessions.UserSessions(ctx, session.UserID()))

	return pageSettings(session, sessions, u, baseData), nil
}

func (p PageSettings) GET(
	r *http.Request,
	session Session,
) (body datapages.Component, redirect datapages.Redirect, err error) {
	if session.IsGuest() {
		return nil, datapages.Redirect{URL: href.PageLogin()}, nil
	}

	sessions := make(map[string]SessionRecord)
	maps.Insert(sessions, p.App.sessions.UserSessions(r.Context(), session.UserID()))
	body, err = p.render(r.Context(), session)
	return body, redirect, err
}

// POSTSave is /settings/save/{$}
func (p PageSettings) POSTSave(
	r *http.Request,
	sse datapages.SSE,
	session Session,
	signals struct {
		Username string `json:"username"`
	},
) (redirect datapages.Redirect, err error) {
	if session.IsGuest() {
		return datapages.Redirect{URL: href.PageLogin()}, nil
	}
	// TODO
	return redirect, nil
}

// POSTCloseSession is /settings/close-session/{token}/{$}
func (p PageSettings) POSTCloseSession(
	r *http.Request,
	session Session,
	path struct {
		Token string `path:"token"`
	},
	sessionClosed datapages.Dispatcher[EventSessionClosed],
) (
	closeSession bool,
	redirect datapages.Redirect,
	err error,
) {
	if session.IsGuest() {
		return false, redirect, domain.ErrUnauthorized
	}
	sess, err := p.App.sessions.Session(r.Context(), path.Token)
	if err != nil {
		return false, redirect, err
	}
	if sess.UserID != session.UserID() {
		return false, redirect, domain.ErrUnauthorized
	}
	// Even though closeSession=true would close the sessions, let's close it
	// explicitly before we sessionClosed the event to make sure it's closed before
	// we claim it is.
	if err := p.App.sessions.CloseSession(r.Context(), path.Token); err != nil {
		return false, redirect, err
	}
	_ = sessionClosed.Dispatch(EventSessionClosed{
		Recipient: datapages.SubjectUser(sess.UserID),
		Token:     path.Token,
	})
	if session.Token() == path.Token {
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
		return redirect, domain.ErrUnauthorized
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
