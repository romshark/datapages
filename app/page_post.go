package app

import (
	"datapages/app/domain"
	"errors"
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

// PagePost is /post/{id}/{$}
type PagePost struct {
	App *App
	Base
}

func (p PagePost) GET(
	r *http.Request,
	session SessionJWT,
	path struct {
		ID string `path:"id"`
	},
) (
	body, head templ.Component,
	redirect Redirect,
	err error,
) {
	post, err := p.App.repo.PostByID(r.Context(), path.ID)
	if err != nil {
		if errors.Is(err, domain.ErrPostNotFound) {
			// Redirect to 404 page.
			return nil, nil, Redirect{Target: "/not-found"}, nil
		}
	}

	similarPosts, err := p.App.repo.SimilarPosts(r.Context(), path.ID, 4)
	if err != nil {
		return nil, nil, redirect, err
	}

	baseData, err := p.baseData(r.Context(), session)
	if err != nil {
		return nil, nil, redirect, err
	}

	body = pagePost(session, post, similarPosts, baseData)
	head = headPost(post.Title, post.Description, post.ImageURL)
	return body, head, redirect, nil
}

func (p PagePost) OnPostArchived(
	sse *datastar.ServerSentEventGenerator,
	event EventPostArchived,
	session SessionJWT,
) error {
	return sse.ExecuteScript("location.replace(location.href);")
}
