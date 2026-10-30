package datapagestest

import (
	"encoding/json"
	"fmt"
	"mime"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// tabStream is a stream of a tab with the number of its events the tab applied.
type tabStream struct {
	s       *Stream
	applied int
}

// sync returns the tab's page with every event its streams received applied.
func (tb *Tab) sync() *page {
	if tb.live == nil {
		tb.live = newPage(tb.resp.Body)
	}
	for _, ts := range tb.streams {
		ts.s.mu.Lock()
		// The reader only appends, so the slice up to its length stays unchanged.
		evs := ts.s.events
		ts.s.mu.Unlock()
		for ; ts.applied < len(evs); ts.applied++ {
			tb.applyEvent(evs[ts.applied],
				fmt.Sprintf("event %d of %s", ts.applied+1, ts.s.desc))
		}
	}
	return tb.live
}

// current returns the synced page and fails if an event could not be applied.
func (tb *Tab) current(t testing.TB) *page {
	t.Helper()
	p := tb.sync()
	if tb.err != nil {
		t.Fatalf("%s: %v", tb.desc(), tb.err)
	}
	p.render()
	return p
}

func (tb *Tab) applyEvent(ev Event, source string) {
	if tb.err != nil {
		return
	}
	warning, err := tb.live.apply(ev)
	if err != nil {
		tb.err = fmt.Errorf("applying %s: %w:\n%s", source, err, describe(ev))
	}
	if warning != "" {
		tb.warnings = append(tb.warnings, source+": "+warning)
	}
}

// applyResponse applies a successful action response as Datastar does.
// It applies the events of an event stream, the elements of an HTML body
// and the signals of a JSON body.
func (tb *Tab) applyResponse(resp *Response) {
	tb.sync()
	if resp.Status < 200 || resp.Status > 299 {
		return
	}
	mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	switch mt {
	case "text/event-stream":
		for i, ev := range parseEvents(resp.Body) {
			tb.applyEvent(ev, fmt.Sprintf("event %d of %s", i+1, resp.desc))
		}
	case "text/html":
		if strings.TrimSpace(resp.Body) != "" {
			tb.applyEvent(Event{
				Type: PatchElements, Data: map[string]string{"elements": resp.Body},
			}, resp.desc)
		}
	case "application/json":
		tb.applyEvent(Event{
			Type: PatchSignals, Data: map[string]string{"signals": resp.Body},
		}, resp.desc)
	}
}

func (tb *Tab) desc() string { return "tab " + tb.url }

// warningText lists the patches Datastar skipped, for a failure message.
func (tb *Tab) warningText() string {
	if len(tb.warnings) == 0 {
		return ""
	}
	return "; skipped patches:\n" + strings.Join(tb.warnings, "\n")
}

// Get returns the element of the page that q selects, after applying the
// events the tab's streams received. It fails unless exactly 1 element matches.
func (tb *Tab) Get(t testing.TB, q Query) *Element {
	t.Helper()
	p := tb.current(t)
	return getOne(t, p, p.doc, q, tb.desc(), tb.warningText())
}

// All returns the elements of the page that q selects, in document order.
func (tb *Tab) All(t testing.TB, q Query) []*Element {
	t.Helper()
	p := tb.current(t)
	return getAll(t, p, p.doc, q, tb.desc())
}

// RequireNone requires the page to have no element that q selects.
func (tb *Tab) RequireNone(t testing.TB, q Query) {
	t.Helper()
	p := tb.current(t)
	ns, err := p.find(p.doc, q)
	if err != nil {
		t.Fatalf("%s: %v", tb.desc(), err)
	}
	if len(ns) > 0 {
		t.Fatalf("%s: %s, want none%s\n%s",
			tb.desc(), matching(len(ns), q), tb.warningText(), p.outline(p.doc))
	}
}

// RequireTree requires the accessibility tree of the contents of the element
// that q selects to be want. See [Node] for what the tree contains.
func (tb *Tab) RequireTree(t testing.TB, q Query, want Tree) {
	t.Helper()
	tb.Get(t, q).RequireTree(t, want)
}

// WaitFor waits until an element of the page matches q and returns it.
// The tab's streams change the page. Like [Tab.Get] it fails when 2 or more
// elements match. It also fails on timeout, and when no open stream remains
// to change the page.
func (tb *Tab) WaitFor(t testing.TB, q Query) *Element {
	t.Helper()
	var ns []*html.Node
	var p *page
	tb.wait(t, q.String(), func(pg *page) (bool, error) {
		var err error
		ns, err = pg.find(pg.doc, q)
		p = pg
		return err == nil && len(ns) > 0, err
	})
	if len(ns) > 1 {
		t.Fatalf("%s: %s, want 1%s\n%s",
			tb.desc(), matching(len(ns), q), tb.warningText(), p.outline(p.doc))
	}
	return &Element{n: ns[0], p: p, desc: tb.desc() + ": " + q.String()}
}

// WaitGone waits until no element of the page matches q.
func (tb *Tab) WaitGone(t testing.TB, q Query) {
	t.Helper()
	tb.wait(t, "no "+q.String(), func(p *page) (bool, error) {
		ns, err := p.find(p.doc, q)
		return err == nil && len(ns) == 0, err
	})
}

// wait applies stream events until done reports true.
func (tb *Tab) wait(t testing.TB, what string, done func(*page) (bool, error)) {
	t.Helper()
	timeout := tb.v.srv.conf.timeout
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		// Taking the channels before applying the events catches an event that
		// arrives in between, since its arrival closes a channel taken here.
		cases := []reflect.SelectCase{{
			Dir: reflect.SelectRecv, Chan: reflect.ValueOf(timer.C),
		}}
		for _, ts := range tb.streams {
			ts.s.mu.Lock()
			if !ts.s.ended {
				cases = append(cases, reflect.SelectCase{
					Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ts.s.changed),
				})
			}
			ts.s.mu.Unlock()
		}
		p := tb.current(t)
		ok, err := done(p)
		if err != nil {
			t.Fatalf("%s: %v", tb.desc(), err)
		}
		if ok {
			return
		}
		if len(cases) == 1 {
			t.Fatalf("%s: no open stream can change the page while waiting for %s%s\n%s",
				tb.desc(), what, tb.warningText(), p.outline(p.doc))
		}
		if chosen, _, _ := reflect.Select(cases); chosen == 0 {
			t.Fatalf("%s: waited %v for %s%s\n%s",
				tb.desc(), timeout, what, tb.warningText(), p.outline(p.doc))
		}
	}
}

