package app

import (
	"net/http"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/classifieds/app/domain"
	"github.com/romshark/datapages/example/classifieds/datapagesgen/href"
)

// PageMyPosts is /my-posts
type PageMyPosts struct {
	App *App
	Base
}

func (p PageMyPosts) GET(
	r *http.Request,
	session Session,
) (
	body, head datapages.Component,
	redirect datapages.Redirect,
	err error,
) {
	if session.IsGuest() {
		return nil, nil, datapages.Redirect{URL: href.PageLogin()}, nil
	}

	user, err := p.App.repo.UserByName(r.Context(), session.UserID())
	if err != nil {
		return nil, nil, redirect, err
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

	body = pageMyPosts(session, baseData, user, postsOfUser)
	head = headUser(user)
	return body, head, redirect, nil
}
