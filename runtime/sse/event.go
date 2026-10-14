package sse

import (
	"bytes"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

// event formats an event in frames and returns to events through [event.free].
type event struct {
	b      []byte   // The frame being filled.
	full   [][]byte // The filled frames before b, oldest first.
	prefix string   // The start of each data line Write adds, as "data: elements ".
	open   bool     // The last data line has no line end yet.
	cr     bool     // The last byte Write saw was a CR.
	wrote  bool     // Write saw a byte.
}

// frameSize bounds pooled memory per frame. Fixed-size buffers prevent large
// events from enlarging buffers later reused by small events. See [Go issue 23199].
//
// BenchmarkHTTPEvent measured a 2 MB event at 716 us with 64 KiB frames and
// 1369 us with 16 KiB frames. 1000 blocked small events hold 66 MB at 64 KiB.
//
// [Go issue 23199]: https://go.dev/issue/23199
const frameSize = 64 << 10

var (
	events = sync.Pool{New: func() any { return &event{b: new([frameSize]byte)[:0]} }}
	frames = sync.Pool{New: func() any { return new([frameSize]byte) }}
)

func newEvent(typ string) *event {
	e := events.Get().(*event)
	e.addString("event: ")
	e.addString(typ)
	e.addByte('\n')
	return e
}

func (e *event) free() {
	first := e.b
	if len(e.full) > 0 {
		first = e.full[0]
		for _, f := range e.full[1:] {
			frames.Put((*[frameSize]byte)(f[:frameSize]))
		}
		frames.Put((*[frameSize]byte)(e.b[:frameSize]))
		// The pool may drop a frame, which e.full must not keep alive.
		clear(e.full)
	}
	*e = event{b: first[:0], full: e.full[:0]}
	events.Put(e)
}

// add appends p, continuing in a new frame when the current one is full.
func (e *event) add(p []byte) {
	if len(e.b)+len(p) <= frameSize {
		e.b = append(e.b, p...)
		return
	}
	spill(e, p)
}

// addString works like [event.add].
func (e *event) addString(s string) {
	if len(e.b)+len(s) <= frameSize {
		e.b = append(e.b, s...)
		return
	}
	spill(e, s)
}

func (e *event) addByte(c byte) {
	if len(e.b) == frameSize {
		e.next()
	}
	e.b = append(e.b, c)
}

func spill[T []byte | string](e *event, p T) {
	for {
		n := copy(e.b[len(e.b):frameSize], p)
		e.b = e.b[:len(e.b)+n]
		p = p[n:]
		if len(p) == 0 {
			return
		}
		e.next()
	}
}

// next continues in a new frame.
// Inlined, it would push [event.addByte] over the inlining budget.
//
//go:noinline
func (e *event) next() {
	e.full = append(e.full, e.b)
	e.b = frames.Get().(*[frameSize]byte)[:0]
}

// spillLine appends what [event.write] appends for line across frames.
func spillLine[T []byte | string](e *event, line T, lf bool) {
	if !e.open {
		e.addString(e.prefix)
	}
	spill(e, line)
	if lf {
		e.addByte('\n')
	}
}

// writeFrames writes the frames of e to w in order.
func (e *event) writeFrames(w io.Writer) error {
	for _, f := range e.full {
		if _, err := w.Write(f); err != nil {
			return err
		}
	}
	_, err := w.Write(e.b)
	return err
}

func (e *event) retry(d time.Duration) {
	if d <= 0 {
		return
	}
	var digits [20]byte
	e.addString("retry: ")
	e.add(strconv.AppendInt(digits[:0], d.Milliseconds(), 10))
	e.addByte('\n')
}

// field appends a data line. value must not contain CR or LF.
func (e *event) field(key, value string) {
	e.addString("data: ")
	e.addString(key)
	e.addString(value)
	e.addByte('\n')
}

// write splits p at CR, LF and CRLF so content cannot inject SSE fields.
// The client rejoins data lines with LF. HTML [normalizes newlines]
// the same way, which preserves the patched DOM and script text.
//
// [normalizes newlines]: https://infra.spec.whatwg.org/#normalize-newlines
func (e *event) write[T []byte | string](p T) (int, error) {
	n := len(p)
	if n == 0 {
		return 0, nil
	}
	e.wrote = true
	if e.cr && p[0] == '\n' {
		// Complete a CRLF split across writes.
		p = p[1:]
	}
	e.cr = false
	// [bytes.IndexByte] handles the common LF-only case.
	// [bytes.IndexAny] avoids quadratic rescanning when the content contains CRs.
	hasCR := indexByte(p, '\r') >= 0
	for len(p) > 0 {
		var i int
		if hasCR {
			i = indexAny(p, "\r\n")
		} else {
			i = indexByte(p, '\n')
		}
		line := p
		if i >= 0 {
			line = p[:i]
		}
		// 1 check of the frame for the whole line keeps the appends of
		// the common case plain. A line that does not fit spills.
		m := len(line)
		if i >= 0 {
			m++
		}
		if !e.open {
			m += len(e.prefix)
		}
		if len(e.b)+m <= frameSize {
			if !e.open {
				e.b = append(e.b, e.prefix...)
			}
			e.b = append(e.b, line...)
			if i >= 0 {
				e.b = append(e.b, '\n')
			}
		} else {
			spillLine(e, line, i >= 0)
		}
		if i < 0 {
			e.open = true
			break
		}
		e.open = false
		if p[i] == '\r' {
			switch {
			case i+1 == len(p):
				e.cr = true
			case p[i+1] == '\n':
				i++
			}
		}
		p = p[i+1:]
	}
	return n, nil
}

func (e *event) Write(p []byte) (int, error) { return e.write(p) }

// WriteString takes s as it is, without a copy to []byte.
func (e *event) WriteString(s string) (int, error) { return e.write(s) }

// indexByte calls [bytes.IndexByte] or [strings.IndexByte] for the type of p.
func indexByte[T []byte | string](p T, c byte) int {
	switch p := any(p).(type) {
	case string:
		return strings.IndexByte(p, c)
	case []byte:
		return bytes.IndexByte(p, c)
	}
	panic("unreachable")
}

// indexAny calls [bytes.IndexAny] or [strings.IndexAny] for the type of p.
func indexAny[T []byte | string](p T, chars string) int {
	switch p := any(p).(type) {
	case string:
		return strings.IndexAny(p, chars)
	case []byte:
		return bytes.IndexAny(p, chars)
	}
	panic("unreachable")
}

// end finishes the data field and event.
func (e *event) end() {
	switch {
	case e.open:
		e.addByte('\n')
	case e.wrote:
		// Preserve a trailing line break with an empty data line.
		e.addString(e.prefix)
		e.addByte('\n')
	}
	e.addByte('\n')
}
