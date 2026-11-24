package app

import (
	"cmp"
	"fmt"
	"net"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/romshark/datapages"
)

// Desktop opens and controls the windows of the desktop app.
type Desktop interface {
	OpenWindow() error
	// SetTitle sets the title of the window on port.
	SetTitle(port, title string)
	// Focus brings the window on port to the front.
	Focus(port string)
}

// App tracks the windows whose SSE stream is open.
type App struct {
	desktop Desktop

	lock       sync.Mutex
	lastNumber int
	windows    map[datapages.StreamID]window
}

type window struct {
	Number int
	// Port is the port the window loads from.
	Port string
}

// listed is a window as the list shows it.
type listed struct {
	window
	// Shared reports whether another window uses the same port.
	Shared bool
}

// NewApp returns an App. With a nil desktop, the page shows a link to a new
// browser tab instead of the "Open window" button.
func NewApp(desktop Desktop) *App {
	return &App{
		desktop: desktop,
		windows: make(map[datapages.StreamID]window),
	}
}

func (*App) Head(_ *http.Request) datapages.Head { return head() }

// view returns the page of window self. self is zero until the window's stream opens.
func (a *App) view(self window) view {
	return view{Desktop: a.desktop != nil, Self: self, Windows: a.list()}
}

// register adds the window of a stream and returns it with its number.
func (a *App) register(id datapages.StreamID, port string) window {
	a.lock.Lock()
	defer a.lock.Unlock()
	a.lastNumber++
	w := window{Number: a.lastNumber, Port: port}
	a.windows[id] = w
	return w
}

func (a *App) unregister(id datapages.StreamID) {
	a.lock.Lock()
	defer a.lock.Unlock()
	delete(a.windows, id)
}

// port returns the port of the window with number, or false if it is closed.
func (a *App) port(number int) (string, bool) {
	a.lock.Lock()
	defer a.lock.Unlock()
	for _, w := range a.windows {
		if w.Number == number {
			return w.Port, true
		}
	}
	return "", false
}

// list returns the connected windows ordered by number.
func (a *App) list() []listed {
	a.lock.Lock()
	defer a.lock.Unlock()
	perPort := make(map[string]int, len(a.windows))
	for _, w := range a.windows {
		perPort[w.Port]++
	}
	l := make([]listed, 0, len(a.windows))
	for _, w := range a.windows {
		l = append(l, listed{window: w, Shared: perPort[w.Port] > 1})
	}
	slices.SortFunc(l, func(x, y listed) int { return cmp.Compare(x.Number, y.Number) })
	return l
}

// StateIndex holds the window of 1 stream. A reconnected stream gets a new number.
type StateIndex struct{ Self window }

// EventWindowsChanged is "windows.changed"
type EventWindowsChanged struct{}

// EventPinged is "windows.pinged"
type EventPinged struct {
	From int       `json:"from"`
	At   time.Time `json:"at"`
}

// view is what [pageIndex] renders.
type view struct {
	// Desktop reports whether the page is in a window of the desktop app.
	Desktop bool
	// Self is zero until the window's stream opens.
	Self    window
	Windows []listed
}

// PageIndex is /
type PageIndex struct{ App *App }

// GET keeps the stream open while the window is minimized or covered.
// By default a hidden page closes its stream, and the window would drop off the list.
func (p PageIndex) GET(_ *http.Request) (
	body datapages.Component,
	enableBackgroundStreaming datapages.EnableBackgroundStreaming,
	err error,
) {
	return pageIndex(p.App.view(window{})), true, nil
}

// StreamOpen renders the new window's page, sets its title and tells the other windows.
// It renders the page itself because the new window may miss the event it dispatches.
func (p PageIndex) StreamOpen(
	r *http.Request,
	streamID datapages.StreamID,
	state datapages.State[StateIndex],
	sse datapages.SSE,
	windowsChanged datapages.Dispatcher[EventWindowsChanged],
) error {
	self := p.App.register(streamID, portOf(r))
	state.Values.Self = self
	err := windowsChanged.Dispatch(EventWindowsChanged{})
	if err == nil {
		err = sse.PatchElement(pageIndex(p.App.view(self)))
	}
	if err != nil {
		// StreamClose does not run after a failed StreamOpen.
		p.App.unregister(streamID)
		return err
	}
	if p.App.desktop != nil {
		p.App.desktop.SetTitle(self.Port, fmt.Sprintf("Window %d", self.Number))
	}
	return nil
}

func (p PageIndex) StreamClose(
	_ *http.Request,
	streamID datapages.StreamID,
	windowsChanged datapages.Dispatcher[EventWindowsChanged],
) error {
	p.App.unregister(streamID)
	return windowsChanged.Dispatch(EventWindowsChanged{})
}

// POSTOpenWindow is /open-window/{$}
func (p PageIndex) POSTOpenWindow(_ *http.Request) error {
	if p.App.desktop == nil {
		return fmt.Errorf("%w: the server opens no windows", datapages.ErrNotFound)
	}
	return p.App.desktop.OpenWindow()
}

// POSTFocus is /focus/{number}
func (p PageIndex) POSTFocus(
	_ *http.Request,
	path datapages.Path[struct {
		Number int `path:"number"`
	}],
) error {
	if p.App.desktop == nil {
		return fmt.Errorf("%w: the server has no windows", datapages.ErrNotFound)
	}
	port, ok := p.App.port(path.Values.Number)
	if !ok {
		return fmt.Errorf("%w: window %d is closed",
			datapages.ErrNotFound, path.Values.Number)
	}
	p.App.desktop.Focus(port)
	return nil
}

// POSTPing is /ping/{$}
func (p PageIndex) POSTPing(
	_ *http.Request,
	state datapages.State[StateIndex],
	pinged datapages.Dispatcher[EventPinged],
) error {
	return pinged.Dispatch(EventPinged{From: state.Values.Self.Number, At: time.Now()})
}

func (p PageIndex) OnWindowsChanged(
	_ EventWindowsChanged,
	sse datapages.SSE,
	state datapages.State[StateIndex],
) error {
	return sse.PatchElement(
		windowList(state.Values.Self, p.App.list(), p.App.desktop != nil),
	)
}

func (p PageIndex) OnPinged(
	event EventPinged,
	sse datapages.SSE,
	state datapages.State[StateIndex],
) error {
	return sse.PatchElement(pinged(
		event.From, state.Values.Self.Number, event.At.Format(time.TimeOnly),
	))
}

// portOf returns the port of r.Host, or r.Host if it has no port.
func portOf(r *http.Request) string {
	if _, port, err := net.SplitHostPort(r.Host); err == nil {
		return port
	}
	return r.Host
}

// othersOnPort returns how many other windows share the port of self.
func othersOnPort(self window, windows []listed) int {
	n := 0
	for _, w := range windows {
		if w.Port == self.Port && w.Number != self.Number {
			n++
		}
	}
	return n
}

// countWindows returns "1 window" or "n windows".
func countWindows(n int) string {
	if n == 1 {
		return "1 window"
	}
	return fmt.Sprintf("%d windows", n)
}
