package stream_test

import (
	"context"
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

	"github.com/starfederation/datastar-go/datastar"
	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/modules/messaging"
	msginmem "github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/sessions"
	"github.com/romshark/datapages/runtime/httpserve"
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
		expiresIn  time.Duration // zero: the session never expires
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
		expiresIn    time.Duration // zero: the session does not expire
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
				}))
			defer srv.Close()

			req := streamRequest(t.Context())
			req.URL.Scheme, req.URL.Host, req.RequestURI = "http", srv.Listener.Addr().String(), ""
			// The client asks for gzip by default, and datastar-go ends
			// a compressed stream without the gzip trailer.
			req.Header.Set("Accept-Encoding", "identity")
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
				require.Contains(t, string(body), "location.reload()")
				require.Contains(t, string(body), "retry: 5000")
			} else {
				require.NotContains(t, string(body), "location.reload()")
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
		onOpen   func(datapages.StreamID, *datastar.ServerSentEventGenerator) error
		notifier *notifier
		wantMsg  string
	}{
		"open hook fails": {
			onOpen: func(datapages.StreamID, *datastar.ServerSentEventGenerator) error {
				return errors.New("open failed")
			},
			wantMsg: "handling stream open hook",
		},
		"open hook panics": {
			onOpen: func(datapages.StreamID, *datastar.ServerSentEventGenerator) error {
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
				sse *datastar.ServerSentEventGenerator, msg string, _ error,
			) {
				require.NotNil(t, sse, "the stream is open when %s fails", msg)
				require.NoError(t, sse.PatchSignals([]byte(`{"error":true}`)))
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
		onErr = func(
			_ http.ResponseWriter, _ *http.Request,
			_ *datastar.ServerSentEventGenerator, msg string, err error,
		) {
			t.Errorf("%s: %v", msg, err)
		}
	}
	return stream.NewHandler(core, msginmem.New(0), nil, store, m, onErr), core, m
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
	_ datapages.StreamID, _ *datastar.ServerSentEventGenerator,
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
