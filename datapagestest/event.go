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

// Datastar SSE event types.
const (
	PatchElements = "datastar-patch-elements"
	PatchSignals  = "datastar-patch-signals"
)

// Event is one Datastar SSE event.
type Event struct {
	// Type is PatchElements or PatchSignals.
	Type string

	// Data maps data-line names to values. Repeated values are joined with "\n".
	// A missing mode morphs elements; a missing selector matches them by id.
	Data map[string]string

	// Retry is the reconnection delay, or 0 when unset. A server sets it on the final
	// event of a stream it ends.
	Retry time.Duration
}

// Root parses the event's elements inside a <template>, preserving table elements
// that html.Parse drops at a document root.
func (e Event) Root() *html.Node {
	return parseHTML("<template>" + e.Data["elements"] + "</template>")
}

// ElementByID returns the event element with id or fails the test.
func (e Event) ElementByID(t testing.TB, id string) *html.Node {
	t.Helper()
	n := ElementByID(e.Root(), id)
	if n == nil {
		t.Fatalf("no element with id %q in %s", id, describe(e))
	}
	return n
}

// Signals decodes a PatchSignals event. Deleted signals have raw value null;
// untouched signals have no key.
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

// Script returns an ExecuteScript-style body-append script and whether one exists.
// Datapages uses this form for SSE redirects, session-end reloads and sse.ExecuteScript.
func (e Event) Script() (string, bool) {
	if e.Type != PatchElements || e.Data["selector"] != "body" ||
		e.Data["mode"] != "append" {
		return "", false
	}
	for n := range e.Root().Descendants() {
		if n.Type != html.ElementNode || n.Data != "script" {
			continue
		}
		var b strings.Builder
		for c := range n.ChildNodes() {
			b.WriteString(c.Data)
		}
		return b.String(), true
	}
	return "", false
}

// Redirect returns the target of an SSE redirect script.
func (e Event) Redirect() (target string, ok bool) {
	if e.Type != PatchElements {
		return "", false
	}
	return scriptRedirect(e.Data["elements"])
}

// redirectPattern accepts each window.location assignment Datapages emits.
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

// parser reads SSE lines using Datastar's "data: <field> <value>" format.
type parser struct {
	cur  Event
	open bool // cur has a field
}

// line returns the event completed by a blank line.
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

// parseEvents parses complete events from an SSE body and omits an unfinished event.
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

// Match selects events for [Stream.WaitFor], [Stream.Absent] and [Response.Event].
type Match struct {
	what string
	f    func(Event) bool
}

// String describes m for failure messages.
func (m Match) String() string { return m.what }

// Matches reports whether m matches e.
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

// Reload matches a page-reload script, including the final event after a session ends.
func Reload() Match {
	return Func("the script that reloads the page", func(e Event) bool {
		script, ok := e.Script()
		return ok && strings.Contains(script, "location.reload()")
	})
}
