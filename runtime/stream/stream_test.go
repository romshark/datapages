package stream_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/testlog"
	"github.com/romshark/datapages/modules/messaging"
	msginmem "github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/sessions"
	"github.com/romshark/datapages/runtime/httpserve"
	dpsse "github.com/romshark/datapages/runtime/sse"
	"github.com/romshark/datapages/runtime/stream"
)

// TestHandleSessionExpiry tests when the stream of a signed-in client ends by
// itself: at its session's ExpiresAt, when a request carrying the session
// starts being served as a guest, and never for a zero ExpiresAt.
func TestHandleSessionExpiry(t *testing.T) {
	// leave is when the client closes the stream,
	// long after any expiry the rows set.
	const leave = 365 * 24 * time.Hour

	for name, tt := range map[string]struct {
		expiresIn  time.Duration // 0: the session never expires
		wantOpen   time.Duration
		wantReason string
	}{
		"expires": {
			expiresIn: time.Hour, wantOpen: time.Hour, wantReason: "expired",
		},
		"already expired": {
			expiresIn: -time.Minute, wantOpen: 0, wantReason: "expired",
		},
		"never expires": {
			wantOpen: leave, wantReason: "client",
		},
	} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h, _, m := newHandler(t, nil, nil)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()

				opened := time.Now()
				var expiresAt time.Time
				if tt.expiresIn != 0 {
					expiresAt = opened.Add(tt.expiresIn)
				}
				var ended time.Time
				done := make(chan struct{})
				go func() {
					defer close(done)
					h.Handle(httptest.NewRecorder(), streamRequest(ctx),
						"token", "alice", expiresAt,
						[]string{"notice.alice"}, nil, nil, drain)
					ended = time.Now()
				}()

				time.Sleep(leave)
				cancel()
				<-done

				require.Equal(t, tt.wantOpen, ended.Sub(opened),
					"how long the stream stayed open")
				require.Equal(t, []string{tt.wantReason}, m.reasons())
			})
		})
	}
}

// TestHandleEnds tests what the page receives when its stream ends, after the
// close hook ran. Shutdown aborts the stream, which pages from earlier releases
// need to reconnect. A session that expires or closes ends it normally with a
// page reload and a reconnect delay longer than the reload's.
func TestHandleEnds(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		expiresIn    time.Duration // 0: the session does not expire
		closeSession bool
		wantErr      error
		wantReload   bool
	}{
		"shutdown":        {wantErr: io.ErrUnexpectedEOF},
		"session expired": {expiresIn: 50 * time.Millisecond, wantReload: true},
		"session closed":  {closeSession: true, wantReload: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			n := &notifier{closed: make(chan struct{})}
			h, core, _ := newHandler(t, n, nil)
			var closed atomic.Bool
			srv := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					var expiresAt time.Time
					if tt.expiresIn != 0 {
						expiresAt = time.Now().Add(tt.expiresIn)
					}
					h.Handle(w, r, "token", "alice", expiresAt,
						[]string{"notice.alice"}, nil,
						func(datapages.StreamID) { closed.Store(true) }, drain)
				},
			))
			defer srv.Close()

			req := streamRequest(t.Context())
			req.URL.Scheme, req.URL.Host, req.RequestURI = "http", srv.Listener.Addr().String(), ""
			resp, err := srv.Client().Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			require.Equal(t, http.StatusOK, resp.StatusCode)

			switch {
			case tt.closeSession:
				close(n.closed)
			case tt.expiresIn == 0:
				require.NoError(t, core.Shutdown(t.Context()))
			}
			body, err := io.ReadAll(resp.Body)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.True(t, closed.Load(), "the close hook ran before the stream ended")
			if tt.wantReload {
				require.Equal(t, reloadEvent, string(body))
			} else {
				require.Empty(t, body)
			}
		})
	}
}

