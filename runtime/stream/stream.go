// Package stream serves the SSE event stream of a page.
// It holds the broker subscription behind the stream,
// the session that may end it and the shutdown that closes it.
//
// Application code must not import this package.
package stream

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
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
	ConnectionOpened()
	// ConnectionClosed counts down the stream the server just let go.
	ConnectionClosed()
	// Disconnect counts why a stream ended.
	// reason is "close", "client" or "shutdown".
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

// NewHandler returns a handler subscribing to broker.
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
// and returns once fn does. onOpen, onClose and fn may be nil.
//
// sessionKey names the session the stream belongs to.
// It is watched only when userID is non-empty and the handler was given a session store.
func (h *Handler) Handle(
	w http.ResponseWriter, r *http.Request,
	sessionKey, userID string,
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

	// The subscription is established before the response head goes out.
	// A client learns the stream is open by reading that head and may dispatch
	// immediately after, which must not reach the broker before this.
	ctx := r.Context()
	sub, err := h.broker.Subscribe(ctx, h.brokerMetrics, subjects...)
	if err != nil {
		// Nothing has been written yet, which lets the error carry a status.
		h.onErr(w, r, nil, "subscribing to message broker", err)
		return
	}

	sse := datastar.NewSSE(w, r, datastar.WithCompression())

	var start time.Time
	if h.metrics != nil {
		h.metrics.ConnectionOpened()
		defer h.metrics.ConnectionClosed()
		start = time.Now()
	}

	subC := sub.C()
	if onOpen != nil {
		if err := onOpen(streamID, sse); err != nil {
			sub.Close()
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
		if err := h.sessions.NotifyClosed(ctx, sessionKey, func() {
			close(sessionClosed)
		}); err != nil {
			// The open hook already ran. This stream holds whatever it took:
			// a subscription, and on a stateful page an instance.
			// The watchdog below is what usually gives those back and it does
			// not exist yet.
			sub.Close()
			if onClose != nil {
				onClose(streamID)
			}
			h.onErr(w, r, sse, "setting up session closure watcher", err)
			return
		}
	}

	go func() {
		reason := ""
		select {
		case <-sessionClosed:
			reason = "close"
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
		if onClose != nil {
			// Not the goroutine net/http recovers.
			defer func() {
				if rec := recover(); rec != nil {
					h.core.LogErr("recovering panic in stream close hook",
						fmt.Errorf("%v\n%s", rec, debug.Stack()))
				}
			}()
			onClose(streamID)
		}
	}()

	fn(streamID, sse, subC)
}
