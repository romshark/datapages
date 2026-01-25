package app

import (
	"datapages/app/domain"
	"datapages/datapagesgen/href"
	"errors"
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

// PageUser is /user/{name}/{$}
type PageUser struct {
	App *App
	Base
}

func (p PageUser) GET(
	r *http.Request,
	session SessionJWT,
	path struct {
		Name string `path:"name"`
	},
) (
	body, head templ.Component,
	redirect Redirect,
	err error,
) {
	user, err := p.App.repo.UserByName(r.Context(), path.Name)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			// Redirect to 404 page.
			return nil, nil, Redirect{Target: href.NotFound()}, nil
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
	sse *datastar.ServerSentEventGenerator,
	event EventPostArchived,
	session SessionJWT,
) error {
	return sse.ExecuteScript("location.replace(location.href);")
}
