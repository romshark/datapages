package app

import (
	"context"
	"net/http"

	"github.com/romshark/datapages/example/classifieds/app/domain"
	"github.com/romshark/datapages/example/classifieds/datapagesgen/href"

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
	session Session,
	query struct {
		Chat string `query:"chat" reflectsignal:"chatselected"`
	},
) (
	body templ.Component,
	redirect string,
	enableBackgroundStreaming bool,
	err error,
) {
	if session.UserID == "" {
		redirect = href.Login()
		return
	}

	baseData, chats, openChat, messages, err := p.getPageData(
		r.Context(), session, query.Chat,
	)
	if err != nil {
		return
	}

	body = pageMessages(session, chats, openChat, messages, baseData)
	// When on the messages page, we want the page to be awake even in the background
	// to notify about new messages coming in.
	enableBackgroundStreaming = true
	return
}

func (p PageMessages) getPageData(
	ctx context.Context, session Session, selectedChat string,
) (base baseData, chats []Chat, openChat Chat, messages []domain.Message, err error) {
	c, err := p.App.repo.Chats(ctx, session.UserID)
	if err != nil {
		return base, chats, openChat, messages, err
	}

	chats = make([]Chat, len(c))

	for i, c := range c {
		p, err := p.App.repo.PostByID(ctx, c.PostID)
		if err != nil {
			return base, chats, openChat, messages, err
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
			chat.LastMessageSenderUserName = lastMessage.SenderUserName
			chat.LastMessageText = lastMessage.Text
		}

		chats[i] = chat
		if c.ID == selectedChat {
			openChat = chat
			messages = c.Messages
		}
	}

	base, err = p.baseData(ctx, session)
	return base, chats, openChat, messages, err
}

func (p PageMessages) getChat(
	ctx context.Context, session Session, selectedChat string,
) (domain.Post, domain.Chat, error) {
	chat, err := p.App.repo.ChatByID(ctx, selectedChat, session.UserID)
	if err != nil {
		return domain.Post{}, domain.Chat{}, err
	}

	post, err := p.App.repo.PostByID(ctx, chat.PostID)
	if err != nil {
		return domain.Post{}, domain.Chat{}, err
	}

	return post, chat, nil
}

// POSTRead is /messages/read/{$}
func (p PageMessages) POSTRead(
	r *http.Request,
	session Session,
	signals struct {
		ChatSelected string `json:"chatselected"`
	},
	query struct {
		MessageID string `query:"msgid"`
	},
	dispatch func(EventMessagingRead) error,
) error {
	post, chat, err := p.getChat(r.Context(), session, signals.ChatSelected)
	if err != nil {
		return err
	}

	var message domain.Message
	for _, m := range chat.Messages {
		if m.ID == query.MessageID {
			message = m
		}
	}
	if message.ID == "" {
		return domain.ErrMessageNotFound
	}

	if session.UserID != chat.SenderUserName && session.UserID != post.MerchantUserName {
		return domain.ErrUnauthorized
	}

	if message.SenderUserName == session.UserID {
		return domain.ErrUnauthorized
	}

	err = p.App.repo.MarkMessageRead(r.Context(), session.UserID, chat.ID, message.ID)
	if err != nil {
		return err
	}

	return dispatch(
		EventMessagingRead{
			TargetUserIDs: []string{chat.SenderUserName, post.MerchantUserName},
			ChatID:        signals.ChatSelected,
			UserID:        session.UserID,
		},
	)
}

// POSTWriting is /messages/writing/{$}
func (p PageMessages) POSTWriting(
	r *http.Request,
	session Session,
	signals struct {
		ChatSelected string `json:"chatselected"`
	},
	dispatch func(
		EventMessagingWriting,
	) error,
) error {
	post, chat, err := p.getChat(r.Context(), session, signals.ChatSelected)
	if err != nil {
		return err
	}

	if session.UserID != chat.SenderUserName && session.UserID != post.MerchantUserName {
		return domain.ErrUnauthorized
	}

	targetUsers := []string{chat.SenderUserName, post.MerchantUserName}

	return dispatch(
		EventMessagingWriting{
			TargetUserIDs: targetUsers,
			ChatID:        signals.ChatSelected,
			UserID:        session.UserID,
		},
	)
}

