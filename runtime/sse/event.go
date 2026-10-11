package sse

import (
	"bytes"
	"strconv"
	"sync"
	"time"
)

// event formats one event and returns to events through [event.free].
type event struct {
	b     []byte
	key   string // The key of the data lines Write adds, such as "elements ".
	open  bool   // The last data line has no line end yet.
	cr    bool   // The last byte Write saw was a CR.
	wrote bool   // Write saw a byte.
}

var events = sync.Pool{New: func() any { return new(event) }}

// maxPooledBuf matches fmt's limit and prevents rare large events
// from retaining their buffers. See [Go issue 23199].
//
// [Go issue 23199]: https://go.dev/issue/23199
const maxPooledBuf = 64 << 10

func newEvent(typ string) *event {
	e := events.Get().(*event)
	e.b = append(e.b, "event: "...)
	e.b = append(e.b, typ...)
	e.b = append(e.b, '\n')
	return e
}

func (e *event) free() {
	if cap(e.b) > maxPooledBuf {
		return
	}
	*e = event{b: e.b[:0]}
	events.Put(e)
}

func (e *event) retry(d time.Duration) {
	if d <= 0 {
		return
	}
	e.b = append(e.b, "retry: "...)
	e.b = strconv.AppendInt(e.b, d.Milliseconds(), 10)
	e.b = append(e.b, '\n')
}

// field appends one data line. value must not contain CR or LF.
func (e *event) field(key, value string) {
	e.b = append(e.b, "data: "...)
	e.b = append(e.b, key...)
	e.b = append(e.b, value...)
	e.b = append(e.b, '\n')
}

// Write splits p at CR, LF and CRLF so content cannot inject SSE fields.
// The client rejoins data lines with LF. HTML [normalizes newlines]
// the same way, which preserves the patched DOM and script text.
//
// [normalizes newlines]: https://infra.spec.whatwg.org/#normalize-newlines
func (e *event) Write(p []byte) (int, error) {
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
	hasCR := bytes.IndexByte(p, '\r') >= 0
	for len(p) > 0 {
		if !e.open {
			e.b = append(e.b, "data: "...)
			e.b = append(e.b, e.key...)
			e.open = true
		}
		var i int
		if hasCR {
			i = bytes.IndexAny(p, "\r\n")
		} else {
			i = bytes.IndexByte(p, '\n')
		}
		if i < 0 {
			e.b = append(e.b, p...)
			break
		}
		e.b = append(e.b, p[:i]...)
		e.b = append(e.b, '\n')
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

// WriteString copies through a stack buffer to avoid converting s to []byte.
func (e *event) WriteString(s string) (int, error) {
	n := len(s)
	var chunk [512]byte
	for s != "" {
		m := copy(chunk[:], s)
		_, _ = e.Write(chunk[:m])
		s = s[m:]
	}
	return n, nil
}

// end finishes the data field and event.
func (e *event) end() {
	switch {
	case e.open:
		e.b = append(e.b, '\n')
	case e.wrote:
		// Preserve a trailing line break with an empty data line.
		e.field(e.key, "")
	}
	e.b = append(e.b, '\n')
}
