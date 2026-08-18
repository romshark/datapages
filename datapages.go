package datapages

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

// Component is anything that renders itself, such as a templ.Component.
type Component interface {
	// Render renders the template to w.
	Render(ctx context.Context, w io.Writer) error
}

// Session is the authenticated session of the client, passed to handlers as the
// session parameter. It's read-only: return a [NewSession] to change it.
//
// Data carries whatever the application needs to keep for the duration of the
// session, use struct{} when it needs nothing:
//
//	func (p PageIndex) GET(
//		r *http.Request, session datapages.Session[struct{}],
//	) (body datapages.Component, err error)
//
// The zero value is the session of an unauthenticated client:
//
//	if session.IsGuest() {
//		return httperr.Forbidden // Unauthenticated client
//	}
type Session[Data any] struct {
	userID    string
	token     string
	issuedAt  time.Time
	expiresAt time.Time
	data      Data
}

// UserID identifies the authenticated user. It's empty for guest clients.
func (s Session[Data]) UserID() string { return s.userID }

// IsGuest reports whether the client is unauthenticated.
func (s Session[Data]) IsGuest() bool { return s.userID == "" }

// Token is the session token from the client's cookie. It's empty for guest clients.
func (s Session[Data]) Token() string { return s.token }

// IssuedAt is the time the session was created at. It's zero for guest clients.
func (s Session[Data]) IssuedAt() time.Time { return s.issuedAt }

// ExpiresAt is the time the session becomes invalid at. Datapages treats a client
// whose session has expired as unauthenticated and removes the session cookie.
// It's zero for guest clients and for sessions that never expire.
func (s Session[Data]) ExpiresAt() time.Time { return s.expiresAt }

// Data is the application-defined payload of the session.
func (s Session[Data]) Data() Data { return s.data }

// MakeSession assembles a session from its parts. It's called by generated code,
// applications return a [NewSession] instead.
func MakeSession[Data any](
	userID, token string, issuedAt, expiresAt time.Time, data Data,
) Session[Data] {
	return Session[Data]{
		userID:    userID,
		token:     token,
		issuedAt:  issuedAt,
		expiresAt: expiresAt,
		data:      data,
	}
}

// NewSession is returned by handlers as the newSession return value to sign a client in.
// Datapages generates the session token and stamps the issuance time,
// then hands the result back to handlers as a [Session].
//
//	func (p PageLogin) POSTSubmit(...) (
//		newSession datapages.NewSession[SessionData],
//		redirect datapages.Redirect,
//		err error,
//	) {
//		return datapages.NewSession[SessionData]{
//			UserID: user.ID,
//			Data:   SessionData{Name: user.Name},
//		}, datapages.Redirect{URL: href.PageIndex()}, nil
//	}
//
// A zero UserID is a no-op, no session is created.
type NewSession[Data any] struct {
	// UserID identifies the authenticated user.
	UserID string

	// ExpiresAt is the time the session becomes invalid at.
	// Leave it zero to let the session live until it's closed explicitly.
	ExpiresAt time.Time

	// Data is the application-defined payload of the session.
	Data Data
}

// Redirect is returned by handlers as the redirect return value to navigate the
// client to another URL:
//
//	func (p PageLogin) POSTSubmit(...) (
//		redirect datapages.Redirect, err error,
//	) {
//		return datapages.Redirect{URL: href.PageIndex()}, nil
//	}
//
// The zero value is a no-op, the client stays on the current page.
type Redirect struct {
	// URL is the target the client is navigated to.
	// An empty URL is a no-op, no redirect is performed.
	URL string

	// Status is the HTTP status code of the redirect response.
	// Zero, or any code that isn't a redirect status, means [net/http.StatusFound].
	//
	// Status is ignored for requests issued by a Datastar action
	// (those carrying the header "Datastar-Request: true"),
	// because they can't follow an HTTP redirect.
	// Those navigate client-side by assigning window.location instead.
	Status int
}

// SSE is the server-sent-event handle passed to action (POST/PUT/PATCH/DELETE)
// and event (OnXXX) handlers.
type SSE interface {
	// Context returns the context of the SSE stream.
	Context() context.Context

	// PatchElement patches the elements rendered by c into the DOM.
	// They are morphed by default, use [WithMode] to patch them differently.
	PatchElement(c Component, opts ...PatchOption) error

	// RemoveElement removes the elements matching the CSS selector from the DOM.
	RemoveElement(selector string) error

	// ExecuteScript runs a script on the client.
	ExecuteScript(script string) error

	// PatchSignals updates client-side signals from v, which is marshaled to JSON.
	// It overwrites the signals the client already has.
	// To send JSON as is, pass an [encoding/json.RawMessage]:
	//
	//	sse.PatchSignals(json.RawMessage(`{"count":42}`))
	PatchSignals(v any) error

	// PatchSignalsIfMissing works like [SSE.PatchSignals] but only sets the
	// signals the client doesn't have yet. The rest keep their values.
	PatchSignalsIfMissing(v any) error

	// Redirect navigates the client to url by assigning window.location.href,
	// which pushes a new browser history entry.
	// To replace the current entry instead, navigate with [SSE.ExecuteScript]:
	//
	//	sse.ExecuteScript(fmt.Sprintf("window.location.replace(%q)", url))
	Redirect(url string) error

	// Prefetch asks the browser to prefetch urls through the speculation rules API.
	// Browsers without support for it ignore the request.
	//
	// https://developer.mozilla.org/en-US/docs/Web/API/Speculation_Rules_API
	Prefetch(urls ...string) error
}

