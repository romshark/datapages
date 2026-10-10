package datapagestest

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// Connect opens the tab's stream the way the page's data-init does, with signals as
// the connection's signals. Datastar sends every public signal of the page, such as
// those [Response.Signals] reads, with the visitor's changes. A nil signals sends "{}".
// The stream closes when the test ends.
//
// A signed-out visitor on a page that also serves an anonymous stream
// is redirected to it, which Connect follows.
func (tb *Tab) Connect(t testing.TB, signals any) *Stream {
	t.Helper()
	enc := encodeSignals(t, signals)
	desc := "GET " + tb.path + "_$/"
	target := tb.v.srv.srv.URL + tb.path + "_$/?datastar=" + url.QueryEscape(enc)
	ctx, cancel := context.WithCancel(t.Context())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		cancel()
		t.Fatalf("%s: building the request: %v", desc, err)
	}
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Accept-Encoding", "identity")
	if tb.instance != "" {
		req.Header.Set("Datapages-Instance", tb.instance)
	}
	resp, err := tb.v.stream.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("%s: %v", desc, err)
	}
	if p := resp.Request.URL.EscapedPath(); p != tb.path+"_$/" {
		desc += " -> " + p
	}
	desc += signalsDesc(enc)
	// A path that misses the stream route can reach a page, which answers 200 with HTML.
	if ct := resp.Header.Get("Content-Type"); resp.StatusCode != http.StatusOK ||
		!isEventStream(ct) {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
		_ = resp.Body.Close()
		cancel()
		t.Fatalf("%s: %s with Content-Type %q, want 200 text/event-stream:\n%s",
			desc, resp.Status, ct, b)
	}
	s := &Stream{
		desc:    desc,
		path:    resp.Request.URL.Path,
		timeout: tb.v.srv.conf.timeout,
		cancel:  cancel,
		done:    make(chan struct{}),
	}
	go s.read(resp.Body)
	t.Cleanup(s.Close)
	return s
}

// signalsDesc names the signals of a stream request in failure messages, shortened.
// A page's streams differ in their path or their signals.
func signalsDesc(enc string) string {
	const limit = 100
	switch {
	case enc == "{}":
		return ""
	case len(enc) > limit:
		enc = enc[:limit] + "..."
	}
	return " with signals " + enc
}

func isEventStream(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	return err == nil && mt == "text/event-stream"
}

// Stream is a tab's SSE connection. A cursor marks the events that [Stream.WaitFor]
// and [Stream.Absent] have consumed.
type Stream struct {
	desc    string
	path    string
	timeout time.Duration
	cancel  context.CancelFunc
	done    chan struct{} // closed when read returns

	mu      sync.Mutex
	events  []Event
	cursor  int
	ended   bool
	closing bool  // Close ended the stream
	err     error // what ended the read
}

func (s *Stream) read(body io.ReadCloser) {
	defer close(s.done)
	defer func() { _ = body.Close() }()
	var p parser
	r := bufio.NewReader(body)
	for {
		// An event that patches a whole page is one line of data and can be large,
		// which a bufio.Scanner with its default buffer would stop at.
		line, err := r.ReadString('\n')
		if err != nil {
			s.mu.Lock()
			s.ended, s.err = true, err
			s.mu.Unlock()
			return
		}
		if ev, ok := p.line(line[:len(line)-1]); ok {
			s.mu.Lock()
			s.events = append(s.events, ev)
			s.mu.Unlock()
		}
	}
}

// Events returns the complete events the stream has carried so far,
// including those before the cursor.
func (s *Stream) Events() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.events)
}

// Path returns the path of the stream route that answered: the page path followed by
// "_$/", or by "_$/anon/" when the server redirected a signed-out visitor there.
func (s *Stream) Path() string { return s.path }

// Ended reports whether the stream has ended: the server ended it, the connection
// failed or the test closed it.
func (s *Stream) Ended() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ended
}

// WaitFor waits for the first event after the cursor that m matches, moves the cursor
// past it and returns it. It fails the test when the stream ends or the timeout passes
// first.
//
// Match what only the expected effect carries. The events of StreamOpen and of
// earlier actions are on the stream too, and may render the same element.
//
// The optional msgAndArgs, a message or a format and its arguments, open the failure
// message, such as to tell two streams of one page apart.
func (s *Stream) WaitFor(t testing.TB, m Match, msgAndArgs ...any) Event {
	t.Helper()
	_, ev := s.waitFor(t, m, msgAndArgs)
	return ev
}