// POSTWritingStopped is /messages/writing-stopped/{$}
func (p PageMessages) POSTWritingStopped(
	r *http.Request,
	session Session,
	signals struct {
		ChatSelected string `json:"chatselected"`
	},
	dispatch func(
		EventMessagingWritingStopped,
	) error,
) error {
	post, chat, err := p.getChat(r.Context(), session, signals.ChatSelected)
	if err != nil {
		return err
	}

	if session.UserID != chat.SenderUserName && session.UserID != post.MerchantUserName {
		return domain.ErrUnauthorized
	}

	targetUsers := []string{chat.SenderUserName, post.MerchantUserName}

	return dispatch(
		EventMessagingWritingStopped{
			TargetUserIDs: targetUsers,
			ChatID:        signals.ChatSelected,
			UserID:        session.UserID,
		},
	)
}

// POSTSendMessage is /messages/sendmessage/{$}
func (p PageMessages) POSTSendMessage(
	r *http.Request,
	session Session,
	signals struct {
		ChatSelected string `json:"chatselected"`
		MessageText  string `json:"messagetext"`
	},
	dispatch func(
		EventMessagingWritingStopped,
		EventMessagingSent,
	) error,
	metrics MessagingChatMessagesSent,
) error {
	var targetUsers []string
	err := func() (err error) {
		defer func() {
			if err != nil {
				metrics.ChatMessagesSent.CounterAdd(1, "failure")
				return
			}
			metrics.ChatMessagesSent.CounterAdd(1, "success")
		}()

		post, chat, err := p.getChat(r.Context(), session, signals.ChatSelected)
		if err != nil {
			return err
		}

		if session.UserID != chat.SenderUserName && session.UserID != post.MerchantUserName {
			return domain.ErrUnauthorized
		}

		targetUsers = []string{chat.SenderUserName, post.MerchantUserName}

		_, err = p.App.repo.NewMessage(
			r.Context(), signals.ChatSelected, session.UserID, signals.MessageText,
		)
		if err != nil {
			return err
		}

		return nil
	}()
	if err != nil {
		return err
	}

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

func (p PageMessages) OnMessagingRead(
	event EventMessagingRead,
	sse *datastar.ServerSentEventGenerator,
	session Session,
	signals struct {
		Chat string `json:"chatselected"`
	},
) error {
	base, chats, openChat, messages, err := p.getPageData(
		sse.Context(), session, signals.Chat,
	)
	if err != nil {
		return err
	}
	return sse.PatchElementTempl(pageMessages(session, chats, openChat, messages, base))
}

func (PageMessages) OnMessagingWriting(
	event EventMessagingWriting,
	sse *datastar.ServerSentEventGenerator,
	session Session,
) error {
	return sse.MarshalAndPatchSignals(struct {
		WritingUser string `json:"writinguser"`
	}{
		WritingUser: event.UserID,
	})
}

func (PageMessages) OnMessagingWritingStopped(
	event EventMessagingWritingStopped,
	sse *datastar.ServerSentEventGenerator,
	session Session,
) error {
	return sse.MarshalAndPatchSignals(struct {
		WritingUser string `json:"writinguser"`
	}{
		WritingUser: "",
	})
}

func (p PageMessages) OnMessagingSent(
	event EventMessagingSent,
	sse *datastar.ServerSentEventGenerator,
	session Session,
	signals struct {
		Chat string `json:"chatselected"`
	},
) error {
	base, chats, openChat, messages, err := p.getPageData(
		sse.Context(), session, signals.Chat,
	)
	if err != nil {
		return err
	}
	return sse.PatchElementTempl(pageMessages(session, chats, openChat, messages, base))
}