// PatchConfig is the accumulated configuration of a [SSE.PatchElement] call.
// The generated runtime translates it to the underlying Datastar options.
//
// Selector and SelectorID both name the patch target and are mutually exclusive.
// If both are set then SelectorID wins, no matter in which order the options were passed.
type PatchConfig struct {
	Selector   string
	SelectorID string
	Mode       PatchMode
}

// PatchMode determines how patched elements are applied to the DOM.
// Removal has no mode, use [SSE.RemoveElement] instead.
// Zero value is equivalent to [PatchModeOuter].
type PatchMode string

const (
	// PatchModeOuter (default) morphs the element into the existing element.
	PatchModeOuter PatchMode = "outer"

	// PatchModeInner replaces the inner HTML of the existing element.
	PatchModeInner PatchMode = "inner"

	// PatchModeReplace replaces the existing element with the new element.
	PatchModeReplace PatchMode = "replace"

	// PatchModePrepend prepends the element inside the existing element.
	PatchModePrepend PatchMode = "prepend"

	// PatchModeAppend appends the element inside the existing element.
	PatchModeAppend PatchMode = "append"

	// PatchModeBefore inserts the element before the existing element.
	PatchModeBefore PatchMode = "before"

	// PatchModeAfter inserts the element after the existing element.
	PatchModeAfter PatchMode = "after"
)

// PatchOption configures [SSE.PatchElement].
type PatchOption func(*PatchConfig)

// WithSelector targets the element(s) matching a CSS selector.
// Mutually exclusive with [WithSelectorID] (which wins if both are given).
func WithSelector(selector string) PatchOption {
	return func(c *PatchConfig) { c.Selector = selector }
}

// WithSelectorID targets the element with the given id.
// Mutually exclusive with [WithSelector] and wins if both are given.
func WithSelectorID(id string) PatchOption {
	return func(c *PatchConfig) { c.SelectorID = id }
}

// WithMode determines how the patched elements are applied to the DOM.
// Defaults to [PatchModeOuter], which morphs.
// mode must be one of the [PatchMode] constants, any other value is ignored:
//
//   - [PatchModeOuter]
//   - [PatchModeInner]
//   - [PatchModeReplace]
//   - [PatchModePrepend]
//   - [PatchModeAppend]
//   - [PatchModeBefore]
//   - [PatchModeAfter]
func WithMode(mode PatchMode) PatchOption {
	return func(c *PatchConfig) { c.Mode = mode }
}

// Sentinel errors for HTTP status codes. Return one from a handler to control
// the status code of the response, directly for a zero-alloc response or
// wrapped around the original error:
//
//	if !valid {
//		return datapages.ErrBadRequest
//	}
//	return fmt.Errorf("%w: %w", datapages.ErrBadRequest, errInvalidInput)
//
// The response body always uses the standard status text
// (for example "Bad Request" for 400), no matter what the error message says.
//
// Don't wrap multiple sentinels in one error. If you do, the first of
// ErrBadRequest, ErrForbidden, ErrNotFound, ErrConflict wins.
//
// Any other error results in 500 Internal Server Error.
var (
	ErrBadRequest = errors.New(http.StatusText(http.StatusBadRequest)) // 400
	ErrForbidden  = errors.New(http.StatusText(http.StatusForbidden))  // 403
	ErrNotFound   = errors.New(http.StatusText(http.StatusNotFound))   // 404
	ErrConflict   = errors.New(http.StatusText(http.StatusConflict))   // 409
)

// Subject is a subject segment of an event. Segment values are appended to the
// event's base subject in field order at dispatch time, one publish per dispatch:
//
//	// EventNotify is "notify"
//	type EventNotify struct {
//		Device datapages.Subject `json:"device"`
//
//		Text string `json:"text"`
//	}
//
// Dispatching it with Device "mobile" publishes to subject "notify.mobile".
//
// A subject field can be bound to a client-side Datastar signal with a signal:"<name>"
// struct tag, which subscribes the client's stream to the segment value the signal holds.
//
// All subject fields must be declared before any payload field.
type Subject string