// waitFor is [Stream.WaitFor] that also returns the index of the event.
func (s *Stream) waitFor(t testing.TB, m Match, msgAndArgs []any) (int, Event) {
	t.Helper()
	deadline := time.Now().Add(s.timeout)
	s.mu.Lock()
	from := s.cursor
	s.mu.Unlock()
	next := from
	for {
		s.mu.Lock()
		// The reading goroutine only appends to events. The elements this copy of the
		// slice shares with the field never change.
		evs, ended, closing, err := s.events, s.ended, s.closing, s.err
		s.mu.Unlock()
		for ; next < len(evs); next++ {
			if m.f(evs[next]) {
				s.mu.Lock()
				s.cursor = next + 1
				s.mu.Unlock()
				return next, evs[next]
			}
		}
		if ended {
			fatalf(t, msgAndArgs, "%s: %s before %s arrived%s",
				s.desc, endReason(closing, err), m, summary(evs[from:]))
		}
		if time.Now().After(deadline) {
			fatalf(t, msgAndArgs, "%s: waited %v for %s%s",
				s.desc, s.timeout, m, summary(evs[from:]))
		}
		time.Sleep(pollInterval)
	}
}

// Absent waits for the first event after the cursor that anchor matches and fails the
// test when an event before it matches prohibited. The cursor moves past the anchor.
//
// Publish the anchor after the events that must not arrive, on a subject this stream
// receives. Its arrival proves their absence: a [messaging.Subscriber] delivers
// messages published one after the other in that order, whatever their subjects.
//
// The optional msgAndArgs open the failure message, as for [Stream.WaitFor].
func (s *Stream) Absent(t testing.TB, prohibited, anchor Match, msgAndArgs ...any) {
	t.Helper()
	s.mu.Lock()
	from := s.cursor
	s.mu.Unlock()
	i, _ := s.waitFor(t, anchor, msgAndArgs)
	for _, ev := range s.Events()[from:i] {
		if prohibited.f(ev) {
			fatalf(t, msgAndArgs, "%s: %s arrived before %s:\n%s",
				s.desc, prohibited, anchor, describe(ev))
		}
	}
}

// WaitEnd waits for the server to end the stream, by ending the response or by
// dropping the connection, and fails the test when the timeout passes first or the
// test closed the stream. A Datapages server ends a stream when its session closes or
// expires, when it shuts down and when StreamOpen fails. The events the stream carried
// stay readable. The optional msgAndArgs open the failure message, as for
// [Stream.WaitFor].
func (s *Stream) WaitEnd(t testing.TB, msgAndArgs ...any) {
	t.Helper()
	select {
	case <-s.done:
	case <-time.After(s.timeout):
		s.mu.Lock()
		evs := s.events[s.cursor:]
		s.mu.Unlock()
		fatalf(t, msgAndArgs, "%s: waited %v for the server to end the stream%s",
			s.desc, s.timeout, summary(evs))
	}
	s.mu.Lock()
	closing := s.closing
	s.mu.Unlock()
	if closing {
		fatalf(t, msgAndArgs, "%s: the test closed the stream before the server ended it",
			s.desc)
	}
}

// fatalf fails the test with format and args, opened by the message msgAndArgs give.
func fatalf(t testing.TB, msgAndArgs []any, format string, args ...any) {
	t.Helper()
	if msg := message(msgAndArgs); msg != "" {
		format = "%s: " + format
		args = append([]any{msg}, args...)
	}
	t.Fatalf(format, args...)
}

// message renders the optional message arguments of a wait the way testify does: a
// format string and its arguments, or one value. It is "" for none.
func message(msgAndArgs []any) string {
	switch len(msgAndArgs) {
	case 0:
		return ""
	case 1:
		return fmt.Sprint(msgAndArgs[0])
	}
	if format, ok := msgAndArgs[0].(string); ok {
		return fmt.Sprintf(format, msgAndArgs[1:]...)
	}
	return fmt.Sprint(msgAndArgs...)
}

// Close closes the stream the way a closing tab does and waits for its reader to
// end. The server runs the page's StreamClose afterwards: wait for its effect on
// another stream or with [Server.WaitFor].
func (s *Stream) Close() {
	s.mu.Lock()
	s.closing = true
	s.mu.Unlock()
	s.cancel()
	<-s.done
}

func endReason(closing bool, err error) string {
	switch {
	case closing:
		return "the test closed the stream"
	case errors.Is(err, io.EOF):
		return "the server ended the stream"
	}
	return fmt.Sprintf("the stream failed with %v", err)
}

// summary lists the events a wait saw, for its failure message.
func summary(evs []Event) string {
	if len(evs) == 0 {
		return "; no events arrived"
	}
	var b strings.Builder
	if len(evs) == 1 {
		b.WriteString("; 1 event arrived:")
	} else {
		fmt.Fprintf(&b, "; %d events arrived:", len(evs))
	}
	for _, ev := range evs {
		b.WriteString("\n")
		b.WriteString(describe(ev))
	}
	return b.String()
}

// describe renders an event in one line, shortened.
func describe(ev Event) string {
	const limit = 200
	var b strings.Builder
	b.WriteString(ev.Type)
	for _, k := range slices.Sorted(maps.Keys(ev.Data)) {
		v := ev.Data[k]
		if len(v) > limit {
			v = v[:limit] + "..."
		}
		fmt.Fprintf(&b, " %s=%q", k, v)
	}
	if ev.Retry > 0 {
		fmt.Fprintf(&b, " retry=%v", ev.Retry)
	}
	return b.String()
}
