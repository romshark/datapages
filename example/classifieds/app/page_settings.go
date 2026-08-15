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
) (body templ.Component, redirect string, err error) {
	if session.IsGuest() {
		return nil, href.PageLogin(), nil
	}

	sessions := make(map[string]SessionRecord)
	maps.Insert(sessions, p.App.sessions.UserSessions(r.Context(), session.UserID()))
	body, err = p.render(r.Context(), session)
	return body, "", err
}

// POSTSave is /settings/save/{$}
func (p PageSettings) POSTSave(
	r *http.Request,
	sse datapages.SSE,
	session Session,
	signals struct {
		Username string `json:"username"`
	},
) (redirect string, err error) {
	if session.IsGuest() {
		return href.PageLogin(), nil
	}
	// TODO
	return "", nil
}

// POSTCloseSession is /settings/close-session/{token}/{$}
func (p PageSettings) POSTCloseSession(
	r *http.Request,
	session Session,
	path struct {
		Token string `path:"token"`
	},
	dispatch func(EventSessionClosed) error,
) (
	closeSession bool,
	redirect string,
	err error,
) {
	if session.IsGuest() {
		return false, "", domain.ErrUnauthorized
	}
	sess, err := p.App.sessions.Session(r.Context(), path.Token)
	if err != nil {
		return false, "", err
	}
	if sess.UserID != session.UserID() {
		return false, "", domain.ErrUnauthorized
	}
	// Even though closeSession=true would close the sessions, let's close it
	// explicitly before we dispatch the event to make sure it's closed before
	// we claim it is.
	if err := p.App.sessions.CloseSession(r.Context(), path.Token); err != nil {
		return false, "", err
	}
	_ = dispatch(EventSessionClosed{
		SubjectUser: []string{sess.UserID},
		Token:       path.Token,
	})
	if session.Token() == path.Token {
		// Closed current session
		return true, href.PageLogin(), nil
	}
	// Closed another session.
	return false, "", nil
}

// POSTCloseAllSessions is /settings/close-all-sessions/{$}
func (p PageSettings) POSTCloseAllSessions(
	r *http.Request,
	session Session,
	dispatch func(EventSessionClosed) error,
) (redirect string, err error) {
	if session.IsGuest() {
		return "", domain.ErrUnauthorized
	}
	closed, err := p.App.sessions.CloseAllUserSessions(r.Context(), nil, session.UserID())
	if err != nil {
		return "", err
	}
	targetUsers := []string{session.UserID()}
	for _, token := range closed {
		_ = dispatch(EventSessionClosed{
			SubjectUser: targetUsers,
			Token:       token,
		})
	}
	return href.PageLogin(), nil
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
