package main

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/desktop-multiwindow/app"
	"github.com/romshark/datapages/example/desktop-multiwindow/app/datapagesgen"
	"github.com/romshark/datapages/example/desktop-multiwindow/guard"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func main() {
	fSharedPort := flag.Bool("shared-port", false,
		"load all windows from 1 port, which limits the app to 6 windows")
	flag.Parse()

	wailsApp := application.New(application.Options{
		Name: "Multiwindow",
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// windows serves the ports of all windows.
	// WriteTimeout stays 0 because a timeout would end the SSE streams.
	windows := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       time.Minute,
	}
	desktop := &wailsDesktop{
		wails:  wailsApp,
		server: windows,
		token:  rand.Text(),
		byPort: make(map[string]*application.WebviewWindow),
	}

	s, err := datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](
		app.NewApp(desktop),
		inmem.New(messaging.DefaultBrokerChanBuffer),
		// The only clients are local webviews, so compression only costs CPU.
		datapages.WithNoSSECompression(),
		datapages.WithNoResponseCompression(),
	)
	if err != nil {
		panic(err)
	}
	// s doesn't listen. windows passes it every request with the launch token.
	windows.Handler = guard.Handler(desktop.token, s)

	if *fSharedPort {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			panic(err)
		}
		desktop.shared = ln
		go serve(windows, ln)
	}

	openFirstWindow(wailsApp, func() {
		if err := desktop.OpenWindow(); err != nil {
			slog.Error("opening the first window", slog.Any("err", err))
			wailsApp.Quit()
		}
	})
	if err := wailsApp.Run(); err != nil {
		panic(err)
	}

	// 10s matches the default of datapages.WithShutdownTimeout.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// s.Shutdown ends the SSE streams, then windows.Shutdown waits for StreamClose.
	if err := errors.Join(s.Shutdown(ctx), windows.Shutdown(ctx)); err != nil {
		slog.Error("shutting down", slog.Any("err", err))
	}
}

// wailsDesktop opens Wails windows, each on its own port unless shared is set.
// A webview opens at most 6 connections per port, and each SSE stream holds 1 open.
type wailsDesktop struct {
	wails  *application.App
	server *http.Server
	// token is the launch token of guard.Handler, new on every launch.
	token string
	// shared is the listener all windows load from with -shared-port.
	shared net.Listener
	opened atomic.Int32

	lock sync.Mutex
	// byPort holds the open windows by port, except those of -shared-port.
	byPort map[string]*application.WebviewWindow
}

// OpenWindow is safe for concurrent use. Wails creates the window on the main thread.
func (d *wailsDesktop) OpenWindow() error {
	ln := d.shared
	if ln == nil {
		var err error
		if ln, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
			return fmt.Errorf("listening for a window: %w", err)
		}
		go serve(d.server, ln)
	}
	// Shift each window down and right of the last one so it doesn't cover it,
	// starting over after 10.
	offset := int(d.opened.Add(1)-1) % 10 * 32
	options := application.WebviewWindowOptions{
		Title:           "Multiwindow",
		Width:           480,
		Height:          560,
		MinWidth:        360,
		MinHeight:       400,
		InitialPosition: application.WindowXY,
		X:               64 + offset,
		Y:               64 + offset,
		URL:             guard.URL(ln.Addr().String(), d.token),
	}
	if ln == d.shared {
		d.wails.Window.NewWithOptions(options)
		return nil
	}

	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	// The page starts loading before NewWithOptions returns.
	// The lock makes SetTitle wait until w is in byPort.
	d.lock.Lock()
	w := d.wails.Window.NewWithOptions(options)
	d.byPort[port] = w
	d.lock.Unlock()

	// Closing the listener frees the port. The webview closes its own connections.
	// Wails calls this in a goroutine. On the main thread it would deadlock with
	// OpenWindow, which holds the lock while it waits for the main thread.
	w.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		d.lock.Lock()
		delete(d.byPort, port)
		d.lock.Unlock()
		_ = ln.Close()
	})
	return nil
}

// SetTitle does nothing with -shared-port, where the port can't tell windows apart.
func (d *wailsDesktop) SetTitle(port, title string) {
	if w := d.window(port); w != nil {
		w.SetTitle(title)
	}
}

// Focus does nothing with -shared-port, where the port can't tell windows apart.
func (d *wailsDesktop) Focus(port string) {
	if w := d.window(port); w != nil {
		w.UnMinimise()
		w.Focus()
	}
}

// window returns the open window on port, or nil.
func (d *wailsDesktop) window(port string) *application.WebviewWindow {
	d.lock.Lock()
	defer d.lock.Unlock()
	return d.byPort[port]
}

// serve serves ln until ln or srv closes.
func serve(srv *http.Server, ln net.Listener) {
	err := srv.Serve(ln)
	if err != nil && !errors.Is(err, net.ErrClosed) &&
		!errors.Is(err, http.ErrServerClosed) {
		slog.Error("serving a window",
			slog.String("addr", ln.Addr().String()), slog.Any("err", err))
	}
}
