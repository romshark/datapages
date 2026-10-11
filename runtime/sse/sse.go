// Package sse writes Datastar SSE events and reads request signals.
// [Stream] implements datapages.SSE.
//
// Application code must not import this package.
package sse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/romshark/datapages"
)

const (
	eventPatchElements = "datastar-patch-elements"
	eventPatchSignals  = "datastar-patch-signals"
)

// scriptOpen starts a script element that removes itself after execution.
const scriptOpen = `<script data-effect="el.remove()">`

var errClosed = errors.New("sse: stream closed")

// Stream writes Datastar events to a response.
// It implements [datapages.SSE], and its methods are safe for concurrent use.
//
// Events are built before the lock. The lock serializes writes and flushes.
type Stream struct {
	ctx    context.Context
	w      http.ResponseWriter
	rc     *http.ResponseController
	coding *coding // Nil without compression.

	mu     sync.Mutex
	enc    *pooledEncoder // Taken from coding by the first event.
	err    error          // The first failed write, which every later event returns.
	closed bool
}

// Open starts and flushes an event stream. It selects the first accepted
// compression from zstd, brotli and gzip. The caller must call [Stream.Close].
//
// Open panics if w does not support flushing because events would wait for the
// handler to return. [Stream.Err] and later events report other flush failures.
func Open(w http.ResponseWriter, r *http.Request) *Stream {
	s := &Stream{ctx: r.Context(), w: w, rc: http.NewResponseController(w)}
	h := w.Header()
	h.Set("Cache-Control", "no-cache")
	h.Set("Content-Type", "text/event-stream")
	if r.ProtoMajor == 1 {
		h.Set("Connection", "keep-alive")
	}
	h.Add("Vary", "Accept-Encoding")
	if c := negotiate(r.Header["Accept-Encoding"]); c != nil {
		h.Set("Content-Encoding", c.name)
		s.coding = c
	}
	if err := s.rc.Flush(); err != nil {
		if errors.Is(err, http.ErrNotSupported) {
			panic(fmt.Sprintf("sse: %T cannot flush", w))
		}
		s.err = err
	}
	return s
}

// Err returns the error of the first write or flush that failed, if any.
func (s *Stream) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Close finishes compression and releases the encoder. [Stream.Err] reports a
// final write failure. Close is idempotent; later events return an error.
func (s *Stream) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	switch {
	case s.coding == nil:
	case s.enc != nil:
		if s.err == nil {
			s.err = s.enc.Close()
		}
		s.coding.put(s.enc)
		s.enc = nil
	case s.err == nil:
		// Send a valid empty stream for the announced coding.
		_, s.err = s.w.Write(s.coding.empty)
	}
}

func (s *Stream) Context() context.Context { return s.ctx }

func (s *Stream) PatchElement(c datapages.Component) error {
	return s.patchElements(c, "", "")
}

func (s *Stream) PatchElementAt(
	c datapages.Component, selectorCSS string, mode datapages.PatchMode,
) error {
	if err := checkSelector(selectorCSS); err != nil {
		return err
	}
	switch mode {
	case datapages.PatchModeInner, datapages.PatchModeReplace,
		datapages.PatchModePrepend, datapages.PatchModeAppend,
		datapages.PatchModeBefore, datapages.PatchModeAfter:
	default:
		// Outer is the wire default. Invalid modes also use the default.
		mode = ""
	}
	return s.patchElements(c, selectorCSS, mode)
}

func (s *Stream) patchElements(
	c datapages.Component, selectorCSS string, mode datapages.PatchMode,
) error {
	e := newEvent(eventPatchElements)
	if selectorCSS != "" {
		e.field("selector ", selectorCSS)
	}
	if mode != "" {
		e.field("mode ", string(mode))
	}
	e.key = "elements "
	if err := c.Render(s.ctx, e); err != nil {
		e.free()
		return fmt.Errorf("rendering elements: %w", err)
	}
	return s.send(e)
}

// checkSelector rejects a selector that could inject an SSE field.
func checkSelector(selectorCSS string) error {
	if strings.ContainsAny(selectorCSS, "\r\n") {
		return fmt.Errorf("%w: %q", datapages.ErrSelectorLineBreak, selectorCSS)
	}
	return nil
}

func (s *Stream) RemoveElement(selectorCSS string) error {
	if err := checkSelector(selectorCSS); err != nil {
		return err
	}
	e := newEvent(eventPatchElements)
	e.field("selector ", selectorCSS)
	e.field("mode ", "remove")
	return s.send(e)
}

func (s *Stream) ExecuteScript(script string) error {
	return s.ExecuteScriptRetry(script, 0)
}

// ExecuteScriptRetry works like [Stream.ExecuteScript] and sets the SSE retry
// field when retry is positive.
func (s *Stream) ExecuteScriptRetry(script string, retry time.Duration) error {
	e := newScriptEvent(retry)
	_, _ = e.WriteString(script)
	_, _ = e.WriteString("</script>")
	return s.send(e)
}

// newScriptEvent returns an element patch with an open self-removing script.
func newScriptEvent(retry time.Duration) *event {
	e := newEvent(eventPatchElements)
	e.retry(retry)
	e.field("selector ", "body")
	e.field("mode ", "append")
	e.key = "elements "
	_, _ = e.WriteString(scriptOpen)
	return e
}

