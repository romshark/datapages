package toxic

import (
	"bufio"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestProxiesPerEndpoint tests that every endpoint has a proxy of its own,
// which keeps its address while its overrides come and go, and stops with the endpoint.
// A request that matches no endpoint passes through the global proxy.
func TestProxiesPerEndpoint(t *testing.T) {
	// Nothing connects, which leaves the upstream undialed.
	p, err := newProxies("127.0.0.1:1")
	require.NoError(t, err)
	t.Cleanup(p.close)
	both := []string{"GET /", "GET /a/{$}"}

	require.NoError(t, p.apply(Settings{}, both))
	root, a := p.addr("GET /"), p.addr("GET /a/{$}")
	require.NotEqual(t, p.globalAddr, root)
	require.NotEqual(t, p.globalAddr, a)
	require.NotEqual(t, root, a)
	require.Equal(t, p.globalAddr, p.addr("GET /none/{$}"))

	require.NoError(t, p.apply(Settings{Endpoints: map[string]Set{
		"GET /a/{$}": {latencyDown: toxicWith(100)},
	}}, both))
	require.Equal(t, a, p.addr("GET /a/{$}"))
	require.NotNil(t, p.endpoints["GET /a/{$}"].Toxics.GetToxic(latencyDown.Name()))
	require.Nil(t, p.endpoints["GET /"].Toxics.GetToxic(latencyDown.Name()))

	require.NoError(t, p.apply(Settings{}, []string{"GET /a/{$}"}))
	require.Equal(t, a, p.addr("GET /a/{$}"))
	require.Nil(t, p.endpoints["GET /a/{$}"].Toxics.GetToxic(latencyDown.Name()))
	require.Equal(t, p.globalAddr, p.addr("GET /"), "the app no longer has GET /")
	require.Len(t, p.endpoints, 1)
}

// TestFrontOverrideOpenStream tests that a change of the overrides of an
// endpoint applies to its open stream: a reset added for the endpoint ends the
// stream at its next event, and a toxic removed from it leaves the stream open.
func TestFrontOverrideOpenStream(t *testing.T) {
	const stream = "GET /_$/{$}"
	for name, tc := range map[string]struct {
		before  map[string]Set
		after   map[string]Set
		wantCut bool
	}{
		"add": {
			after: map[string]Set{
				stream: {{Type: ResetPeer, Stream: Downstream}: toxicWith(0)},
			},
			wantCut: true,
		},
		"remove": {
			before: map[string]Set{stream: {latencyDown: toxicWith(1)}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			f, base := startFrontWith(t, Config{
				Endpoints: []Endpoint{
					{Pattern: stream, Owner: "PageIndex", Handler: "GETStream"},
				},
				Settings: Settings{Endpoints: tc.before},
			}, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				for {
					if _, err := io.WriteString(w, "data: x\n\n"); err != nil {
						return
					}
					_ = http.NewResponseController(w).Flush()
					select {
					case <-r.Context().Done():
						return
					case <-time.After(20 * time.Millisecond):
					}
				}
			})

			resp, err := http.Get(base + "/_$/")
			require.NoError(t, err)
			done := make(chan struct{})
			t.Cleanup(func() {
				close(done)
				_ = resp.Body.Close()
			})
			// events receives nil for each event, then the error that ended the stream,
			// io.ErrUnexpectedEOF when it ended cleanly. The stream of the upstream
			// doesn't end, so either way the front ended it.
			events := make(chan error)
			send := func(err error) bool {
				select {
				case events <- err:
					return true
				case <-done:
					return false
				}
			}
			go func() {
				sc := bufio.NewScanner(resp.Body)
				for sc.Scan() {
					if sc.Text() == "" && !send(nil) {
						return
					}
				}
				err := sc.Err()
				if err == nil {
					err = io.ErrUnexpectedEOF
				}
				send(err)
			}()
			require.NoError(t, <-events, "the stream opened")

			require.NoError(t, f.Update(func(s *Settings) error {
				s.Endpoints = tc.after
				return nil
			}))
			deadline := time.After(5 * time.Second)
			for range 10 {
				select {
				case err := <-events:
					if err != nil {
						require.True(t, tc.wantCut, "the stream ended after the change")
						return
					}
				case <-deadline:
					t.Fatal("no event and no end of the stream")
				}
			}
			require.False(t, tc.wantCut, "the stream got 10 events after the change")
		})
	}
}
