// Package stream serves the SSE event stream of a page.
// It holds the broker subscription behind the stream,
// the session that may end it and the shutdown that closes it.
//
// Application code must not import this package.
package stream

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/starfederation/datastar-go/datastar"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/sessions"
	"github.com/romshark/datapages/runtime/httpserve"
)

// Metrics counts what the handler does. A nil Metrics counts nothing.
type Metrics interface {
	// ConnectionOpened counts a stream the server just accepted.
	// w lets an implementation mark the request as
	// a stream rather than a request being served.
	ConnectionOpened(w http.ResponseWriter)
	// ConnectionClosed counts down the stream the server just let go.
	ConnectionClosed()
	// Disconnect counts why a stream ended.
	// reason is "close", "expired", "client" or "shutdown".
	Disconnect(reason string)
	// ConnectionDuration records how long a stream was open.
	ConnectionDuration(since time.Time)
}

// ErrorHandler answers a request the stream could not serve.
// sse is nil while nothing has been written yet.
type ErrorHandler func(
	w http.ResponseWriter, r *http.Request,
	sse *datastar.ServerSentEventGenerator, msg string, err error,
)

// Handler serves the streams of one server.
type Handler struct {
	core          *httpserve.Core
	broker        messaging.Broker
	brokerMetrics messaging.Metrics
	sessions      sessions.CloseNotifier
	metrics       Metrics
	onErr         ErrorHandler
	seq           atomic.Uint64
}

// NewHandler returns a handler that subscribes to broker.
//
// sessions ends a stream when the session it belongs to is closed.
// It may be nil, in which case no stream watches its session. metrics may be nil.
func NewHandler(
	core *httpserve.Core,
	broker messaging.Broker,
	brokerMetrics messaging.Metrics,
	sessions sessions.CloseNotifier,
	metrics Metrics,
	onErr ErrorHandler,
) *Handler {
	return &Handler{
		core:          core,
		broker:        broker,
		brokerMetrics: brokerMetrics,
		sessions:      sessions,
		metrics:       metrics,
		onErr:         onErr,
	}
}

// Handle serves the stream of one request, subscribed to subjects,
// and returns once fn does. onOpen and onClose may be nil; fn must not be nil.
//
// sessionKey names the session the stream belongs to.
// It's watched only when userID is non-empty and the handler was given a session store.
// A non-zero expiresAt ends the stream when the session expires. Without that limit,
// it could keep rendering events after later requests treat the session as a guest.
//
// onClose runs only after onOpen succeeds. A failing or panicking onOpen must
// release partial state itself, so onClose may assume initialization completed.
//
// Handle recovers panics from onClose because no other caller can report them.
// The caller owns panics from fn; generated implementations recover them.
func (h *Handler) Handle(
	w http.ResponseWriter, r *http.Request,
	sessionKey, userID string, expiresAt time.Time,
	subjects []string,
	onOpen func(
		streamID datapages.StreamID,
		sse *datastar.ServerSentEventGenerator,
	) error,
	onClose func(streamID datapages.StreamID),
	fn func(
		streamID datapages.StreamID,
		sse *datastar.ServerSentEventGenerator,
		ch <-chan messaging.Message,
	),
) {
	if !h.core.CheckDatastarRequest(w, r) {
		return
	}

	streamID := datapages.StreamID(h.seq.Add(1))

	// Subscribe before writing the response head. A client may dispatch as soon
	// as it reads the head, so the broker subscription must already exist.
	ctx := r.Context()
	sub, err := h.broker.Subscribe(ctx, h.brokerMetrics, subjects...)
	if err != nil {
		// Nothing has been written yet, which lets the error carry a status.
		h.onErr(w, r, nil, "subscribing to message broker", err)
		return
	}

	// Own the subscription until the watcher goroutine takes it over.
	// A panic before handoff would otherwise leave it in the broker forever.
	handedOff := false
	defer func() {
		if !handedOff {
			sub.Close()
		}
	}()

	sse := datastar.NewSSE(w, r, datastar.WithCompression())

	subC := sub.C()
	if onOpen != nil {
		if err := callOnOpen(onOpen, streamID, sse); err != nil {
			h.onErr(w, r, sse, "handling stream open hook", err)
			return
		}
	}

	// A nil channel never fires, which is what a stream without a session selects on.
	var sessionClosed chan struct{}
	if h.sessions != nil && userID != "" {
		sessionClosed = make(chan struct{})
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		// A store may report closure more than once; closing the channel twice panics.
		var once sync.Once
		if err := h.sessions.NotifyClosed(ctx, sessionKey, func() {
			once.Do(func() { close(sessionClosed) })
		}); err != nil {
			// The open hook already ran. This stream holds whatever it took:
			// on a stateful page an instance, which only onClose gives back.
			h.runCloseHook(onClose, streamID)
			h.onErr(w, r, sse, "setting up session closure watcher", err)
			return
		}
	}

	// Count the request as a stream only after StreamOpen and session watcher
	// setup succeed. A request rejected by StreamOpen remains ordinary.
	var start time.Time
	if h.metrics != nil {
		h.metrics.ConnectionOpened(w)
		defer h.metrics.ConnectionClosed()
		start = time.Now()
	}

	handedOff = true
	go func() {
		// [sessions.CloseNotifier.NotifyClosed] does not report expiry.
		// A store may retain an expired session until a later request or
		// [sessions.ExpiredDeleter.DeleteExpired].
		var expired <-chan time.Time
		if !expiresAt.IsZero() {
			t := time.NewTimer(time.Until(expiresAt))
			defer t.Stop()
			expired = t.C
		}
		reason := ""
		select {
		case <-sessionClosed:
			reason = "close"
		case <-expired:
			reason = "expired"
		case <-r.Context().Done():
			reason = "client"
		case <-h.core.ShutdownCh():
			reason = "shutdown"
		}
		if h.metrics != nil {
			h.metrics.Disconnect(reason)
			h.metrics.ConnectionDuration(start)
		}
		sub.Close()
	}()

	fn(streamID, sse, subC)

	// fn may drain buffered messages through handlers that need state released
	// by onClose. Run onClose synchronously after fn so [http.Server.Shutdown] waits.
	h.runCloseHook(onClose, streamID)
}

// callOnOpen runs the stream open hook and turns a panic in it into a
// [github.com/romshark/datapages.PanicError] for the error handler.
func callOnOpen(
	onOpen func(datapages.StreamID, *datastar.ServerSentEventGenerator) error,
	streamID datapages.StreamID,
	sse *datastar.ServerSentEventGenerator,
) (err error) {
	defer func() {
		if v := recover(); v != nil {
			err = datapages.PanicError{Value: v, Stack: debug.Stack()}
		}
	}()
	return onOpen(streamID, sse)
}

// runCloseHook calls onClose when non-nil. It logs instead of propagating
// a panic because the error path still owes a response and the normal path
// has already written one.
func (h *Handler) runCloseHook(
	onClose func(streamID datapages.StreamID), streamID datapages.StreamID,
) {
	if onClose == nil {
		return
	}
	defer h.recoverPanic(streamID)
	onClose(streamID)
}

// recoverPanic swallows a panic and reports it against the stream it happened on.
func (h *Handler) recoverPanic(streamID datapages.StreamID) {
	v := recover()
	if v == nil {
		return
	}
	h.core.Logger().Error("recovered panic while closing a stream",
		slog.Any("panic", v),
		slog.Uint64("stream-id", uint64(streamID)),
		slog.String("stack", string(debug.Stack())))
}
