// Package throttle limits how fast bytes move through a reader.
//
// One [Limiter] is one budget, and every transfer sharing it competes for the
// same bytes per second. Slowing a read is what enforces a limit on an upload:
// the body is taken from the socket more slowly, its buffers fill and TCP
// stops the sender, which a client cannot opt out of.
package throttle

import (
	"context"
	"io"
	"sync"
	"time"
)

const (
	// burst is how much of the budget an idle limiter keeps,
	// so that a transfer does not wait before its first bytes.
	burst = 250 * time.Millisecond

	// step is the most bytes charged at once, whatever the budget allows.
	step = 32 << 10
)

// Limiter is a shared byte budget whose zero rate lets everything through.
// It's safe for concurrent use.
type Limiter struct {
	mu     sync.Mutex
	rate   int64 // bytes per second, zero is unlimited
	tokens float64
	last   time.Time
}

// NewLimiter returns a limiter of bytesPerSecond, zero for unlimited.
func NewLimiter(bytesPerSecond int64) *Limiter {
	l := &Limiter{}
	l.SetRate(bytesPerSecond)
	l.mu.Lock()
	defer l.mu.Unlock()
	// A limiter nobody has read from is idle.
	l.tokens = l.burstLocked()
	return l
}

// Rate is the current budget in bytes per second, zero for unlimited.
func (l *Limiter) Rate() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rate
}

// SetRate changes the budget of every transfer sharing this limiter.
// One already waiting for bytes it was charged keeps waiting:
// the new rate reaches it with its next read.
func (l *Limiter) SetRate(bytesPerSecond int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.refill(time.Now())
	if bytesPerSecond < 0 {
		bytesPerSecond = 0
	}
	l.rate = bytesPerSecond
	// The budget starts fresh: a debt or a surplus counted in
	// the old rate would take a different time to pay off.
	l.tokens = min(max(l.tokens, 0), l.burstLocked())
}

// Wait blocks until n bytes may pass, or until ctx ends.
func (l *Limiter) Wait(ctx context.Context, n int) error {
	d := l.reserve(n)
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// reserve charges n bytes and reports how long the caller waits for them.
// The charge goes through even when the budget is short, which makes concurrent
// transfers share the shortfall instead of racing for it.
func (l *Limiter) reserve(n int) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.rate <= 0 {
		return 0
	}
	l.refill(time.Now())
	l.tokens -= float64(n)
	if l.tokens >= 0 {
		return 0
	}
	return time.Duration(-l.tokens / float64(l.rate) * float64(time.Second))
}

func (l *Limiter) refill(now time.Time) {
	elapsed := now.Sub(l.last)
	l.last = now
	if l.rate <= 0 || elapsed <= 0 {
		return
	}
	l.tokens = min(l.tokens+elapsed.Seconds()*float64(l.rate), l.burstLocked())
}

func (l *Limiter) burstLocked() float64 {
	return float64(l.rate) * burst.Seconds()
}

// window is how many bytes one read may take. It bounds how long a single read pauses,
// which is what lets a changed limit reach a running transfer instead
// of being waited out at the old one.
func (l *Limiter) window() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.rate <= 0 {
		return step
	}
	return int(min(step, max(1, int64(l.burstLocked()))))
}

// Reader returns r paced by l.
// A read waiting for budget when ctx ends stops with the error of ctx.
func Reader(ctx context.Context, r io.Reader, l *Limiter) io.Reader {
	return &reader{ctx: ctx, r: r, l: l}
}

// ReadSeeker is [Reader] for what [net/http.ServeContent] serves.
func ReadSeeker(ctx context.Context, rs io.ReadSeeker, l *Limiter) io.ReadSeeker {
	return &readSeeker{ctx: ctx, r: rs, l: l, s: rs}
}

// reader carries the context of the transfer it paces, since [io.Reader] takes none.
// It outlives no request: the caller reads it within the one it was built for.
type reader struct {
	ctx context.Context
	r   io.Reader
	l   *Limiter
}

func (t *reader) Read(p []byte) (int, error) {
	if w := t.l.window(); len(p) > w {
		p = p[:w]
	}
	n, err := t.r.Read(p)
	if n > 0 {
		// Charged after the read: a short one must not pay for bytes that never arrived.
		if errWait := t.l.Wait(t.ctx, n); errWait != nil && err == nil {
			err = errWait
		}
	}
	return n, err
}

type readSeeker struct {
	reader
	s io.Seeker
}

func (t *readSeeker) Seek(offset int64, whence int) (int64, error) {
	return t.s.Seek(offset, whence)
}
