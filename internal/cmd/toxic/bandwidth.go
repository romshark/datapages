package toxic

import (
	"context"
	"crypto/rand"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ProfileCookie identifies a browser profile. Its tabs share client bandwidth
// and per-client data-cap usage because requests do not identify their tab.
const ProfileCookie = "datapages_toxiproxy"

// profile returns the profile ID in the cookie of r, or a new ID and set true.
func profile(r *http.Request) (id string, set bool) {
	if c, err := r.Cookie(ProfileCookie); err == nil && c.Value != "" {
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

// links schedules bandwidth by browser or request, scope, stream and toxic type.
// Requests on the same link take turns by chunk.
type links struct {
	lock sync.Mutex
	// free maps a link to the time it finishes the data reserved on it.
	free    map[linkKey]time.Time
	sweepAt int
}

type linkKey struct {
	// profile identifies a per-client link. request identifies a per-request link.
	// Both are zero for a [ServerBandwidth] toxic.
	profile string
	request uint64
	// scope is the route pattern of the endpoint that overrides the toxic,
	// empty for the global one.
	scope  string
	stream Stream
	typ    Type
}

// minSweep is the number of links above which [links.reserve] drops idle ones.
// An idle link paces as a new one does. Each request of a client without cookies,
// such as curl, starts a link.
const minSweep = 64

func newLinks() *links {
	return &links{free: map[linkKey]time.Time{}, sweepAt: minSweep}
}

// reserve queues n bytes at rate KB/s on the link of key and returns the time
// the link finishes them.
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

// caps tracks per-client [DataCap] usage.
type caps struct {
	lock    sync.Mutex
	used    map[linkKey]capUse
	sweepAt int
}

type capUse struct {
	bytes int64
	last  time.Time
}

// capIdle expires unused counts once the cache exceeds minSweep.
// A client without cookies creates a new count for each request.
const capIdle = 10 * time.Minute

func newCaps() *caps { return &caps{used: map[linkKey]capUse{}, sweepAt: minSweep} }

func (c *caps) bytes(key linkKey) int64 {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.used[key].bytes
}

func (c *caps) add(key linkKey, n int64, now time.Time) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.used[key] = capUse{bytes: c.used[key].bytes + n, last: now}
	if len(c.used) > c.sweepAt {
		for k, u := range c.used {
			if now.Sub(u.last) > capIdle {
				delete(c.used, k)
			}
		}
		c.sweepAt = max(minSweep, 2*len(c.used))
	}
}

func (c *caps) reset() {
	c.lock.Lock()
	defer c.lock.Unlock()
	clear(c.used)
	c.sweepAt = minSweep
}

// pacer applies bandwidth and data-cap toxics to 1 request stream.
type pacer struct {
	f       *Front
	ctx     context.Context
	pattern string // of the endpoint of the request
	profile string
	request uint64
	stream  Stream

	// roll selects probabilistic toxics once per request.
	roll float32

	// used is the per-request [DataCap] usage.
	used int64
}

type limit struct {
	rate int64
	key  linkKey
}

// step describes the limits on the next chunk.
type step struct {
	// The slowest link in limits[:n] determines the delay.
	limits [3]limit
	n      int

	// room ends the chunk at the [DataCap] boundary.
	room int64

	// capped reports that capKey must count the chunk.
	capped bool
	capKey linkKey
}

func (s *step) add(rate int64, key linkKey) {
	s.limits[s.n] = limit{rate: rate, key: key}
	s.n++
}

// step snapshots the limits for the next chunk, which applies settings changes
// to open streams. Concurrent requests can each exceed a shared cap by 1 write
// because they read and update its usage separately.
func (p *pacer) step() step {
	toxics := p.f.toxics.Load()
	s := step{room: math.MaxInt64}
	if x, ok := p.toxic(toxics, ClientBandwidth); ok {
		s.add(x.toxic.Values[0], p.link(x, x.toxic.Values[1]))
	}
	if x, ok := p.toxic(toxics, ServerBandwidth); ok {
		s.add(x.toxic.Values[0],
			linkKey{scope: x.scope, stream: p.stream, typ: ServerBandwidth})
	}
	x, ok := p.toxic(toxics, DataCap)
	if !ok {
		return s
	}
	volume, rate, per := x.toxic.Values[0], x.toxic.Values[1], x.toxic.Values[2]
	s.capped, s.capKey = true, p.link(x, per)
	used := p.used
	if per == PerClient {
		used = p.f.caps.bytes(s.capKey)
	}
	if used < volume {
		s.room = volume - used
	} else {
		s.add(rate, s.capKey)
	}
	return s
}

func (s step) paced() bool { return s.n > 0 }

// size limits n to the slowest link's chunk and the remaining cap volume.
func (s step) size(n int) int {
	for _, l := range s.limits[:s.n] {
		n = min(n, chunkSize(l.rate))
	}
	return int(min(int64(n), s.room))
}

func (p *pacer) toxic(toxics *frontToxics, t Type) (frontToxic, bool) {
	x, ok := toxics.toxic(p.pattern, Slot{Type: t, Stream: p.stream})
	return x, ok && p.roll < x.toxic.Toxicity
}

// link returns the sharing key selected by per.
func (p *pacer) link(x frontToxic, per int64) linkKey {
	k := linkKey{scope: x.scope, stream: p.stream, typ: x.slot.Type}
	if per == PerRequest {
		k.request = p.request
	} else {
		k.profile = p.profile
	}
	return k
}

func (p *pacer) count(s step, n int) {
	switch {
	case !s.capped || n == 0:
	case s.capKey.request != 0:
		p.used += int64(n)
	default:
		p.f.caps.add(s.capKey, int64(n), time.Now())
	}
}

// wait returns after every link carries n bytes or the request ends.
func (p *pacer) wait(s step, n int) error {
	now := time.Now()
	var done time.Time
	for _, l := range s.limits[:s.n] {
		if t := p.f.links.reserve(l.key, n, l.rate, now); t.After(done) {
			done = t
		}
	}
	d := time.Until(done)
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

// pacedWriter applies downstream bandwidth and data-cap limits.
type pacedWriter struct {
	http.ResponseWriter
	p *pacer
}

func (w *pacedWriter) Write(b []byte) (int, error) {
	var written int
	for len(b) > 0 {
		s := w.p.step()
		chunk := b[:s.size(len(b))]
		if err := w.p.wait(s, len(chunk)); err != nil {
			return written, err
		}
		n, err := w.ResponseWriter.Write(chunk)
		w.p.count(s, n)
		written += n
		if err != nil {
			return written, err
		}
		if s.paced() {
			// Without a flush the server buffers the chunk.
			_ = http.NewResponseController(w.ResponseWriter).Flush()
		}
		b = b[n:]
	}
	return written, nil
}

// Unwrap returns the underlying writer for [http.ResponseController].
func (w *pacedWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// pacedBody applies upstream bandwidth and data-cap limits.
type pacedBody struct {
	io.ReadCloser
	p *pacer
}

func (b *pacedBody) Read(buf []byte) (int, error) {
	s := b.p.step()
	n, err := b.ReadCloser.Read(buf[:s.size(len(buf))])
	b.p.count(s, n)
	if n > 0 {
		if werr := b.p.wait(s, n); werr != nil {
			return n, werr
		}
	}
	return n, err
}
