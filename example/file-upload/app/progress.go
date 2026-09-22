package app

import (
	"io"
	"time"
)

// tickReader calls fn while r is read, at most once per interval and once more
// when r ends. Without it the page learns of the bytes of a chunk only when
// its request is over, which an upload limit makes a long wait.
type tickReader struct {
	r     io.Reader
	fn    func()
	every time.Duration
	last  time.Time
}

func (t *tickReader) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	if n == 0 && err == nil {
		return n, err
	}
	now := time.Now()
	if err != nil || now.Sub(t.last) >= t.every {
		t.last = now
		t.fn()
	}
	return n, err
}
