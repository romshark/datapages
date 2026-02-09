package app

import (
	"net/http"

	"github.com/a-h/templ"
)

// PageIndex is /
type PageIndex struct {
	App *App
	Base
}

func (p PageIndex) GET(
	r *http.Request,
	session Session,
) (body templ.Component, err error) {
	mainCategories, err := p.App.repo.MainCategories(r.Context())
	if err != nil {
		return nil, err
	}
	recentlyPosted, err := p.App.repo.RecentlyPosted(r.Context())
	if err != nil {
		return nil, err
	}
	unreadChats, err := p.baseData(r.Context(), session)
	if err != nil {
		return nil, err
	}
	return pageIndex(session, mainCategories, recentlyPosted, unreadChats), nil
}
