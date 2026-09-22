package app

import (
	"io"
	"net/http"
	"strings"
	"time"
)

// ChunkIdleTimeout is how long a chunk may deliver nothing before the server
// gives up on it.
const ChunkIdleTimeout = 30 * time.Second

// ChunkDeadline moves the read deadline of a chunk request forward while its
// bytes arrive.
//
// A server sets one deadline for a whole request, which an upload that is slow
// by design outlives: a megabyte at the lowest limit the page offers takes a
// quarter of an hour, and the transfer dies with "i/o timeout" halfway
// through. Refreshing on every read turns the total into an idle deadline,
// which still ends a connection that stopped sending.
func ChunkDeadline(idle time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPut ||
				!strings.HasPrefix(r.URL.Path, chunkPathPrefix) {
				next.ServeHTTP(w, r)
				return
			}
			rc := http.NewResponseController(w)
			if err := rc.SetReadDeadline(time.Now().Add(idle)); err == nil {
				r.Body = idleBody{body: r.Body, rc: rc, idle: idle}
			}
			next.ServeHTTP(w, r)
		})
	}
}

type idleBody struct {
	body io.ReadCloser
	rc   *http.ResponseController
	idle time.Duration
}

func (b idleBody) Read(p []byte) (int, error) {
	n, err := b.body.Read(p)
	if n > 0 {
		_ = b.rc.SetReadDeadline(time.Now().Add(b.idle))
	}
	return n, err
}

func (b idleBody) Close() error { return b.body.Close() }
