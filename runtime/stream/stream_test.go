package stream_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/starfederation/datastar-go/datastar"
	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/modules/messaging"
	msginmem "github.com/romshark/datapages/modules/messaging/inmem"
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
				h, m := newHandler(t)
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

func newHandler(t *testing.T) (*stream.Handler, *metrics) {
	t.Helper()
	core, err := httpserve.NewCore(datapages.ServerConfig{}, "")
	require.NoError(t, err)
	m := &metrics{}
	h := stream.NewHandler(core, msginmem.New(0), nil, nil, m, func(
		_ http.ResponseWriter, _ *http.Request,
		_ *datastar.ServerSentEventGenerator, msg string, err error,
	) {
		t.Errorf("%s: %v", msg, err)
	})
	return h, m
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