// TestHandleFailedOpenDelaysReconnect tests that a stream whose open hook or
// session watcher fails ends with a reconnect delay, after the error handler
// wrote to the stream. Datastar would otherwise reconnect it after 1s and run
// the failing open again.
func TestHandleFailedOpenDelaysReconnect(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		onOpen   func(datapages.StreamID, *dpsse.Stream) error
		notifier *notifier
		wantMsg  string
	}{
		"open hook fails": {
			onOpen: func(datapages.StreamID, *dpsse.Stream) error {
				return errors.New("open failed")
			},
			wantMsg: "handling stream open hook",
		},
		"open hook panics": {
			onOpen: func(datapages.StreamID, *dpsse.Stream) error {
				panic("open panicked")
			},
			wantMsg: "handling stream open hook",
		},
		"session watcher fails": {
			notifier: &notifier{err: errors.New("watch failed")},
			wantMsg:  "setting up session closure watcher",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var msgs []string
			h, _, _ := newHandler(t, tt.notifier, func(
				_ http.ResponseWriter, _ *http.Request,
				sse *dpsse.Stream, msg string, _ error,
			) {
				require.NotNil(t, sse, "the stream is open when %s fails", msg)
				require.NoError(t, sse.PatchSignals(json.RawMessage(`{"error":true}`)))
				msgs = append(msgs, msg)
			})

			w := httptest.NewRecorder()
			h.Handle(w, streamRequest(t.Context()), "token", "alice", time.Time{},
				[]string{"notice.alice"}, tt.onOpen, nil, drain)

			require.Equal(t, []string{tt.wantMsg}, msgs)
			body := w.Body.String()
			i := strings.Index(body, `"error":true`)
			require.GreaterOrEqual(t, i, 0, "the error handler's event is missing:\n%s", body)
			require.Contains(t, body[i:], "retry: 5000",
				"the reconnect delay does not follow the error handler's event")
		})
	}
}

// TestHandleFailedSubscribe tests the answer to a stream whose subscription fails.
// An error handler that answers on a stream gets one that ends with a reconnect delay,
// as after a failed open. Datastar would otherwise reconnect the 200 after 1s.
// Any other error handler gets no stream, which leaves it the status to write.
func TestHandleFailedSubscribe(t *testing.T) {
	t.Parallel()

	for name, errOnStream := range map[string]bool{"status": false, "stream": true} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			core, err := httpserve.NewCore(datapages.ServerConfig{}, "")
			require.NoError(t, err)
			errBroker := errors.New("broker down")
			var msgs []string
			h := stream.NewHandler(core, failingBroker{err: errBroker}, nil, nil, nil,
				func(
					w http.ResponseWriter, _ *http.Request,
					sse *dpsse.Stream, msg string, err error,
				) {
					require.ErrorIs(t, err, errBroker)
					msgs = append(msgs, msg)
					if sse == nil {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					require.NoError(t, sse.PatchSignals(json.RawMessage(`{"error":true}`)))
				}, errOnStream)

			w := httptest.NewRecorder()
			h.Handle(w, streamRequest(t.Context()), "", "", time.Time{},
				[]string{"notice"}, nil, nil, drain)

			require.Equal(t, []string{"subscribing to message broker"}, msgs)
			body := w.Body.String()
			if !errOnStream {
				require.Equal(t, http.StatusServiceUnavailable, w.Code)
				require.Empty(t, body)
				return
			}
			require.Equal(t, http.StatusOK, w.Code)
			i := strings.Index(body, `"error":true`)
			require.GreaterOrEqual(t, i, 0, "the error handler's event is missing:\n%s", body)
			require.Contains(t, body[i:], "retry: 5000",
				"the reconnect delay does not follow the error handler's event")
		})
	}
}

// TestHandleShutdownWithBlockedWrite tests that shutdown aborts a stream while
// another goroutine is blocked writing to it, as to a client that stopped
// reading. Waiting for that write would hold the stream until the shutdown
// timeout, since only the abort closes the connection.
func TestHandleShutdownWithBlockedWrite(t *testing.T) {
	t.Parallel()

	h, core, _ := newHandler(t, nil, nil)
	w := &blockingWriter{
		ResponseRecorder: httptest.NewRecorder(),
		entered:          make(chan struct{}),
		release:          make(chan struct{}),
	}
	defer close(w.release)
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		h.Handle(w, streamRequest(t.Context()), "", "", time.Time{},
			[]string{"notice"},
			func(_ datapages.StreamID, sse *dpsse.Stream) error {
				go func() { _ = sse.PatchSignals(json.RawMessage(`{}`)) }()
				<-w.entered
				return nil
			}, nil, drain)
	}()
	<-w.entered
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	_ = core.Shutdown(ctx)
	select {
	case v := <-done:
		require.Equal(t, http.ErrAbortHandler, v)
	case <-time.After(time.Second):
		t.Fatal("Handle has not aborted 1s after shutdown")
	}
}

