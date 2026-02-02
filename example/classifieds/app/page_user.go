package app

import (
	"errors"
	"net/http"

	"github.com/romshark/datapages/example/classifieds/app/domain"
	"github.com/romshark/datapages/example/classifieds/datapagesgen/href"

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
			return nil, nil, Redirect{Target: href.Error404()}, nil
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
	sse *datastar.ServerSentEventGenerator,
	session SessionJWT,
) error {
	return sse.ExecuteScript("location.replace(location.href);")
}
