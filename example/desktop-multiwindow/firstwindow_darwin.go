//go:build darwin

package main

import (
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// openFirstWindow calls open once the app is active, or after 1s.
//
// A window shown before the app is active opens in the current Space.
// Run from the terminal of a fullscreen editor, it covers the editor.
// Queuing the window before Run doesn't help, because Wails activates the app
// and creates queued windows in separate goroutines.
// Activation from a terminal can fail, hence the 1s fallback.
func openFirstWindow(app *application.App, open func()) {
	var once sync.Once
	app.Event.OnApplicationEvent(events.Mac.ApplicationDidBecomeActive,
		func(*application.ApplicationEvent) { once.Do(open) })
	time.AfterFunc(time.Second, func() { once.Do(open) })
}
