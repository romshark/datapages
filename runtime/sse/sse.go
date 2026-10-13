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
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/romshark/datapages"
)

// The event types of the Datastar SDK specification, at the client version that
// [github.com/romshark/datapages/runtime/httpserve.DefaultDatastarJSSrc] loads.
// See [datastar-patch-elements] and [datastar-patch-signals].
//
// [datastar-patch-elements]: https://github.com/starfederation/datastar/blob/v1.0.4/sdk/ADR.md#serversenteventgeneratorpatchelements
// [datastar-patch-signals]: https://github.com/starfederation/datastar/blob/v1.0.4/sdk/ADR.md#serversenteventgeneratorpatchsignals
const (
	eventPatchElements = "datastar-patch-elements"
	eventPatchSignals  = "datastar-patch-signals"
)

// scriptOpen starts a script element that removes itself after execution.
// [ExecuteScript] specifies it, along with the selector body and mode append.
//
// [ExecuteScript]: https://github.com/starfederation/datastar/blob/v1.0.4/sdk/ADR.md#serversenteventgeneratorexecutescript
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

	lock   sync.Mutex
	enc    *pooledEncoder // Taken from coding by the first event.
	err    error          // The first failed write, which every later event returns.
	closed bool
}

// Open starts and flushes an event stream. It selects the first coding of c
// that r accepts by name, else gzip when r accepts *, and none when c is nil.
// The caller must call [Stream.Close].
// The headers and the first flush follow the [SDK specification].
//
// Open panics if w doesn't support flushing because events would wait for the
// handler to return. [Stream.Err] and later events report other flush failures.
//
// [SDK specification]: https://github.com/starfederation/datastar/blob/v1.0.4/sdk/ADR.md#construction--initialization
func Open(w http.ResponseWriter, r *http.Request, c *Compression) *Stream {
	s := &Stream{ctx: r.Context(), w: w, rc: http.NewResponseController(w)}
	h := w.Header()
	h.Set("Cache-Control", "no-cache")
	h.Set("Content-Type", "text/event-stream")
	if r.ProtoMajor == 1 {
		h.Set("Connection", "keep-alive")
	}
	if c != nil {
		h.Add("Vary", "Accept-Encoding")
	}
	if cd := c.negotiate(r.Header["Accept-Encoding"]); cd != nil {
		h.Set("Content-Encoding", cd.name)
		s.coding = cd
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
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.err
}

// Close finishes compression and releases the encoder. [Stream.Err] reports a
// final write failure. Close is idempotent; later events return an error.
func (s *Stream) Close() {
	s.lock.Lock()
	defer s.lock.Unlock()
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
	e.prefix = "data: elements "
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
	e.prefix = "data: elements "
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
	e.prefix = "data: signals "
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
	// would end that element and leave the rest of it in the DOM as markup.
	// [writeJSONStringBody] writes "<" as \u003c, which JavaScript reads back as "<".
	e := newScriptEvent(0)
	_, _ = e.WriteString(`setTimeout(() => window.location.href = "`)
	writeJSONStringBody(e, target)
	_, _ = e.WriteString(`")</script>`)
	return s.send(e)
}

// Prefetch appends [speculation rules] in a retained script element.
// Removing it [unregisters the rules]. The URLs go in as JSON strings,
// escaped for the script without changing percent-encoding.
//
// [speculation rules]: https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Elements/script/type/speculationrules
// [unregisters the rules]: https://html.spec.whatwg.org/multipage/webappapis.html#unregister-speculation-rules
func (s *Stream) Prefetch(urls ...string) error {
	if len(urls) == 0 {
		return nil
	}
	e := newEvent(eventPatchElements)
	e.field("selector ", "body")
	e.field("mode ", "append")
	e.prefix = "data: elements "
	_, _ = e.WriteString(
		`<script type="speculationrules">{"prefetch":[{"source":"list","urls":["`,
	)
	for i, u := range urls {
		if i > 0 {
			_, _ = e.WriteString(`","`)
		}
		writeJSONStringBody(e, u)
	}
	_, _ = e.WriteString(`"]}]}</script>`)
	return s.send(e)
}

// send finishes, writes, flushes and frees e.
func (s *Stream) send(e *event) error {
	defer e.free()
	e.end()
	s.lock.Lock()
	defer s.lock.Unlock()
	switch {
	case s.err != nil:
		return s.err
	case s.closed:
		return errClosed
	}
	if err := s.ctx.Err(); err != nil {
		return err
	}
	if err := s.write(e); err != nil {
		s.err = err
		return err
	}
	return nil
}

// write writes the frames of e and flushes them once, at the end of the event.
func (s *Stream) write(e *event) error {
	if s.coding == nil {
		if err := e.writeFrames(s.w); err != nil {
			return err
		}
		return s.rc.Flush()
	}
	if s.enc == nil {
		s.enc = s.coding.take(s.w)
	}
	if err := e.writeFrames(s.enc); err != nil {
		return err
	}
	if err := s.enc.Flush(); err != nil {
		return err
	}
	return s.rc.Flush()
}

const hexDigits = "0123456789abcdef"

// controlEscapes holds the JSON escape of each control byte.
var controlEscapes = func() (t [0x20]string) {
	for c := range t {
		t[c] = `\u00` + hexDigits[c>>4:c>>4+1] + hexDigits[c&0xf:c&0xf+1]
	}
	return t
}()

// writeJSONStringBody writes s inside a JSON string, which is also a JavaScript string,
// in a single-line script. It escapes JSON syntax, controls and '<' to prevent
// line breaks, </script> and <!--. Every escape is a constant,
// which lets it write into an event without a buffer.
func writeJSONStringBody(w io.StringWriter, s string) {
	last := 0
	for i := 0; i < len(s); i++ {
		var esc string
		switch c := s[i]; {
		case c < 0x20:
			esc = controlEscapes[c]
		case c == '"':
			esc = `\"`
		case c == '\\':
			esc = `\\`
		case c == '<':
			esc = `\u003c`
		default:
			continue
		}
		_, _ = w.WriteString(s[last:i])
		_, _ = w.WriteString(esc)
		last = i + 1
	}
	_, _ = w.WriteString(s[last:])
}