// Title returns the text of the page's <title>.
func (tb *Tab) Title(t testing.TB) string {
	t.Helper()
	p := tb.current(t)
	for n := range p.doc.Descendants() {
		if n.Type == html.ElementNode && n.Data == "title" {
			return Text(n)
		}
	}
	return ""
}

// Signal returns the value of the signal at the dotted path, nil when the page
// has no such signal. Numbers are float64, as encoding/json decodes them.
func (tb *Tab) Signal(t testing.TB, path string) any {
	t.Helper()
	v, err := tb.current(t).signal(path)
	if err != nil {
		t.Fatalf("%s: reading signal %s: %v", tb.desc(), path, err)
	}
	return v
}

// SetSignal sets the signal at the dotted path to value, as a script of the page would.
// The tab sends it with its next action and [Tab.Connect].
func (tb *Tab) SetSignal(t testing.TB, path string, value any) {
	t.Helper()
	p := tb.current(t)
	if p.signals == nil {
		t.Fatalf("%s: setting signal %s: %v", tb.desc(), path, p.sigErr)
	}
	v, err := normalize(value)
	if err != nil {
		t.Fatalf("%s: setting signal %s: %v", tb.desc(), path, err)
	}
	setSignal(p.signals, path, v, false)
}

// requestSignals returns the signals the tab sends with a request, encoded.
func (tb *Tab) requestSignals(t testing.TB, f signalFilter) string {
	t.Helper()
	p := tb.current(t)
	if p.signals == nil {
		t.Fatalf("%s: the page's signals: %v", tb.desc(), p.sigErr)
	}
	b, err := json.Marshal(f.apply(p.signals))
	if err != nil {
		t.Fatalf("%s: encoding signals: %v", tb.desc(), err)
	}
	return string(b)
}
