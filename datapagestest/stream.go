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

// Connect opens the tab's SSE stream with the tab's signals, leaving out local
// signals as Datastar does. Set others first with [Tab.SetSignal]. The tab applies
// the stream's events to its page. Connect follows an anonymous-stream redirect,
// times out while waiting for response headers and closes the stream during cleanup.
func (tb *Tab) Connect(t testing.TB) *Stream {
	t.Helper()
	enc := tb.requestSignals(t, signalFilter{})
	desc := "GET " + tb.path + "_$/"
	target := tb.v.srv.srv.URL + tb.path + "_$/?datastar=" + url.QueryEscape(enc)
	timeout := tb.v.srv.conf.timeout
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
	// A context deadline would also close the live stream; limit only the handshake.
	timer := time.AfterFunc(timeout, cancel)
	resp, err := tb.v.stream.Do(req)
	if !timer.Stop() {
		if err == nil {
			_ = resp.Body.Close()
		}
		t.Fatalf("%s: no response header within %v", desc, timeout)
	}
	if err != nil {
		cancel()
		t.Fatalf("%s: %v", desc, err)
	}
	if p := resp.Request.URL.EscapedPath(); p != tb.path+"_$/" {
		desc += " -> " + p
	}
	desc += signalsDesc(enc)
	// A missing stream route can return a 200 HTML page.
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
		timeout: timeout,
		cancel:  cancel,
		done:    make(chan struct{}),
		changed: make(chan struct{}),
	}
	go s.read(resp.Body)
	t.Cleanup(s.Close)
	tb.streams = append(tb.streams, &tabStream{s: s})
	return s
}

// signalsDesc formats signals for a failure message, capped at 100 bytes.
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

// Stream is a tab's SSE connection. Its cursor tracks consumed events.
type Stream struct {
	desc    string
	path    string
	timeout time.Duration
	cancel  context.CancelFunc
	done    chan struct{} // closed when read returns

	mu      sync.Mutex
	changed chan struct{} // closed and replaced when an event arrives or the read ends
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
		// A full-page patch can exceed a bufio.Scanner buffer.
		line, err := r.ReadString('\n')
		if err != nil {
			s.mu.Lock()
			s.ended, s.err = true, err
			s.notifyLocked()
			s.mu.Unlock()
			return
		}
		if ev, ok := p.line(line[:len(line)-1]); ok {
			s.mu.Lock()
			s.events = append(s.events, ev)
			s.notifyLocked()
			s.mu.Unlock()
		}
	}
}

// notifyLocked wakes blocked waits, allowing a synctest clock to advance.
func (s *Stream) notifyLocked() {
	close(s.changed)
	s.changed = make(chan struct{})
}

// Events returns all received events, including consumed events.
func (s *Stream) Events() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.events)
}

// Path returns the final stream route, including an anonymous-stream redirect.
func (s *Stream) Path() string { return s.path }

// Ended reports whether the server, connection or test ended the stream.
func (s *Stream) Ended() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ended
}

// WaitFor returns the first event matching m at or after the cursor,
// then advances past it. It fails if the stream ends or the wait times out.
//
// Match narrowly: StreamOpen and earlier actions may render the same element.
//
// msgAndArgs optionally prefixes the failure message.
func (s *Stream) WaitFor(t testing.TB, m Match, msgAndArgs ...any) Event {
	t.Helper()
	_, ev := s.waitFor(t, m, msgAndArgs)
	return ev
}

// waitFor also returns the matched event's index.
func (s *Stream) waitFor(t testing.TB, m Match, msgAndArgs []any) (int, Event) {
	t.Helper()
	timeout := time.NewTimer(s.timeout)
	defer timeout.Stop()
	s.mu.Lock()
	from := s.cursor
	s.mu.Unlock()
	next := from
	for {
		s.mu.Lock()
		// The reader only appends. Existing shared elements remain immutable.
		evs, ended, closing, err, changed :=
			s.events, s.ended, s.closing, s.err, s.changed
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
		select {
		case <-changed:
		case <-timeout.C:
			fatalf(t, msgAndArgs, "%s: waited %v for %s%s",
				s.desc, s.timeout, m, summary(evs[from:]))
		}
	}
}

// Absent advances through anchor and fails if prohibited arrives first.
//
// Publish anchor after the prohibited window on any subject the stream receives.
// [messaging.Subscriber] ordering makes its arrival prove the earlier event absent.
//
// msgAndArgs behaves as in [Stream.WaitFor].
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

// WaitEnd waits for the server or connection to end the stream. It fails on timeout
// or test closure; received events remain readable.
// msgAndArgs behaves as in [Stream.WaitFor].
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
		fatalf(t, msgAndArgs,
			"%s: the test closed the stream before the server ended it",
			s.desc)
	}
}

// fatalf prefixes a failure with the optional message.
func fatalf(t testing.TB, msgAndArgs []any, format string, args ...any) {
	t.Helper()
	if msg := message(msgAndArgs); msg != "" {
		format = "%s: " + format
		args = append([]any{msg}, args...)
	}
	t.Fatalf(format, args...)
}

// message formats optional wait arguments like testify.
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

// Close cancels the stream and waits for its reader. The server then runs StreamClose;
// wait for its effect on another stream or with [Server.WaitFor].
func (s *Stream) Close() {
	s.mu.Lock()
	// Preserve a prior server-ended state for WaitEnd and failure messages.
	if !s.ended {
		s.closing = true
	}
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

// summary formats events seen by a failed wait.
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

// describe renders a shortened, one-line event.
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
