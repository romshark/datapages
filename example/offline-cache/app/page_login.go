package app

import (
	"errors"
	"net/http"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/offline-cache/app/domain"
	"github.com/romshark/datapages/example/offline-cache/datapagesgen/href"
)

// PageLogin is /login
type PageLogin struct{ App *App }

func (PageLogin) GET(
	r *http.Request,
	session Session,
	pageCache datapages.PageCacheWriter,
	query datapages.Query[struct {
		Next string `query:"next"`
	}],
) (
	body datapages.Component,
	redirect datapages.Redirect,
	disableRefreshAfterHidden datapages.DisableRefreshAfterHidden,
	err error,
) {
	if !session.IsGuest() {
		// Already logged in.
		return nil, datapages.Redirect{
			URL: href.PageIndex(href.QueryPageIndex{}),
		}, false, nil
	}

	// Sign-in needs the server, so the offline snapshot just says so. Only guests
	// reach this point, so the snapshot is stable.
	if ver := offlineCacheVersion(session, ""); pageCache.Version() != ver {
		pageCache.Set(
			href.PageLogin(href.QueryPageLogin{}),
			loginOffline(),
			ver,
		)
	}
	return pageLogin(false, query.Values.Next), datapages.Redirect{}, true, nil
}

// POSTSubmit is /login/submit
func (p PageLogin) POSTSubmit(
	r *http.Request,
	session Session,
	pageCache datapages.PageCacheWriter,
	signals datapages.Signals[struct {
		EmailOrUsername string `json:"emailorusername"`
		Password        string `json:"password"`
		Next            string `json:"next"`
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
			URL:    href.PageIndex(href.QueryPageIndex{}),
			Status: http.StatusSeeOther,
		}
		return
	}

	uid, err := p.App.repo.Login(signals.Values.EmailOrUsername, signals.Values.Password)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) ||
			errors.Is(err, domain.ErrUserNotFound) {
			// Re-render the page with feedback.
			err, body = nil, pageLogin(true, signals.Values.Next)
		}
		return
	}

	newSession = datapages.NewSession[struct{}]{UserID: uid}
	// A new identity signed in: drop the previous (guest) offline cache so no
	// stale, wrong-session page is served offline. Pages re-cache with the correct
	// navbar as they are visited; the redirect below re-bakes the landing page.
	pageCache.ClearAll()
	dest := signals.Values.Next
	if !isSafeRelativePath(dest) {
		dest = href.PageIndex(href.QueryPageIndex{})
	}
	redirect = datapages.Redirect{URL: dest, Status: http.StatusSeeOther}
	return
}

// isSafeRelativePath reports whether p is a safe in-app redirect target,
// i.e. a rooted path that is not protocol-relative ("//host").
func isSafeRelativePath(p string) bool {
	return len(p) > 1 && p[0] == '/' && p[1] != '/'
}
