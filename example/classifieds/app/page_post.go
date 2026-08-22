package app

import (
	"errors"
	"net/http"
	"strings"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/classifieds/app/datapagesgen/href"
	"github.com/romshark/datapages/example/classifieds/app/domain"
)

// PagePost is /post/{slug}
type PagePost struct {
	App *App
	Base
}

func (p PagePost) GET(
	r *http.Request,
	session Session,
	path datapages.Path[struct {
		Slug string `path:"slug"`
	}],
) (
	body datapages.Component, head datapages.Head,
	redirect datapages.Redirect,
	err error,
) {
	if strings.TrimSpace(path.Values.Slug) == "" {
		err = domain.ErrUnauthorized
		return
	}

	post, err := p.App.repo.PostBySlug(r.Context(), path.Values.Slug)
	if err != nil {
		if errors.Is(err, domain.ErrPostNotFound) {
			// Redirect to 404 page.
			return nil, head, datapages.Redirect{URL: href.PageError404()}, nil
		}
	}

	similarPosts, err := p.App.repo.SimilarPosts(r.Context(), post.ID, 4)
	if err != nil {
		return nil, head, redirect, err
	}

	baseData, err := p.baseData(r.Context(), session)
	if err != nil {
		return nil, head, redirect, err
	}

	var chatID string
	if !session.IsGuest() {
		chat, err := p.App.repo.ChatByPostID(r.Context(), session.UserID(), post.ID)
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
	sse datapages.SSE,
	session Session,
	path datapages.Path[struct {
		Slug string `path:"slug"`
	}],
	signals datapages.Signals[struct {
		MessageText string `json:"messagetext"`
	}],
	messagingSent datapages.Dispatcher[EventMessagingSent],
) error {
	if session.IsGuest() {
		return domain.ErrUnauthorized
	}

	if strings.TrimSpace(path.Values.Slug) == "" {
		return domain.ErrUnauthorized
	}

	_ = sse.PatchElement(fragmentMessageFormSending())

	post, err := p.App.repo.PostBySlug(sse.Context(), path.Values.Slug)
	if err != nil {
		return err
	}

	if session.UserID() == post.MerchantUserName {
		return domain.ErrUnauthorized
	}

	chatID, err := p.App.repo.NewChat(
		sse.Context(), post.ID, session.UserID(), signals.Values.MessageText,
	)
	if err != nil {
		return err
	}

	for _, recipient := range []string{post.MerchantUserName, session.UserID()} {
		if err := messagingSent.Dispatch(EventMessagingSent{
			Recipient: datapages.SubjectUser(recipient),
			ChatID:    chatID,
			UserID:    session.UserID(),
		}); err != nil {
			return err
		}
	}

	return sse.PatchElement(fragmentMessageFormLinkToChat(chatID))
}

func (p PagePost) OnPostArchived(
	event EventPostArchived,
	sse datapages.SSE,
	session Session,
) error {
	return sse.ExecuteScript("location.replace(location.href);")
}