// SubjectUser is a subject segment carrying the ID of the user the event is addressed to.
// Its stream requires authentication and delivers the event only
// to the client authenticated as that user.
//
//	// EventDirectMessage is "dm"
//	type EventDirectMessage struct {
//		Recipient datapages.SubjectUser `json:"recipient"`
//
//		Text string `json:"text"`
//	}
//
// An application with such an event must define a session type,
// since the stream subscribes with the ID of the authenticated user.
// That same binding is why the field must not carry a signal:"<name>" tag.
//
// One dispatch publishes to one subject. To address several users,
// dispatch once per user, which leaves the handler in control of
// what happens when one of the publishes fails.
type SubjectUser string

// SubjectStateID is a subject segment carrying the state ID of the tab
// the event is addressed to. It is resolved on the server side at stream
// connect from the HMAC-validated Datapages-Instance header of the
// connecting tab, so only the tab whose state ID matches the dispatched
// value receives the event.
//
//	// EventFiltersUpdated is "filters.updated"
//	type EventFiltersUpdated struct {
//		Tab datapages.SubjectStateID
//
//		Query string `json:"query"`
//	}
//
// A handler dispatching such an event takes stateID string alongside
// state *T, and a page handling it must be stateful.
// Since the segment is bound to the connecting tab, the field must not
// carry a signal:"<name>" tag, and it must be the event's only subject field.
type SubjectStateID string

// DispatchConfig is the accumulated configuration of a [Dispatch] call.
// Generated code assembles it from the [DispatchOption] values the call passes.
type DispatchConfig struct {
	// Context controls the publish: how long it may take and when it's given up on.
	// It defaults to the context of the handler that dispatches,
	// see [WithDispatchContext].
	Context context.Context
}

// DispatchOption configures a single [Dispatch] call.
type DispatchOption func(*DispatchConfig)

// WithDispatchContext publishes with ctx instead of the handler's context.
// A nil ctx is a no-op, the default is kept.
//
// Handlers rarely need this. The default is the request context in actions and
// the request context without its cancelation in stream hooks, which run while
// the stream is being torn down. Reach for it when the event is dispatched
// after the handler returned, from a goroutine that outlives it,
// or when the publish needs a deadline of its own:
//
//	// POSTInvite is /team/invite
//	func (p PageTeam) POSTInvite(
//		r *http.Request,
//		signals struct {
//			Email string `json:"email"`
//		},
//		dispatchSent datapages.Dispatch[EventInviteSent],
//	) error {
//		// The request context ends with the response, before the mail is out.
//		// This one keeps its values, drops its cancelation and sets a deadline.
//		ctx, cancel := context.WithTimeout(
//			context.WithoutCancel(r.Context()), time.Minute,
//		)
//		go func() {
//			defer cancel()
//			if err := p.App.SendInvite(ctx, signals.Email); err != nil {
//				slog.Error("sending invite", slog.Any("err", err))
//				return
//			}
//			_ = dispatchSent(
//				EventInviteSent{Email: signals.Email},
//				datapages.WithDispatchContext(ctx),
//			)
//		}()
//		return nil // Return OK immediately, dispatch event asynchronously.
//	}
func WithDispatchContext(ctx context.Context) DispatchOption {
	return func(c *DispatchConfig) {
		if ctx != nil {
			c.Context = ctx
		}
	}
}

// Dispatch publishes an event. Handlers receive it as a parameter,
// which may carry any name; the type is what makes it a dispatcher.
// One dispatcher publishes one event type, so a handler that
// publishes three declares three, and calls each as often as it needs:
//
//	func (p PageChat) POSTSend(
//		r *http.Request,
//		signals struct {
//			RoomID      string   `json:"room_id"`
//			Text        string   `json:"text"`
//			Attachments []string `json:"attachments"`
//		},
//		dispatchAttached datapages.Dispatch[EventAttachmentAdded],
//		dispatchWritingStopped datapages.Dispatch[EventWritingStopped],
//		dispatchSent datapages.Dispatch[EventMessageSent],
//	) error {
//		room, err := p.App.Room(r.Context(), signals.RoomID)
//		if err != nil {
//			return err
//		}
//		var errs []error
//		for _, name := range signals.Attachments {
//			errs = append(errs, dispatchAttached(EventAttachmentAdded{
//				Recipients: room.ParticipantIDs,
//				Name:       name,
//			}))
//		}
//		return errors.Join(append(errs,
//			dispatchWritingStopped(EventWritingStopped{
//				Recipients: room.ParticipantIDs,
//			}),
//			dispatchSent(EventMessageSent{
//				Recipients: room.ParticipantIDs,
//				Message:    signals.Text,
//			}),
//		)...)
//	}
//
// The events go out in the order the handler dispatches them,
// arguments of one call included. Nothing is atomic across them:
// a failed publish neither undoes the ones before it nor stops the ones after.
//
// The publish uses the context of the handler it's dispatched from,
// which [WithDispatchContext] overrides.
type Dispatch[Event any] func(event Event, options ...DispatchOption) error
