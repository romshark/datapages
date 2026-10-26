package toxic

import (
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// profileCookie is the cookie the [Front] tells browser profiles apart by.
// The requests of 1 profile share the rate of a [Bandwidth] toxic, as the
// requests of 1 device share its network. A request carries nothing that
// names its tab, and the tabs of a profile share their connections to the front.
// A private window has cookies of its own.
const profileCookie = "datapages_toxiproxy"

// profile returns the browser profile of r by its cookie. A request without
// the cookie starts a profile, and set reports that the response sets it.
func profile(r *http.Request) (id string, set bool) {
	if c, err := r.Cookie(profileCookie); err == nil && c.Value != "" {
		return c.Value, false
	}
	return rand.Text(), true
}

// removeCookie removes the cookie with the name from the Cookie header of h.
func removeCookie(h http.Header, name string) {
	var kept []string
	for _, line := range h["Cookie"] {
		var parts []string
		for part := range strings.SplitSeq(line, ";") {
			part = strings.TrimSpace(part)
			if k, _, _ := strings.Cut(part, "="); part != "" && k != name {
				parts = append(parts, part)
			}
		}
		if len(parts) > 0 {
			kept = append(kept, strings.Join(parts, "; "))
		}
	}
	if len(kept) == 0 {
		h.Del("Cookie")
		return
	}
	h["Cookie"] = kept
}

// links holds the links that carry the data of the [Bandwidth] toxics:
// 1 per browser profile, scope and stream.
// The requests on 1 link take turns by the chunk, which shares its rate among them.
type links struct {
	lock sync.Mutex
	// free maps a link to when it is done with the data it was given.
	free    map[linkKey]time.Time
	sweepAt int
}

type linkKey struct {
	profile string
	// scope is the route pattern of the endpoint that overrides the toxic,
	// empty for the global one.
	scope  string
	stream Stream
}

// minSweep is the number of links above which [links.reserve] drops those
// that are done. A link that is done paces as a new one does, which makes
// dropping it invisible. Every request of a client without cookies,
// such as curl, starts a link.
const minSweep = 64

func newLinks() *links {
	return &links{free: map[linkKey]time.Time{}, sweepAt: minSweep}
}

// reserve returns when the link of key is done carrying n bytes at rate KB/s,
// after the data it was given before.
func (l *links) reserve(key linkKey, n int, rate int64, now time.Time) time.Time {
	l.lock.Lock()
	defer l.lock.Unlock()
	start := l.free[key]
	if start.Before(now) {
		start = now
	}
	// A KB is 1000 bytes, as for Toxiproxy, which makes the rate bytes per ms.
	done := start.Add(time.Duration(n) * time.Millisecond / time.Duration(rate))
	l.free[key] = done
	if len(l.free) > l.sweepAt {
		for k, t := range l.free {
			if !t.After(now) {
				delete(l.free, k)
			}
		}
		l.sweepAt = max(minSweep, 2*len(l.free))
	}
	return done
}

// chunkTime is the time a link takes to carry 1 chunk.
// A shorter one takes turns more often and wakes more timers.
const chunkTime = 20 * time.Millisecond

// chunkSize returns the bytes a link at rate KB/s carries in chunkTime.
func chunkSize(rate int64) int {
	return max(1, int(rate*chunkTime.Milliseconds()))
}

// pacer paces 1 stream of 1 request by the [Bandwidth] toxic that applies to it.
type pacer struct {
	f       *Front
	ctx     context.Context
	pattern string // of the endpoint of the request
	profile string
	stream  Stream

	// roll decides the toxicity once for the request: the toxic applies
	// while its toxicity is above roll.
	roll float32
}

// rate returns the rate of the toxic that applies and the key of its link,
// ok false for none. It reads the settings for every chunk, which applies
// a change to an open response, such as the stream of a page.
func (p *pacer) rate() (rate int64, key linkKey, ok bool) {
	x, ok := p.f.toxics.Load().toxic(p.pattern, Slot{Type: Bandwidth, Stream: p.stream})
	if !ok || p.roll >= x.toxic.Toxicity {
		return 0, linkKey{}, false
	}
	return x.toxic.Values[0], linkKey{
		profile: p.profile,
		scope:   x.scope,
		stream:  p.stream,
	}, true
}

// wait returns once the link of key carried n bytes at rate KB/s, or the error
// of the request when it ends first.
func (p *pacer) wait(key linkKey, n int, rate int64) error {
	d := time.Until(p.f.links.reserve(key, n, rate, time.Now()))
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-p.ctx.Done():
		return p.ctx.Err()
	case <-t.C:
		return nil
	}
}

// pacedWriter writes the response to the browser at the rate of the
// downstream [Bandwidth] toxic.
type pacedWriter struct {
	http.ResponseWriter
	p *pacer
}

func (w *pacedWriter) Write(b []byte) (int, error) {
	var written int
	for len(b) > 0 {
		rate, key, ok := w.p.rate()
		if !ok {
			n, err := w.ResponseWriter.Write(b)
			return written + n, err
		}
		chunk := b[:min(len(b), chunkSize(rate))]
		if err := w.p.wait(key, len(chunk), rate); err != nil {
			return written, err
		}
		n, err := w.ResponseWriter.Write(chunk)
		written += n
		if err != nil {
			return written, err
		}
		// The server buffers what it writes, which would hold back the chunk.
		_ = http.NewResponseController(w.ResponseWriter).Flush()
		b = b[n:]
	}
	return written, nil
}

// Unwrap returns the writer of the response, which [http.ResponseController] flushes.
func (w *pacedWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// pacedBody reads the request body from the browser at the rate of the
// upstream [Bandwidth] toxic.
type pacedBody struct {
	io.ReadCloser
	p *pacer
}

func (b *pacedBody) Read(buf []byte) (int, error) {
	rate, key, ok := b.p.rate()
	if !ok {
		return b.ReadCloser.Read(buf)
	}
	n, err := b.ReadCloser.Read(buf[:min(len(buf), chunkSize(rate))])
	if n > 0 {
		if werr := b.p.wait(key, n, rate); werr != nil {
			return n, werr
		}
	}
	return n, err
}
