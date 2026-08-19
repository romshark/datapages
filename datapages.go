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

// Signals carries the client-side Datastar signals.
// GET, action (POST/PUT/PATCH/DELETE) and StreamOpen handlers may
// receive it as a parameter.
//
// Values is a struct whose exported fields each name a signal with a json:"<name>" tag:
//
//	func (p PageChat) POSTSend(
//		r *http.Request,
//		signals datapages.Signals[struct {
//			Text string `json:"text"`
//		}],
//	) error {
//		return p.App.Send(r.Context(), signals.Values.Text)
//	}
type Signals[Values any] struct{ Values Values }

// Path carries the URL path variables of the route.
// GET and action (POST/PUT/PATCH/DELETE) handlers may receive it as a parameter.
//
// Values is a struct whose exported fields each name a route variable with a
// path:"<name>" tag:
//
//	// PagePost is /post/{slug}
//	func (p PagePost) GET(
//		r *http.Request,
//		path datapages.Path[struct {
//			Slug string `path:"slug"`
//		}],
//	) (body datapages.Component, err error) {
//		return postView(path.Values.Slug), nil
//	}
//
// Every route variable needs a field and every field needs a route variable.
type Path[Values any] struct{ Values Values }

// Query carries the URL query parameters.
// GET and action (POST/PUT/PATCH/DELETE) handlers may receive it as a parameter.
//
// Values is a struct whose exported fields each name a parameter with a
// query:"<name>" tag.
// A parameter the URL doesn't carry leaves its field at the zero value:
//
//	func (p PageSearch) GET(
//		r *http.Request,
//		query datapages.Query[struct {
//			Term string `query:"term"`
//		}],
//	) (body datapages.Component, err error) {
//		return results(query.Values.Term), nil
//	}
//
// A field can carry a reflectsignal:"<name>" tag naming a signal of the
// handler's [Signals] parameter. The query parameter gives that signal its
// value on page load, and the browser URL is rewritten whenever the signal changes:
//
//	func (p PageSearch) GET(
//		r *http.Request,
//		query datapages.Query[struct {
//			Term string `query:"term" reflectsignal:"term"`
//		}],
//		signals datapages.Signals[struct {
//			Term string `json:"term"`
//		}],
//	) (body datapages.Component, err error) {
//		return results(query.Values.Term), nil
//	}
//
// The tag value must match a json tag in the signals struct.
type Query[Values any] struct{ Values Values }

// StreamID identifies one SSE stream instance within the process.
// StreamOpen and StreamClose must receive it, event (OnXXX) handlers may:
//
//	func (p PageIndex) StreamOpen(
//		r *http.Request, streamID datapages.StreamID,
//	) error {
//		return p.App.OpenTab(streamID)
//	}
//
//	func (p PageIndex) StreamClose(
//		r *http.Request, streamID datapages.StreamID,
//	) error {
//		return p.App.CloseTab(streamID)
//	}
//
// It pairs the two hooks for the same stream. One session can hold several streams,
// one per open tab, and the ID is what tells them apart: register per-tab state under
// it in StreamOpen, read that state in the OnXXX handlers,
// and drop it in StreamClose. It also ties the log lines of one stream together.
//
// Keep it server-side and never hand it to clients.
type StreamID uint64

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
	// Each element is morphed into the element carrying its id.
	// Use [SSE.PatchElementAt] to name a target or to patch in another mode.
	PatchElement(c Component) error

	// PatchElementAt patches the elements rendered by c into the element(s)
	// matching the CSS selector, applying them in the given mode.
	// An empty selector targets by element id, like [SSE.PatchElement] does.
	// The zero PatchMode morphs using [PatchModeOuter], like [SSE.PatchElement] does.
	// Any value that is not a [PatchMode] constant is ignored.
	PatchElementAt(c Component, selectorCSS string, mode PatchMode) error

	// RemoveElement removes the elements matching the CSS selector from the DOM.
	RemoveElement(selectorCSS string) error

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

// Dispatcher publishes events of one type. Handlers receive it as a parameter,
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
//		attachmentAdded datapages.Dispatcher[EventAttachmentAdded],
//		writingStopped datapages.Dispatcher[EventWritingStopped],
//		messageSent datapages.Dispatcher[EventMessageSent],
//	) error {
//		room, err := p.App.Room(r.Context(), signals.RoomID)
//		if err != nil {
//			return err
//		}
//		var errs []error
//		for _, name := range signals.Attachments {
//			errs = append(errs, attachmentAdded.Dispatch(EventAttachmentAdded{
//				Recipients: room.ParticipantIDs,
//				Name:       name,
//			}))
//		}
//		return errors.Join(append(errs,
//			writingStopped.Dispatch(EventWritingStopped{
//				Recipients: room.ParticipantIDs,
//			}),
//			messageSent.Dispatch(EventMessageSent{
//				Recipients: room.ParticipantIDs,
//				Message:    signals.Text,
//			}),
//		)...)
//	}
//
// The events go out in the order the handler dispatches them,
// arguments of one call included. Nothing is atomic across them:
// a failed publish neither undoes the ones before it nor stops the ones after.
type Dispatcher[Event any] interface {
	// Dispatch publishes the event. Every open stream subscribed to its subject
	// receives it, and the OnXXX handler of that page runs.
	// The publish uses the handler's context: the request context in actions,
	// the request context without cancelation in stream hooks.
	Dispatch(event Event) error

	// DispatchCtx is [Dispatcher.Dispatch] with ctx for the publish.
	// Use it when the event goes out after the handler returned,
	// or when the publish needs its own deadline:
	//
	//	// POSTInvite is /team/invite
	//	func (p PageTeam) POSTInvite(
	//		r *http.Request,
	//		signals struct {
	//			Email string `json:"email"`
	//		},
	//		inviteSent datapages.Dispatcher[EventInviteSent],
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
	//			_ = inviteSent.DispatchCtx(ctx, EventInviteSent{Email: signals.Email})
	//		}()
	//		return nil // Return OK immediately, dispatch event asynchronously.
	//	}
	DispatchCtx(ctx context.Context, event Event) error
}
