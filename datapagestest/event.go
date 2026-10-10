package datapagestest

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// Event types a Datapages server sends.
const (
	PatchElements = "datastar-patch-elements"
	PatchSignals  = "datastar-patch-signals"
)

// Event is one Datastar SSE event.
type Event struct {
	// Type is PatchElements or PatchSignals.
	Type string

	// Data holds the event's data lines by field name, such as "elements",
	// "selector", "mode" and "signals". Repeated lines are joined with "\n".
	// Without a mode the elements are morphed into the page, and without a
	// selector each one is matched by its id.
	Data map[string]string

	// Retry is the reconnection delay the event's retry field sets, 0 when it has none.
	// A Datapages server sets it on the last event of a stream it ends, such as the
	// reload at the end of a session.
	Retry time.Duration
}

// Root parses the event's elements the way Datastar does, inside a <template>.
// html.Parse alone drops a <tr>, <td> or <tbody> at the root of a patch.
func (e Event) Root() *html.Node {
	return parseHTML("<template>" + e.Data["elements"] + "</template>")
}

// ElementByID returns the element with id among the event's elements and fails the
// test when there is none.
func (e Event) ElementByID(t testing.TB, id string) *html.Node {
	t.Helper()
	n := ElementByID(e.Root(), id)
	if n == nil {
		t.Fatalf("no element with id %q in %s", id, describe(e))
	}
	return n
}

// Signals decodes the top level of a PatchSignals event. A signal the patch deletes
// has the raw value null. A signal the patch leaves alone has no key, which a struct
// decoded from the event cannot tell apart from null.
func (e Event) Signals(t testing.TB) map[string]json.RawMessage {
	t.Helper()
	if e.Type != PatchSignals {
		t.Fatalf("%s is not a signal patch", describe(e))
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(e.Data["signals"]), &m); err != nil {
		t.Fatalf("decoding the signals of %s: %v", describe(e), err)
	}
	return m
}

// Redirect returns the target of the redirect script the event appends, the form a
// redirect takes after SSE started and the form of every sse.Redirect.
func (e Event) Redirect() (target string, ok bool) {
	if e.Type != PatchElements {
		return "", false
	}
	return scriptRedirect(e.Data["elements"])
}

// redirectPattern finds the target in each script Datapages redirects with:
// window.location = "<Go string>" before SSE starts, window.location=<JSON string>
// from a page-cache action, and window.location.href = <JSON string> in an event.
var redirectPattern = regexp.MustCompile(
	`window\.location(?:\.href)?\s*=\s*("(?:[^"\\]|\\.)*")`,
)

func scriptRedirect(script string) (string, bool) {
	m := redirectPattern.FindStringSubmatch(script)
	if m == nil {
		return "", false
	}
	if s, err := strconv.Unquote(m[1]); err == nil {
		return s, true
	}
	var s string
	if err := json.Unmarshal([]byte(m[1]), &s); err == nil {
		return s, true
	}
	return "", false
}

// parser turns the lines of an SSE body into events. The Datastar SDK ends a line with
// "\n" and writes each data line as "data: <field> <value>".
type parser struct {
	cur  Event
	open bool // cur has a field
}

// line takes one line without its "\n" and returns the event a blank line finishes.
func (p *parser) line(l string) (Event, bool) {
	l = strings.TrimSuffix(l, "\r")
	if l == "" {
		if !p.open {
			return Event{}, false
		}
		ev := p.cur
		p.cur, p.open = Event{}, false
		return ev, true
	}
	if strings.HasPrefix(l, ":") {
		return Event{}, false // A comment.
	}
	field, value, _ := strings.Cut(l, ":")
	value = strings.TrimPrefix(value, " ")
	switch field {
	case "event":
		p.cur.Type, p.open = value, true
	case "data":
		name, v, _ := strings.Cut(value, " ")
		if p.cur.Data == nil {
			p.cur.Data = map[string]string{}
		}
		if prev, ok := p.cur.Data[name]; ok {
			v = prev + "\n" + v
		}
		p.cur.Data[name] = v
		p.open = true
	case "retry":
		if ms, err := strconv.Atoi(value); err == nil {
			p.cur.Retry = time.Duration(ms) * time.Millisecond
		}
	}
	return Event{}, false
}

// parseEvents parses a whole SSE body.
// An event whose closing blank line is missing is left out.
func parseEvents(body string) []Event {
	var p parser
	var evs []Event
	for {
		i := strings.IndexByte(body, '\n')
		if i < 0 {
			return evs
		}
		if ev, ok := p.line(body[:i]); ok {
			evs = append(evs, ev)
		}
		body = body[i+1:]
	}
}

// Match selects the events [Stream.WaitFor] waits for and [Stream.Absent] rules out.
type Match struct {
	what string
	f    func(Event) bool
}

// String names the events m matches, as failure messages do.
func (m Match) String() string { return m.what }

// Matches reports whether m matches e, such as one of the events of an action's
// response that [Response.Events] parses.
func (m Match) Matches(e Event) bool { return m.f(e) }

// Func matches the events f reports true for. what names them in failure messages.
func Func(what string, f func(Event) bool) Match { return Match{what: what, f: f} }

// Element matches a PatchElements event whose elements contain the element with id.
func Element(id string) Match {
	return Func(fmt.Sprintf("an element with id %q", id), func(e Event) bool {
		return e.Type == PatchElements && ElementByID(e.Root(), id) != nil
	})
}

// TextContains matches a PatchElements event whose element with id contains text,
// compared as [Text] renders it.
func TextContains(id, text string) Match {
	what := fmt.Sprintf("an element with id %q containing %q", id, text)
	return Func(what, func(e Event) bool {
		if e.Type != PatchElements {
			return false
		}
		n := ElementByID(e.Root(), id)
		return n != nil && strings.Contains(Text(n), text)
	})
}

// Signal matches a PatchSignals event that patches the top-level signal name,
// including a patch that deletes it with null.
func Signal(name string) Match {
	return Func(fmt.Sprintf("a patch of signal %q", name), func(e Event) bool {
		if e.Type != PatchSignals {
			return false
		}
		var m map[string]json.RawMessage
		if json.Unmarshal([]byte(e.Data["signals"]), &m) != nil {
			return false
		}
		_, ok := m[name]
		return ok
	})
}
