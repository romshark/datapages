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

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/sessions"
	"github.com/romshark/datapages/runtime/httpserve"
	dpsse "github.com/romshark/datapages/runtime/sse"
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
	sse *dpsse.Stream, msg string, err error,
)

// Handler serves the streams of 1 server.
type Handler struct {
	core          *httpserve.Core
	broker        messaging.Broker
	brokerMetrics messaging.Metrics
	sessions      sessions.CloseNotifier
	metrics       Metrics
	onErr         ErrorHandler
	errOnStream   bool
	seq           atomic.Uint64
}

// NewHandler returns a handler that subscribes to broker.
//
// sessions ends a stream when the session it belongs to is closed.
// It may be nil, in which case no stream watches its session. metrics may be nil.
//
// errOnStream says onErr requires an event stream when sse is nil.
func NewHandler(
	core *httpserve.Core,
	broker messaging.Broker,
	brokerMetrics messaging.Metrics,
	sessions sessions.CloseNotifier,
	metrics Metrics,
	onErr ErrorHandler,
	errOnStream bool,
) *Handler {
	return &Handler{
		core:          core,
		broker:        broker,
		brokerMetrics: brokerMetrics,
		sessions:      sessions,
		metrics:       metrics,
		onErr:         onErr,
		errOnStream:   errOnStream,
	}
}

// Handle serves the stream of 1 request, subscribed to subjects,
// and returns once fn does. onOpen and onClose may be nil; fn must not be nil.
// compression is what the stream negotiates its coding from, nil for none.
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
// A panic from fn runs onClose on its way out of Handle.
//
// On shutdown, Handle runs onClose, interrupts writes and aborts with
// [http.ErrAbortHandler] to force reconnection. A closed or expired session
// receives [reloadScript]. Setup failures receive [reconnectDelay].
func (h *Handler) Handle(
	w http.ResponseWriter, r *http.Request, compression *dpsse.Compression,
	sessionKey, userID string, expiresAt time.Time,
	subjects []string,
	onOpen func(
		streamID datapages.StreamID,
		sse *dpsse.Stream,
	) error,
	onClose func(streamID datapages.StreamID),
	fn func(
		streamID datapages.StreamID,
		sse *dpsse.Stream,
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
		h.Fail(w, r, compression, "subscribing to message broker", err)
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

	sse := dpsse.Open(w, r, compression)
	intr := &interrupter{rc: http.NewResponseController(w)}
	stopInterrupt := h.core.AfterShutdown(func() { intr.interrupt() })
	// This defer runs after Close, whose blocked write the deadline ends.
	defer func() {
		stopInterrupt()
		intr.retire()
	}()
	aborting := false
	defer func() {
		// The shutdown path sets the write deadline before aborting.
		if !aborting {
			sse.Close()
		}
	}()
	if sse.Err() != nil {
		// The client left before the response headers were flushed.
		return
	}

	subC := sub.C()
	if onOpen != nil {
		if err := callOnOpen(onOpen, streamID, sse); err != nil {
			h.onErr(w, r, sse, "handling stream open hook", err)
			_ = sse.DelayReconnect(reconnectDelay)
			return
		}
	}
	// onClose gives back what onOpen took, such as a per-tab state instance.
	// The defer runs it after fn, a panic in fn included.
	defer h.runCloseHook(onClose, streamID)

	// A nil channel never fires, which is what a stream without a session selects on.
	var sessionClosed chan struct{}
	if h.sessions != nil && userID != "" {
		sessionClosed = make(chan struct{})
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		// A store may report closure more than once; closing the channel 2 times panics.
		var once sync.Once
		if err := h.sessions.NotifyClosed(ctx, sessionKey, func() {
			once.Do(func() { close(sessionClosed) })
		}); err != nil {
			h.onErr(w, r, sse, "setting up session closure watcher", err)
			_ = sse.DelayReconnect(reconnectDelay)
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

	// The watcher stores shutdown and sessionEnded before it
	// closes the subscription, which ends fn.
	var shutdown, sessionEnded atomic.Bool
	handedOff = true
	go func() {
		// [sessions.CloseNotifier.NotifyClosed] doesn't report expiry.
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
			sessionEnded.Store(true)
		case <-expired:
			reason = "expired"
			sessionEnded.Store(true)
		case <-r.Context().Done():
			reason = "client"
		case <-h.core.ShutdownCh():
			reason = "shutdown"
			shutdown.Store(true)
		}
		if h.metrics != nil {
			h.metrics.Disconnect(reason)
			h.metrics.ConnectionDuration(start)
		}
		sub.Close()
	}()

	fn(streamID, sse, subC)

	switch {
	case shutdown.Load():
		// End blocked and later writes before net/http reuses the writer.
		if intr.interrupt() {
			sse.Close()
		}
		aborting = true
		// net/http resets the connection and logs nothing for this value.
		panic(http.ErrAbortHandler)
	case sessionEnded.Load():
		_ = sse.ExecuteScriptRetry(reloadScript, reconnectDelay)
	}
}

// interrupter sets a past deadline to fail current and later writes. net/http's abort
// cannot end a write blocked on an HTTP/1.1 connection lock or HTTP/2 flow control.
// A wrapper without Unwrap may hide the deadline operation.
type interrupter struct {
	rc      *http.ResponseController
	mu      sync.Mutex
	set     bool
	retired bool // net/http may reuse the writer once Handle has returned.
}

// interrupt sets the deadline and reports whether it's set.
func (i *interrupter) interrupt() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	if !i.set && !i.retired {
		i.set = i.rc.SetWriteDeadline(time.Now()) == nil
	}
	return i.set
}

// retire keeps interrupt from touching the writer after Handle returns.
func (i *interrupter) retire() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.retired = true
}

