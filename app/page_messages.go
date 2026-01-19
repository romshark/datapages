package app

import (
	"datapages/app/domain"
	"fmt"
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

// PageMessages is /messages
type PageMessages struct {
	App *App
	Base
}

func (p PageMessages) GET(
	r *http.Request,
	session SessionJWT,
	query struct {
		Chat string `query:"chat" reflectsignal:"selected"`
	},
) (body templ.Component, redirect Redirect, err error) {
	if session.UserID == "" {
		return nil, Redirect{Target: "/login"}, nil
	}

	c, err := p.App.repo.Chats(r.Context(), session.UserID)
	if err != nil {
		return nil, redirect, err
	}

	chats := make([]Chat, len(c))
	var openChat Chat
	var messages []domain.Message

	for i, c := range c {
		p, err := p.App.repo.PostByID(r.Context(), c.PostID)
		if err != nil {
			return nil, redirect, fmt.Errorf("gettting post %s: %w", c.PostID, err)
		}

		chat := Chat{
			ID:             c.ID,
			Title:          p.Title,
			PostSlug:       p.Slug,
			UnreadMessages: c.UnreadMessages,
		}

		// Only set last message if messages exist
		if len(c.Messages) > 0 {
			lastMessage := c.Messages[len(c.Messages)-1]
			chat.LastMessageSenderUserID = lastMessage.SenderUserID
			chat.LastMessageText = lastMessage.Text
		}

		chats[i] = chat
		if c.ID == query.Chat {
			openChat = chat
			messages = c.Messages
		}
	}

	baseData, err := p.baseData(r.Context(), session)
	if err != nil {
		return nil, redirect, err
	}

	fmt.Printf("OPEN CAHT: %#v\n", openChat)
	return pageMessages(session, chats, openChat, messages, baseData), redirect, nil
}

// POSTSendMessage is /messages/sendmessage/{$}
func (p PageMessages) POSTSendMessage(
	r *http.Request,
	session SessionJWT,
	signals struct {
		ChatSelected string `json:"chatselected"`
		MessageText  string `json:"messagetext"`
	},
	dispatch func(
		EventMessagingWritingStopped,
		EventMessagingSent,
	) error,
) error {
	chat, err := p.App.repo.ChatByID(r.Context(), signals.ChatSelected)
	if err != nil {
		return err
	}

	post, err := p.App.repo.PostByID(r.Context(), chat.PostID)
	if err != nil {
		return err
	}

	targetUsers := []string{chat.SenderUserID, post.MerchantUserID}

	return dispatch(
		EventMessagingWritingStopped{
			TargetUserIDs: targetUsers,
			ChatID:        signals.ChatSelected,
			UserID:        session.UserID,
		},
		EventMessagingSent{
			TargetUserIDs: targetUsers,
			ChatID:        signals.ChatSelected,
			UserID:        session.UserID,
		},
	)
}

func (PageMessages) OnMessagingWriting(
	sse *datastar.ServerSentEventGenerator,
	event EventMessagingWriting,
	session SessionJWT,
) error {
	// TODO
	// use SSE to patch the page
	return nil
}

func (PageMessages) OnMessagingWritingStopped(
	sse *datastar.ServerSentEventGenerator,
	event EventMessagingWritingStopped,
	session SessionJWT,
) error {
	// TODO
	// use SSE to patch the page
	return nil
}

func (PageMessages) OnMessagingSent(
	sse *datastar.ServerSentEventGenerator,
	event EventMessagingSent,
	session SessionJWT,
) error {
	// TODO
	// use SSE to patch the page
	return nil
}
