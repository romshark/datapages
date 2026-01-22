package app

import (
	"datapages/app/domain"
	"errors"
	"net/http"
	"strings"

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
	if strings.TrimSpace(path.Slug) == "" {
		err = domain.ErrUnauthorized
		return
	}

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

	var chatID string
	if session.UserID != "" {
		chat, err := p.App.repo.ChatByPostID(r.Context(), session.UserID, post.ID)
		if err != nil {
			if !errors.Is(err, domain.ErrChatNotFound) {
				return body, head, redirect, err
			}
		}
		chatID = chat.ID
	}

	body = pagePost(session, post, similarPosts, baseData, chatID)
	head = headPost(post.Title, post.Description, post.ImageURL)
	return body, head, redirect, nil
}

// POSTSendMessage is /post/{slug}/send-message/{$}
func (p PagePost) POSTSendMessage(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
	session SessionJWT,
	path struct {
		Slug string `json:"slug"`
	},
	signals struct {
		MessageText string `json:"messagetext"`
	},
	dispatch func(EventMessagingSent) error,
) error {
	if session.UserID == "" {
		return domain.ErrUnauthorized
	}

	if strings.TrimSpace(path.Slug) == "" {
		return domain.ErrUnauthorized
	}

	_ = sse.PatchElementTempl(fragmentMessageFormSending())

	if err := dispatch(EventMessagingSent{}); err != nil {
		return sse.PatchElementTempl(fragmentMessageForm(path.Slug))
	}

	post, err := p.App.repo.PostBySlug(r.Context(), path.Slug)
	if err != nil {
		return err
	}

	if session.UserID == post.MerchantUserName {
		return domain.ErrUnauthorized
	}

	chatID, err := p.App.repo.NewChat(
		r.Context(), post.ID, session.UserID, signals.MessageText,
	)
	if err != nil {
		return err
	}

	if err := dispatch(EventMessagingSent{
		TargetUserIDs: []string{post.MerchantUserName, session.UserID},
	}); err != nil {
		return err
	}

	return sse.PatchElementTempl(fragmentMessageFormLinkToChat(chatID))
}

func (p PagePost) OnPostArchived(
	sse *datastar.ServerSentEventGenerator,
	event EventPostArchived,
	session SessionJWT,
) error {
	return sse.ExecuteScript("location.replace(location.href);")
}