// Fail handles a broker or StreamConfig failure before the stream opens.
// When errOnStream is set, it opens a stream and sends [reconnectDelay].
func (h *Handler) Fail(
	w http.ResponseWriter, r *http.Request, compression *dpsse.Compression,
	msg string, err error,
) {
	if !h.errOnStream {
		h.onErr(w, r, nil, msg, err)
		return
	}
	sse := dpsse.Open(w, r, compression)
	defer sse.Close()
	h.onErr(w, r, sse, msg, err)
	_ = sse.DelayReconnect(reconnectDelay)
}

// Reload answers a stream request with [reloadScript].
//
// Generated code calls it when a page rendered for a session reconnects as a guest.
// compression is the page stream's setting.
func (h *Handler) Reload(
	w http.ResponseWriter, r *http.Request, compression *dpsse.Compression,
) {
	h.core.Logger().Debug("stream request from a page whose session ended",
		slog.String("path", r.URL.Path))
	sse := dpsse.Open(w, r, compression)
	defer sse.Close()
	_ = sse.ExecuteScriptRetry(reloadScript, reconnectDelay)
}

// reconnectDelay applies after setup failure or session end.
//
// 5s limits a failing StreamOpen to 12 calls per minute and exceeds the 1s
// [reloadScript] delay. Datastar retains it until the next page load.
const reconnectDelay = 5 * time.Second

// reloadScript reloads the page for the session the browser now holds.
//
// The 1s delay lets the tab that signed out follow the sign-out redirect.
// Its stream ends when the action closes the session, before the redirect
// arrives, and a reload would replace that navigation.
// history.replaceState changes a form POST history entry to GET so that
// location.reload doesn't submit the form again.
const reloadScript = `setTimeout(() => { ` +
	`try { history.replaceState(history.state, "", location.href) } catch {} ` +
	`location.reload() }, 1000)`

// callOnOpen runs the stream open hook and turns a panic in it into a
// [github.com/romshark/datapages.PanicError] for the error handler.
func callOnOpen(
	onOpen func(datapages.StreamID, *dpsse.Stream) error,
	streamID datapages.StreamID,
	sse *dpsse.Stream,
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
