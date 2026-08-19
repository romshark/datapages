package app

import (
	"errors"
	"net/http"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/classifieds/app/domain"
	"github.com/romshark/datapages/example/classifieds/datapagesgen/href"
)

// PageUser is /user/{name}/{$}
type PageUser struct {
	App *App
	Base
}

func (p PageUser) GET(
	r *http.Request,
	session Session,
	path datapages.Path[struct {
		Name string `path:"name"`
	}],
) (
	body, head datapages.Component,
	redirect datapages.Redirect,
	err error,
) {
	user, err := p.App.repo.UserByName(r.Context(), path.Values.Name)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			// Redirect to 404 page.
			return nil, nil, datapages.Redirect{URL: href.PageError404()}, nil
		}
	}

	postsOfUser, err := p.App.repo.SearchPosts(
		r.Context(), domain.PostSearchParams{
			MerchantName: user.Name,
		},
	)
	if err != nil {
		return nil, nil, redirect, err
	}

	baseData, err := p.baseData(r.Context(), session)
	if err != nil {
		return nil, nil, redirect, err
	}

	body = pageUser(session, baseData, user, postsOfUser)
	head = headUser(user)
	return body, head, redirect, nil
}

func (p PageUser) OnPostArchived(
	event EventPostArchived,
	sse datapages.SSE,
	session Session,
) error {
	return sse.ExecuteScript("location.replace(location.href);")
}
