package app

import (
	"errors"
	"net/http"
	"time"

	"github.com/a-h/templ"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/offline-cache/app/domain"
	"github.com/romshark/datapages/example/offline-cache/datapagesgen/href"
)

// PageLogin is /login
type PageLogin struct{ App *App }

func (PageLogin) GET(
	r *http.Request,
	session Session,
	offlineCache datapages.OfflineCacheWriter,
	query struct {
		Next string `query:"next"`
	},
) (
	body templ.Component,
	redirect string,
	disableRefreshAfterHidden bool,
	err error,
) {
	if session.UserID != "" {
		// Already logged in.
		return nil, href.PageIndex(href.QueryPageIndex{}), false, nil
	}

	// Sign-in needs the server, so the offline snapshot just says so. Only guests
	// reach this point, so the snapshot is stable.
	if ver := offlineCacheVersion(session, ""); offlineCache.Version() != ver {
		offlineCache.Set(
			href.PageLogin(href.QueryPageLogin{}),
			offlineDoc(loginOffline()),
			ver,
		)
	}
	return pageLogin(false, query.Next), "", true, nil
}

// POSTSubmit is /login/submit
func (p PageLogin) POSTSubmit(
	r *http.Request,
	session Session,
	offlineCache datapages.OfflineCacheWriter,
	signals struct {
		EmailOrUsername string `json:"emailorusername"`
		Password        string `json:"password"`
		Next            string `json:"next"`
	},
) (
	body templ.Component,
	redirect string,
	redirectStatus int,
	newSession Session,
	err error,
) {
	if session.UserID != "" {
		// Already logged in.
		redirect, redirectStatus = href.PageIndex(href.QueryPageIndex{}), http.StatusSeeOther
		return
	}

	uid, err := p.App.repo.Login(signals.EmailOrUsername, signals.Password)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) ||
			errors.Is(err, domain.ErrUserNotFound) {
			// Re-render the page with feedback.
			err, body = nil, pageLogin(true, signals.Next)
		}
		return
	}

	newSession = Session{UserID: uid, IssuedAt: time.Now()}
	// A new identity signed in: drop the previous (guest) offline cache so no
	// stale, wrong-session page is served offline. Pages re-cache with the correct
	// navbar as they are visited; the redirect below re-bakes the landing page.
	offlineCache.ClearAll()
	dest := signals.Next
	if !isSafeRelativePath(dest) {
		dest = href.PageIndex(href.QueryPageIndex{})
	}
	redirect, redirectStatus = dest, http.StatusSeeOther
	return
}

// isSafeRelativePath reports whether p is a safe in-app redirect target,
// i.e. a rooted path that is not protocol-relative ("//host").
func isSafeRelativePath(p string) bool {
	return len(p) > 1 && p[0] == '/' && p[1] != '/'
}
