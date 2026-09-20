package app

import (
	"errors"
	"net/http"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/offline-cache/app/datapagesgen/href"
	"github.com/romshark/datapages/example/offline-cache/app/domain"
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
		return nil, datapages.Redirect{
			URL: href.PageIndex(href.QueryPageIndex{}),
		}, false, nil
	}

	// Only guests reach this snapshot, so its session-specific version is stable.
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
			err, body = nil, pageLogin(true, signals.Values.Next)
		}
		return
	}

	newSession = datapages.NewSession[struct{}]{UserID: uid}
	// Prevent guest snapshots from being served to the signed-in user.
	// The destination and later visits repopulate the cache.
	pageCache.ClearAll()
	dest := signals.Values.Next
	if !isSafeRelativePath(dest) {
		dest = href.PageIndex(href.QueryPageIndex{})
	}
	redirect = datapages.Redirect{URL: dest, Status: http.StatusSeeOther}
	return
}

// isSafeRelativePath reports whether p is rooted but not protocol-relative.
func isSafeRelativePath(p string) bool {
	return len(p) > 1 && p[0] == '/' && p[1] != '/'
}