func (s *Stream) PatchSignals(v any) error {
	j, err := marshalSignals(v)
	if err != nil {
		return err
	}
	return s.patchSignals(j, false, 0)
}

func (s *Stream) PatchSignalsIfMissing(v any) error {
	j, err := marshalSignals(v)
	if err != nil {
		return err
	}
	return s.patchSignals(j, true, 0)
}

var emptySignals = []byte("{}")

// DelayReconnect sends a no-op signal patch with retry as the SSE retry field.
func (s *Stream) DelayReconnect(retry time.Duration) error {
	return s.patchSignals(emptySignals, false, retry)
}

// patchSignals sends valid JSON. Event framing normalizes its CR and LF to LF.
func (s *Stream) patchSignals(j []byte, onlyIfMissing bool, retry time.Duration) error {
	e := newEvent(eventPatchSignals)
	e.retry(retry)
	if onlyIfMissing {
		e.field("onlyIfMissing ", "true")
	}
	e.key = "signals "
	_, _ = e.Write(j)
	return s.send(e)
}

// marshalSignals encodes v as JSON. A json.RawMessage passes through.
func marshalSignals(v any) ([]byte, error) {
	if raw, ok := v.(json.RawMessage); ok {
		if !json.Valid(raw) {
			return nil, errors.New("signals are not valid JSON")
		}
		return raw, nil
	}
	j, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshaling signals JSON: %w", err)
	}
	return j, nil
}

func (s *Stream) Redirect(target string) error {
	// The redirect travels as a <script> element. A "</script>" in the target
	// would end that element and leave the rest of it in the DOM as markup,
	// which is why the target goes in JSON-encoded: encoding/json writes
	// "<" as \u003c, and JavaScript reads it back as "<".
	enc, err := json.Marshal(target)
	if err != nil {
		return fmt.Errorf("encoding redirect target: %w", err)
	}
	e := newScriptEvent(0)
	_, _ = e.WriteString("setTimeout(() => window.location.href = ")
	_, _ = e.Write(enc)
	_, _ = e.WriteString(")</script>")
	return s.send(e)
}

// Prefetch appends rules in a retained script element.
// Removing it [unregisters the rules].
//
// [unregister the rules]: https://html.spec.whatwg.org/multipage/webappapis.html#unregister-speculation-rules
func (s *Stream) Prefetch(urls ...string) error {
	if len(urls) == 0 {
		return nil
	}
	e := newEvent(eventPatchElements)
	e.field("selector ", "body")
	e.field("mode ", "append")
	e.key = "elements "
	_, _ = e.WriteString(`<script type="speculationrules">`)
	_, _ = e.WriteString(speculationRules(urls))
	_, _ = e.WriteString("</script>")
	return s.send(e)
}

// send finishes, writes, flushes and frees e.
func (s *Stream) send(e *event) error {
	defer e.free()
	e.end()
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case s.err != nil:
		return s.err
	case s.closed:
		return errClosed
	}
	if err := s.ctx.Err(); err != nil {
		return err
	}
	if err := s.write(e.b); err != nil {
		s.err = err
		return err
	}
	return nil
}

func (s *Stream) write(b []byte) error {
	if s.coding == nil {
		if _, err := s.w.Write(b); err != nil {
			return err
		}
		return s.rc.Flush()
	}
	if s.enc == nil {
		s.enc = s.coding.take(s.w)
	}
	if _, err := s.enc.Write(b); err != nil {
		return err
	}
	if err := s.enc.Flush(); err != nil {
		return err
	}
	return s.rc.Flush()
}

// speculationRules returns a JSON body of [speculation rules] that prefetches URLs.
// It escapes script content without changing percent-encoded URLs.
//
// [speculation rules]: https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Elements/script/type/speculationrules
func speculationRules(urls []string) string {
	var b strings.Builder
	b.Grow(speculationRulesLen(urls))
	b.WriteString(rulesHead)
	for i, u := range urls {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('"')
		writeJSONStringBody(&b, u)
		b.WriteByte('"')
	}
	b.WriteString(rulesTail)
	return b.String()
}

const rulesHead, rulesTail = `{"prefetch":[{"source":"list","urls":[`, `]}]}`

// speculationRulesLen returns the exact rules length when no URL needs escaping.
func speculationRulesLen(urls []string) int {
	n := len(rulesHead) + len(rulesTail) + len(urls) - 1 // the commas
	for _, u := range urls {
		n += len(`""`) + len(u)
	}
	return n
}

const hexDigits = "0123456789abcdef"

// writeJSONStringBody writes s inside a JSON string in a one-line script. It
// escapes JSON syntax, controls and '<' to prevent line breaks, </script> and <!--.
func writeJSONStringBody(b *strings.Builder, s string) {
	last := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x20 && c != '"' && c != '\\' && c != '<' {
			continue
		}
		b.WriteString(s[last:i])
		if c == '"' || c == '\\' {
			b.WriteByte('\\')
			b.WriteByte(c)
		} else {
			b.WriteString(`\u00`)
			b.WriteByte(hexDigits[c>>4])
			b.WriteByte(hexDigits[c&0xf])
		}
		last = i + 1
	}
	b.WriteString(s[last:])
}
