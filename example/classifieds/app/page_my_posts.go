package app

import (
	"net/http"

	"github.com/romshark/datapages/example/classifieds/app/domain"
	"github.com/romshark/datapages/example/classifieds/datapagesgen/href"

	"github.com/a-h/templ"
)

// PageMyPosts is /my-posts
type PageMyPosts struct {
	App *App
	Base
}

func (p PageMyPosts) GET(
	r *http.Request,
	session SessionJWT,
) (
	body, head templ.Component,
	redirect Redirect,
	err error,
) {
	if session.UserID == "" {
		return nil, nil, Redirect{Target: href.Login()}, nil
	}

	user, err := p.App.repo.UserByName(r.Context(), session.UserID)
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
