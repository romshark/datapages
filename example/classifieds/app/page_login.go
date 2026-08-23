package app

import (
	"errors"
	"net/http"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/classifieds/app/datapagesgen/href"
	"github.com/romshark/datapages/example/classifieds/app/domain"
)

// PageLogin is /login
type PageLogin struct{ App *App }

func (PageLogin) GET(r *http.Request, session Session) (
	body datapages.Component,
	redirect datapages.Redirect,
	disableRefreshAfterHidden datapages.DisableRefreshAfterHidden,
	err error,
) {
	if !session.IsGuest() {
		// Already logged in
		return nil, datapages.Redirect{URL: href.PageIndex()}, false, nil
	}
	return pageLogin(false), redirect, true, nil
}

// POSTSubmit is /login/submit
func (p PageLogin) POSTSubmit(
	r *http.Request,
	session Session,
	signals datapages.Signals[struct {
		EmailOrUsername string `json:"emailorusername"`
		Password        string `json:"password"`
	}],
) (
	body datapages.Component,
	redirect datapages.Redirect,
	newSession datapages.NewSession[struct{}],
	err error,
) {
	if !session.IsGuest() {
		// Already logged in.
		redirect = datapages.Redirect{
			URL:    href.PageIndex(),
			Status: http.StatusSeeOther,
		}
		return
	}
	uid, err := p.App.repo.Login(signals.Values.EmailOrUsername, signals.Values.Password)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) ||
			errors.Is(err, domain.ErrUserNotFound) {
			p.App.LoginSubmissions.WithLabelValues("failure").Inc()
			// Re-render page with feedback
			err, body = nil, pageLogin(true)
		}
		return
	}

	p.App.LoginSubmissions.WithLabelValues("success").Inc()
	newSession = datapages.NewSession[struct{}]{UserID: uid}
	redirect = datapages.Redirect{
		URL:    href.PageIndex(),
		Status: http.StatusSeeOther,
	}
	return
}
