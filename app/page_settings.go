package app

import (
	"net/http"

	"github.com/a-h/templ"
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

	baseData, err := p.baseData(r.Context(), session)
	if err != nil {
		return nil, redirect, err
	}

	return pageSettings(session, baseData), redirect, nil
}

// POSTSave is /settings/save/{$}
func (p PageSettings) POSTSave(
	r *http.Request,
	session SessionJWT,
	signals struct {
		Username string `json:"username"`
	},
) (body templ.Component, redirect Redirect, err error) {
	if session.UserID == "" {
		return nil, Redirect{Target: "/login"}, nil
	}

	baseData, err := p.baseData(r.Context(), session)
	if err != nil {
		return nil, redirect, err
	}

	return pageSettings(session, baseData), redirect, nil
}
