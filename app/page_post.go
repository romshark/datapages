package app

import (
	"datapages/app/domain"
	"errors"
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

// PagePost is /post/{slug}/{$}
type PagePost struct {
	App *App
	Base
}

func (p PagePost) GET(
	r *http.Request,
	session SessionJWT,
	path struct {
		Slug string `path:"slug"`
	},
) (
	body, head templ.Component,
	redirect Redirect,
	err error,
) {
	post, err := p.App.repo.PostBySlug(r.Context(), path.Slug)
	if err != nil {
		if errors.Is(err, domain.ErrPostNotFound) {
			// Redirect to 404 page.
			return nil, nil, Redirect{Target: "/not-found"}, nil
		}
	}

	similarPosts, err := p.App.repo.SimilarPosts(r.Context(), post.ID, 4)
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

// POSTSendMessage is /post/{slug}/send-message/{$}
func (p PagePost) POSTSendMessage(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
	session SessionJWT,
	signals struct {
		MessageText string `json:"messagetext"`
	},
	dispatch func(EventMessagingSent) error,
) error {
	_ = sse.PatchElementTempl(fragmentMessageFormSending())

	if err := dispatch(EventMessagingSent{}); err != nil {
		return sse.PatchElementTempl(fragmentMessageForm())
	}
	chatID := "" // TODO
	return sse.PatchElementTempl(fragmentMessageFormSent(chatID))
}

func (p PagePost) OnPostArchived(
	sse *datastar.ServerSentEventGenerator,
	event EventPostArchived,
	session SessionJWT,
) error {
	return sse.ExecuteScript("location.replace(location.href);")
}
