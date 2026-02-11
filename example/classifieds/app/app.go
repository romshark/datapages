package app

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"time"

	"github.com/romshark/datapages/example/classifieds/app/domain"
	"github.com/romshark/datapages/example/classifieds/datapagesgen/href"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

type Session struct {
	UserID string

	IssuedAt time.Time
}

type SessionManager interface {
	Session(ctx context.Context, token string) (Session, error)
	CloseSession(ctx context.Context, token string) error
	CloseAllUserSessions(buffer []string, userID string) ([]string, error)
	UserSessions(userID string) iter.Seq2[string, Session]
}

type App struct {
	sessions SessionManager
	repo     *domain.Repository
}

func NewApp(
	sessions SessionManager,
	repo *domain.Repository,
) *App {
	return &App{
		sessions: sessions,
		repo:     repo,
	}
}

type SearchParams struct {
	Term     string `json:"term" query:"t" reflectsignal:"term"`
	Category string `json:"category" query:"c" reflectsignal:"category"`
	PriceMin int64  `json:"pmin,omitempty" query:"pmin" reflectsignal:"pmin"`
	PriceMax int64  `json:"pmax,omitempty" query:"pmax" reflectsignal:"pmax"`
	Location string `json:"location" query:"l" reflectsignal:"location"`
}

// POSTSignOut is /sign-out/{$}
func (*App) POSTSignOut(r *http.Request, _ Session) (
	closeSession bool,
	redirect string,
	err error,
) {
	return true, href.Login(), nil
}

// POSTCause500 is /cause-500-internal-error/{$}
func (*App) POSTCause500(r *http.Request) error {
	return fmt.Errorf("this is an intentional 500 internal error")
}

func (*App) Recover500(
	err error,
	sse *datastar.ServerSentEventGenerator,
) error {
	return sse.PatchElementTempl(toastError500(),
		datastar.WithSelectorID("toaster"),
		datastar.WithModeAppend())
	// Or use script execution:
	//
	// 	return sse.ExecuteScript(`
	// 		document.dispatchEvent(new CustomEvent('basecoat:toast', {
	// 			detail: {
	// 				config: {
	// 					category: 'error',
	// 					title: 'Error',
	// 					description: 'Something went wrong on our side.',
	// 					cancel: {
	// 						label: 'Dismiss'
	// 					}
	// 				}
	// 			}
	// 		}))
	// 	`)
}

// Page render funcs
func (*App) Head(r *http.Request) (body templ.Component, err error) {
	return head(), nil
}

type Chat struct {
	ID                        string
	Title                     string
	PostID                    string
	PostSlug                  string
	UnreadMessages            int
	LastMessageSenderUserName string
	LastMessageText           string
}

// Base is the main page wrapper
type Base struct{ App *App }

type baseData struct {
	UnreadChats   int
	UserAvatarURL string
}

func (b Base) baseData(
	ctx context.Context, session Session,
) (baseData, error) {
	if session.UserID == "" {
		return baseData{}, nil // Guest
	}
	unreadChats, err := b.App.repo.ChatsWithUnreadMessages(ctx, session.UserID)
	if err != nil {
		return baseData{}, fmt.Errorf(
			"fetching number of unread chats with unread messages: %w", err,
		)
	}
	user, err := b.App.repo.UserByID(ctx, session.UserID)
	if err != nil {
		return baseData{}, err
	}
	return baseData{
		UnreadChats:   unreadChats,
		UserAvatarURL: user.AvatarImageURL,
	}, nil
}

func (b Base) OnMessagingSent(
	event EventMessagingSent,
	sse *datastar.ServerSentEventGenerator,
	session Session,
) error {
	unreadChats, err := b.App.repo.ChatsWithUnreadMessages(sse.Context(), session.UserID)
	if err != nil {
		return err
	}
	if err := sse.PatchElementTempl(fragmentMessagesLink(unreadChats)); err != nil {
		return err
	}
	if err := sse.MarshalAndPatchSignals(struct {
		MessageText string `json:"messagetext"`
	}{
		MessageText: "",
	}); err != nil {
		return err
	}
	if session.UserID != event.UserID {
		return sse.ExecuteScript(`
			(() => {
				const audio = new Audio("/static/message-notification.mp3");
				audio.play();
			})();
		`)
	}
	return nil
}

func (b Base) OnMessagingRead(
	event EventMessagingRead,
	sse *datastar.ServerSentEventGenerator,
	session Session,
) error {
	unreadChats, err := b.App.repo.ChatsWithUnreadMessages(sse.Context(), session.UserID)
	if err != nil {
		return err
	}
	return sse.PatchElementTempl(fragmentMessagesLink(unreadChats))
}

// PageError404 is /not-found
type PageError404 struct {
	App *App
	Base
}

func (p PageError404) GET(
	r *http.Request,
	session Session,
) (body templ.Component, err error) {
	baseData, err := p.baseData(r.Context(), session)
	if err != nil {
		return nil, err
	}
	return pageError404(session, baseData), nil
}

// PageError500 is /whoops
type PageError500 struct{ App *App }

func (PageError500) GET(r *http.Request) (
	body templ.Component,
	disableRefreshAfterHidden bool,
	err error,
) {
	return pageError500(), true, nil
}

type MessagingChatMessagesSent struct {
	// Total number of chat message send attempts
	ChatMessagesSent interface {
		CounterAdd(delta float64, result string) // result=success|failure
	} `name:"chat_messages_sent_total" subsystem:"messaging"`
}
