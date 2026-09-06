// Package logsample provides a throttling log handler that bounds the log
// volume a repeated warning produces.
package logsample

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// DefaultLimit is how many distinct records are tracked at once.
	// A build holds a handful of mistakes, so 64 covers them with room to
	// spare while keeping the set to a few kilobytes, which is what a value
	// taken from a request cannot grow further.
	DefaultLimit = 64

	// DefaultInterval is how long a record is held back after one is written.
	// A minute is 1440 lines a day per record.
	DefaultInterval = time.Minute
)

// Handler writes one record per distinct message and attribute set per
// interval and counts what it holds back. The next record of that kind
// carries the count as "suppressed". It is safe for concurrent use.
type Handler struct {
	next     slog.Handler
	limit    int
	interval time.Duration
	prefix   string
	state    *state
}

type state struct {
	seen  sync.Map // key -> *entry
	lock  sync.Mutex
	count int
}

type entry struct {
	written    atomic.Int64 // unix nanos of the last record written
	suppressed atomic.Int64
}

// New wraps next. A limit below one means [DefaultLimit],
// an interval below one [DefaultInterval].
func New(next slog.Handler, limit int, interval time.Duration) *Handler {
	if limit < 1 {
		limit = DefaultLimit
	}
	if interval < 1 {
		interval = DefaultInterval
	}
	return &Handler{
		next:     next,
		limit:    limit,
		interval: interval,
		state:    &state{},
	}
}

func (h *Handler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l)
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	write, suppressed := h.state.admit(h.key(r), h.limit, h.interval)
	if !write {
		return nil
	}
	if suppressed > 0 {
		r.AddAttrs(slog.Int64("suppressed", suppressed))
	}
	return h.next.Handle(ctx, r)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{
		next:     h.next.WithAttrs(attrs),
		limit:    h.limit,
		interval: h.interval,
		prefix:   h.prefix + attrsKey(attrs),
		state:    h.state,
	}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{
		next:     h.next.WithGroup(name),
		limit:    h.limit,
		interval: h.interval,
		prefix:   h.prefix + name + "\x00",
		state:    h.state,
	}
}

// attrsKey renders the attributes a handler carries into its key.
func attrsKey(attrs []slog.Attr) string {
	var b strings.Builder
	for _, a := range attrs {
		b.WriteString(a.Key)
		b.WriteByte('=')
		b.WriteString(a.Value.String())
		b.WriteByte(0)
	}
	return b.String()
}

// key identifies a record by what it says. The time and the source line are
// left out, since they differ on every render.
func (h *Handler) key(r slog.Record) string {
	var b strings.Builder
	b.WriteString(h.prefix)
	b.WriteString(r.Level.String())
	b.WriteByte(0)
	b.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool {
		b.WriteByte(0)
		b.WriteString(a.Key)
		b.WriteByte('=')
		b.WriteString(a.Value.String())
		return true
	})
	return b.String()
}

// admit reports whether the record is written and how many of its kind were
// held back since the last one.
//
// A known key takes the lock-free path. sync.Map carries the keys because one
// is stored once and read on every later render, and the timestamp moves by CAS,
// which lets exactly one caller of a burst write.
func (s *state) admit(
	key string, limit int, interval time.Duration,
) (write bool, suppressed int64) {
	t := time.Now().UnixNano()
	if v, ok := s.seen.Load(key); ok {
		e := v.(*entry)
		last := e.written.Load()
		if t-last < int64(interval) || !e.written.CompareAndSwap(last, t) {
			e.suppressed.Add(1)
			return false, 0
		}
		return true, e.suppressed.Swap(0)
	}
	return s.track(key, t, limit, interval), 0
}

// track records a key the set doesn't hold yet. A full set first drops what
// has gone quiet, which is what lets a mistake found later still be reported.
func (s *state) track(key string, t int64, limit int, interval time.Duration) bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.count >= limit {
		s.evictQuiet(t, interval)
	}
	if s.count >= limit {
		return false
	}
	e := &entry{}
	e.written.Store(t)
	if _, loaded := s.seen.LoadOrStore(key, e); loaded {
		return false
	}
	s.count++
	return true
}

// evictQuiet drops every key whose last record is older than the interval.
// Such a key is due to be written again anyway, so nothing is lost but its
// suppressed count.
func (s *state) evictQuiet(t int64, interval time.Duration) {
	s.seen.Range(func(k, v any) bool {
		if t-v.(*entry).written.Load() >= int64(interval) {
			s.seen.Delete(k)
			s.count--
		}
		return true
	})
}
