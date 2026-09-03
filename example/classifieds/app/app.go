package app

import (
	"context"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/classifieds/app/datapagesgen/assets"
	"github.com/romshark/datapages/example/classifieds/app/datapagesgen/href"
	"github.com/romshark/datapages/example/classifieds/app/domain"
	"github.com/romshark/datapages/modules/sessions"
)

type Session = datapages.Session[struct{}]

type Metrics struct {
	LoginSubmissions *prometheus.CounterVec
	ChatMessagesSent *prometheus.CounterVec
}

// SessionRecord is what the session manager stores for a session.
type SessionRecord = sessions.Record[struct{}]

type SessionManager interface {
	// Declared by modules/sessions, so either built-in store satisfies them:
	// inmem in development, natskv in production, no source change between.
	sessions.Closer
	sessions.UserSessionCloser
	sessions.UserSessionIterator[struct{}]

	Session(ctx context.Context, token string) (SessionRecord, error)
}

type App struct {
	Metrics

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
func (*App) POSTSignOut(r *http.Request, session Session) (
	closeSession datapages.CloseSession,
	redirect datapages.Redirect,
	err error,
) {
	return true, datapages.Redirect{URL: href.PageLogin()}, nil
}

// POSTCause500 is /cause-500-internal-error/{$}
func (*App) POSTCause500(r *http.Request) error {
	return fmt.Errorf("this is an intentional 500 internal error")
}

func (*App) RecoverError(
	err error,
	sse datapages.SSE,
) error {
	return sse.PatchElementAt(toastError500(), "#toaster", datapages.PatchModeAppend)
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
func (*App) Head(r *http.Request) datapages.Head {
	return head()
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
	if session.IsGuest() {
		return baseData{}, nil // Guest
	}
	unreadChats, err := b.App.repo.ChatsWithUnreadMessages(ctx, session.UserID())
	if err != nil {
		return baseData{}, fmt.Errorf(
			"fetching number of unread chats with unread messages: %w", err,
		)
	}
	user, err := b.App.repo.UserByID(ctx, session.UserID())
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
	sse datapages.SSE,
	session Session,
) error {
	unreadChats, err := b.App.repo.ChatsWithUnreadMessages(sse.Context(), session.UserID())
	if err != nil {
		return err
	}
	if err := sse.PatchElement(fragmentMessagesLink(unreadChats)); err != nil {
		return err
	}
	if err := sse.PatchSignals(struct {
		MessageText string `json:"messagetext"`
	}{
		MessageText: "",
	}); err != nil {
		return err
	}
	if session.UserID() != event.UserID {
		return sse.ExecuteScript(fmt.Sprintf(`
			(() => {
				const audio = new Audio("%s");
				audio.play();
			})();
		`, assets.Path("message-notification.mp3")))
	}
	return nil
}

func (b Base) OnMessagingRead(
	event EventMessagingRead,
	sse datapages.SSE,
	session Session,
) error {
	unreadChats, err := b.App.repo.ChatsWithUnreadMessages(sse.Context(), session.UserID())
	if err != nil {
		return err
	}
	return sse.PatchElement(fragmentMessagesLink(unreadChats))
}

// PageError404 is /not-found
type PageError404 struct {
	App *App
	Base
}

func (p PageError404) GET(
	r *http.Request,
	session Session,
) (body datapages.Component, err error) {
	baseData, err := p.baseData(r.Context(), session)
	if err != nil {
		return nil, err
	}
	return pageError404(session, baseData), nil
}

// PageError500 is /whoops
type PageError500 struct{ App *App }

func (PageError500) GET(r *http.Request) (
	body datapages.Component,
	disableRefreshAfterHidden datapages.DisableRefreshAfterHidden,
	err error,
) {
	return pageError500(), true, nil
}
