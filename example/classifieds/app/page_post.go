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
		err = errForbidden(domain.ErrUnauthorized)
		return
	}

	post, err := p.App.repo.PostBySlug(r.Context(), path.Values.Slug)
	if err != nil {
		if errors.Is(err, domain.ErrPostNotFound) {
			// Redirect to 404 page.
			return nil, head, datapages.Redirect{URL: href.PageError404()}, nil
		}
		return nil, head, redirect, err
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
) (err error) {
	if session.IsGuest() {
		return errForbidden(domain.ErrUnauthorized)
	}

	if strings.TrimSpace(path.Values.Slug) == "" {
		return errForbidden(domain.ErrUnauthorized)
	}

	_ = sse.PatchElement(fragmentMessageFormSending())
	defer func() {
		if err != nil {
			// RecoverError only appends a toast, which would leave the form
			// at "Sending..." with nothing to send it again.
			_ = sse.PatchElement(fragmentMessageForm(path.Values.Slug))
		}
	}()

	post, err := p.App.repo.PostBySlug(sse.Context(), path.Values.Slug)
	if err != nil {
		return err
	}

	if session.UserID() == post.MerchantUserName {
		return errForbidden(domain.ErrUnauthorized)
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

	if err := sse.PatchSignals(struct {
		MessageText string `json:"messagetext"`
	}{MessageText: ""}); err != nil {
		return err
	}
	return sse.PatchElement(fragmentMessageFormLinkToChat(chatID))
}
