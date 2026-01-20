package app

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

// PageSettings is /settings
type PageSettings struct {
	App *App
	Base
}

func (p PageSettings) GET(
	r *http.Request,
	session SessionJWT,
) (body templ.Component, redirect Redirect, err error) {
	if session.UserID == "" {
		return nil, Redirect{Target: "/login"}, nil
	}

	u, err := p.App.repo.UserByID(r.Context(), session.UserID)
	if err != nil {
		return nil, redirect, err
	}

	baseData, err := p.baseData(r.Context(), session)
	if err != nil {
		return nil, redirect, err
	}

	return pageSettings(session, u, baseData), redirect, nil
}

// POSTSave is /settings/save/{$}
func (p PageSettings) POSTSave(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
	session SessionJWT,
	signals struct {
		Username string `json:"username"`
	},
) error {
	if session.UserID == "" {
		return sse.Redirect("/login")
	}
	// TODO
	return nil
}
