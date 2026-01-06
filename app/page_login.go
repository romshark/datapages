package app

import (
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

// PageLogin is /login
type PageLogin struct{ App *App }

func (PageLogin) GET(r *http.Request, session SessionJWT) (
	body templ.Component, redirect Redirect, err error,
) {
	if session.UserID != "" {
		// Already logged in
		return nil, Redirect{Target: "/"}, nil
	}
	return pageLogin(false), redirect, nil
}

// POSTSubmit is /login/submit/{$}
func (PageLogin) POSTSubmit(
	_ *http.Request,
	sse *datastar.ServerSentEventGenerator,
	setSessionJWT func(userID string, expire time.Time, claims map[string]any),
	signals struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	},
) error {
	if signals.Email != "user@test.net" || signals.Password != "testuser" {
		return sse.PatchElementTempl(pageLogin(true))
	}
	now := time.Now()
	setSessionJWT("testuser", now.Add(24*time.Hour), nil)
	return sse.Redirect("/")
}
