package app

import (
	"context"
	"fmt"
	"maps"
	"net/http"

	"github.com/romshark/datapages/example/classifieds/app/domain"
	"github.com/romshark/datapages/example/classifieds/datapagesgen/href"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

// PageSettings is /settings
type PageSettings struct {
	App *App
	Base
}

func (p PageSettings) render(
	ctx context.Context, session Session,
) (templ.Component, error) {
	u, err := p.App.repo.UserByID(ctx, session.UserID)
	if err != nil {
		return nil, err
	}

	baseData, err := p.baseData(ctx, session)
	if err != nil {
		return nil, err
	}

	sessions := make(map[string]Session)
	maps.Insert(sessions, p.App.sessions.UserSessions(session.UserID))

	return pageSettings(session, sessions, u, baseData), nil
}

func (p PageSettings) GET(
	r *http.Request,
	session Session,
) (body templ.Component, redirect string, err error) {
	if session.UserID == "" {
		return nil, href.Login(), nil
	}

	sessions := make(map[string]Session)
	maps.Insert(sessions, p.App.sessions.UserSessions(session.UserID))
	body, err = p.render(r.Context(), session)
	return body, "", err
}

// POSTSave is /settings/save/{$}
func (p PageSettings) POSTSave(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
	session Session,
	signals struct {
		Username string `json:"username"`
	},
) (redirect string, err error) {
	if session.UserID == "" {
		return href.Login(), nil
	}
	// TODO
	return "", nil
}

// POSTCloseSession is /settings/close-session/{token}/{$}
func (p PageSettings) POSTCloseSession(
	r *http.Request,
	sessionToken string,
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
	if session.UserID == "" {
		return false, "", domain.ErrUnauthorized
	}
	sess, err := p.App.sessions.Session(r.Context(), path.Token)
	if err != nil {
		return false, "", err
	}
	if sess.UserID != session.UserID {
		return false, "", domain.ErrUnauthorized
	}
	// Even though closeSession=true would close the sessions, let's close it
	// explicitly before we dispatch the event to make sure it's closed before
	// we claim it is.
	if err := p.App.sessions.CloseSession(r.Context(), path.Token); err != nil {
		return false, "", err
	}
	_ = dispatch(EventSessionClosed{
		TargetUserIDs: []string{sess.UserID},
		Token:         path.Token,
	})
	if sessionToken == path.Token {
		// Closed current session
		return true, href.Login(), nil
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
	if session.UserID == "" {
		return "", domain.ErrUnauthorized
	}
	closed, err := p.App.sessions.CloseAllUserSessions(nil, session.UserID)
	if err != nil {
		return "", err
	}
	targetUsers := []string{session.UserID}
	for _, token := range closed {
		_ = dispatch(EventSessionClosed{
			TargetUserIDs: targetUsers,
			Token:         token,
		})
	}
	return href.Login(), nil
}

func (p PageSettings) OnSessionClosed(
	event EventSessionClosed,
	sse *datastar.ServerSentEventGenerator,
	sessionToken string,
	session Session,
) error {
	if event.Token == sessionToken {
		fmt.Println("CURENT SESS TERMINATED")
		if err := sse.ConsoleLog("REDIRECT TO lOGIN NOW"); err != nil {
			return err
		}
		// Current session was closed
		return sse.Redirect(href.Login())
	}
	body, err := p.render(sse.Context(), session)
	if err != nil {
		return err
	}
	return sse.PatchElementTempl(body)
}
