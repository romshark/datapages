package app

import (
	"errors"
	"net/http"
	"time"

	"github.com/romshark/datapages/example/classifieds/app/domain"
	"github.com/romshark/datapages/example/classifieds/datapagesgen/href"

	"github.com/a-h/templ"
)

// PageLogin is /login
type PageLogin struct{ App *App }

func (PageLogin) GET(r *http.Request, session Session) (
	body templ.Component,
	redirect Redirect,
	disableRefreshAfterHidden bool,
	err error,
) {
	if session.UserID != "" {
		// Already logged in
		return nil, Redirect{Target: href.Index()}, false, nil
	}
	return pageLogin(false), redirect, true, nil
}

// POSTSubmit is /login/submit
func (p PageLogin) POSTSubmit(
	r *http.Request,
	session Session,
	signals struct {
		EmailOrUsername string `json:"emailorusername"`
		Password        string `json:"password"`
	},
	metrics struct {
		// Number of login submissions
		LoginSubmissions interface {
			CounterAdd(delta float64, result string)
		} `name:"login_submissions_total" subsystem:"auth"`
	},
) (
	body templ.Component,
	redirect Redirect,
	newSession Session,
	err error,
) {
	if session.UserID != "" {
		// Already logged in.
		redirect = Redirect{Target: href.Index(), Status: http.StatusSeeOther}
		return
	}
	uid, err := p.App.repo.Login(signals.EmailOrUsername, signals.Password)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) ||
			errors.Is(err, domain.ErrUserNotFound) {
			metrics.LoginSubmissions.CounterAdd(1, "failure")
			// Re-render page with feedback
			err, body = nil, pageLogin(true)
		}
		return
	}

	metrics.LoginSubmissions.CounterAdd(1, "success")
	now := time.Now()
	newSession = Session{
		UserID:   uid,
		IssuedAt: now,
	}
	redirect = Redirect{Target: href.Index(), Status: http.StatusSeeOther}
	return
}