// TestHandleClosesAfterPanic tests a stream whose message loop panics.
// The panic leaves Handle for its caller, net/http in a server, and the close
// hook runs on the way: it gives back what the open hook took.
func TestHandleClosesAfterPanic(t *testing.T) {
	t.Parallel()

	h, _, _ := newHandler(t, nil, nil)
	opened, closed := false, false
	require.PanicsWithValue(t, "the message loop panicked", func() {
		h.Handle(httptest.NewRecorder(), streamRequest(t.Context()),
			"", "", time.Time{}, []string{"notice"},
			func(datapages.StreamID, *dpsse.Stream) error {
				opened = true
				return nil
			},
			func(datapages.StreamID) { closed = true },
			func(datapages.StreamID, *dpsse.Stream,
				<-chan messaging.Message,
			) {
				panic("the message loop panicked")
			})
	})
	require.True(t, opened, "the open hook did not run")
	require.True(t, closed, "the close hook did not run after the panic")
}

// TestHandleCloseHookPanics tests a close hook that panics. No caller is left
// to report it, which makes Handle log it with the stack of the panic and return.
func TestHandleCloseHookPanics(t *testing.T) {
	t.Parallel()

	var log testlog.Buffer
	core, err := httpserve.NewCore(datapages.ServerConfig{Logger: log.Logger()}, "")
	require.NoError(t, err)
	h := stream.NewHandler(core, msginmem.New(0), nil, nil, nil, failOnErr(t), false)
	h.Handle(httptest.NewRecorder(), streamRequest(t.Context()),
		"", "", time.Time{}, []string{"notice"}, nil,
		func(datapages.StreamID) { panic("the close hook panicked") },
		func(
			datapages.StreamID,
			*dpsse.Stream,
			<-chan messaging.Message,
		) {
		})
	log.RequireMatch(t, `^level=ERROR msg="recovered panic while closing a stream" `+
		`panic="the close hook panicked" stream-id=1 stack="goroutine .*`+
		`TestHandleCloseHookPanics\.func`)
}

// TestHandleClientGoneBeforeOpen tests disconnect during subscription setup.
// Handle skips the open hook, closes the subscription and reports nothing.
func TestHandleClientGoneBeforeOpen(t *testing.T) {
	t.Parallel()

	core, err := httpserve.NewCore(datapages.ServerConfig{}, "")
	require.NoError(t, err)
	b := &slowBroker{Broker: msginmem.New(0), entered: make(chan struct{})}
	h := stream.NewHandler(core, b, nil, nil, nil, failOnErr(t), false)
	panicked := make(chan any, 1)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			defer func() { panicked <- recover() }()
			h.Handle(w, r, "", "", time.Time{}, []string{"notice"},
				func(datapages.StreamID, *dpsse.Stream) error {
					t.Error("the open hook ran for a client that is gone")
					return nil
				}, nil, drain)
		},
	))
	// An HTTP/2 client resets the stream when it cancels the request,
	// after which no write reaches it. Over HTTP/1.1 the first write after
	// the client closed the connection can still succeed.
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()

	ctx, disconnect := context.WithCancel(t.Context())
	req := streamRequest(ctx)
	req.URL.Scheme, req.URL.Host, req.RequestURI = "https", srv.Listener.Addr().String(), ""
	go func() {
		if resp, err := srv.Client().Do(req); err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-b.entered
	disconnect()

	require.Nil(t, <-panicked, "Handle panicked")
	_, open := <-b.sub.C()
	require.False(t, open, "the subscription is still open")
}

// TestHandleWriterWithoutFlushPanics tests that a writer that cannot flush still panics
// with the client connected. It breaks every stream, which a quiet return would hide.
func TestHandleWriterWithoutFlushPanics(t *testing.T) {
	t.Parallel()

	h, _, _ := newHandler(t, nil, nil)
	require.Panics(t, func() {
		h.Handle(struct{ http.ResponseWriter }{httptest.NewRecorder()},
			streamRequest(t.Context()), "", "", time.Time{}, []string{"notice"},
			nil, nil, drain)
	})
}

// TestReload tests that a guest on a stream that requires a session gets
// the reload event and nothing else.
func TestReload(t *testing.T) {
	t.Parallel()

	h, _, _ := newHandler(t, nil, nil)
	w := httptest.NewRecorder()
	h.Reload(w, streamRequest(t.Context()))
	require.Equal(t, reloadEvent, w.Body.String())
}

// TestReloadClientGone tests a client that disconnects before the response head
// reaches it. Reload writes nothing.
func TestReloadClientGone(t *testing.T) {
	t.Parallel()

	h, _, _ := newHandler(t, nil, nil)
	ctx, disconnect := context.WithCancel(t.Context())
	disconnect()
	w := httptest.NewRecorder()
	h.Reload(goneWriter{w}, streamRequest(ctx))
	require.Empty(t, w.Body.String())
}

// slowBroker holds Subscribe until the request is canceled, as a broker round
// trip outlasts a client that leaves at once. It keeps the subscription in sub.
type slowBroker struct {
	messaging.Broker
	entered chan struct{}
	sub     messaging.Subscription
}

func (b *slowBroker) Subscribe(
	ctx context.Context, m messaging.Metrics, subjects ...string,
) (messaging.Subscription, error) {
	close(b.entered)
	<-ctx.Done()
	sub, err := b.Broker.Subscribe(ctx, m, subjects...)
	b.sub = sub
	return sub, err
}

// failingBroker fails every subscription with err.
type failingBroker struct {
	messaging.Broker
	err error
}

func (b failingBroker) Subscribe(
	context.Context, messaging.Metrics, ...string,
) (messaging.Subscription, error) {
	return nil, b.err
}

// blockingWriter blocks its first Write until release is closed, as a client
// that stopped reading does. It closes entered when the Write blocks.
type blockingWriter struct {
	*httptest.ResponseRecorder
	entered, release chan struct{}
	n                atomic.Int32
}

func (b *blockingWriter) Write(p []byte) (int, error) {
	if b.n.Add(1) == 1 {
		close(b.entered)
		<-b.release
	}
	return b.ResponseRecorder.Write(p)
}

// reloadEvent is the SSE event that reloads the page. Its script waits 1s,
// and its retry field holds off the reconnect for 5s, which lets the reload come first.
const reloadEvent = "event: datastar-patch-elements\n" +
	"retry: 5000\n" +
	"data: selector body\n" +
	"data: mode append\n" +
	`data: elements <script data-effect="el.remove()">setTimeout(() => { ` +
	`try { history.replaceState(history.state, "", location.href) } catch {} ` +
	`location.reload() }, 1000)</script>` + "\n\n"

// goneWriter is a disconnected client's response. Flushing it fails.
type goneWriter struct{ http.ResponseWriter }

func (goneWriter) FlushError() error { return errors.New("client gone") }

// newHandler returns a handler that watches sessions with notifier, which may be nil.
// A nil onErr fails the test on any error the handler reports.
func newHandler(
	t *testing.T, notifier *notifier, onErr stream.ErrorHandler,
) (*stream.Handler, *httpserve.Core, *metrics) {
	t.Helper()
	core, err := httpserve.NewCore(datapages.ServerConfig{}, "")
	require.NoError(t, err)
	m := &metrics{}
	// A nil *notifier in the interface would count as a session store.
	var store sessions.CloseNotifier
	if notifier != nil {
		store = notifier
	}
	if onErr == nil {
		onErr = failOnErr(t)
	}
	return stream.NewHandler(core, msginmem.New(0), nil, store, m, onErr, false), core, m
}

func failOnErr(t *testing.T) stream.ErrorHandler {
	return func(
		_ http.ResponseWriter, _ *http.Request,
		_ *dpsse.Stream, msg string, err error,
	) {
		t.Errorf("%s: %v", msg, err)
	}
}

// notifier reports each watched session as closed when the closed channel is closed.
// A non-nil err makes NotifyClosed fail instead.
type notifier struct {
	closed chan struct{}
	err    error
}

func (n *notifier) NotifyClosed(ctx context.Context, _ string, fn func()) error {
	if n.err != nil {
		return n.err
	}
	go func() {
		select {
		case <-n.closed:
			fn()
		case <-ctx.Done():
		}
	}()
	return nil
}

func streamRequest(ctx context.Context) *http.Request {
	r := httptest.NewRequestWithContext(ctx, http.MethodGet, "/_$/", nil)
	r.Header.Set("Datastar-Request", "true")
	return r
}

// drain reads the subscription until the handler closes it.
func drain(
	_ datapages.StreamID, _ *dpsse.Stream,
	ch <-chan messaging.Message,
) {
	for range ch {
	}
}

// metrics records why each stream ended.
type metrics struct {
	lock sync.Mutex
	list []string
}

func (*metrics) ConnectionOpened(http.ResponseWriter) {}
func (*metrics) ConnectionClosed()                    {}
func (*metrics) ConnectionDuration(time.Time)         {}

func (m *metrics) Disconnect(reason string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.list = append(m.list, reason)
}

func (m *metrics) reasons() []string {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.list
}
