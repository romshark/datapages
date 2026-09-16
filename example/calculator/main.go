package main

// #cgo darwin LDFLAGS: -framework UniformTypeIdentifiers
import "C"

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/calculator/app"
	"github.com/romshark/datapages/example/calculator/app/datapagesgen"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func main() {
	// Find a free port.
	// TODO: The port could be taken between Close and ListenAndServe (race).
	// Fix by adding a Serve(net.Listener) method to the generated server.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	a := app.NewApp()
	// The application dispatches no events. NewServer still requires a broker.
	msgBroker := inmem.New(0)
	s, err := datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](a, msgBroker, datapages.WithAssets(app.StaticFS, false))
	if err != nil {
		panic(err)
	}

	serverCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- s.ListenAndServe(serverCtx, addr)
	}()

	// Wait for the HTTP server to be ready before launching the window.
	for {
		select {
		case err := <-serverErr:
			panic(fmt.Sprintf("server failed to start: %v", err))
		default:
		}
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
	}

	wailsApp := application.New(application.Options{
		Name: "Calculator",
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		OnShutdown: func() {
			cancel()
		},
	})

	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "Calculator",
		Width:     400,
		Height:    600,
		MinWidth:  260,
		MinHeight: 540,
		URL:       fmt.Sprintf("http://%s", addr),
	})

	if err := wailsApp.Run(); err != nil {
		panic(err)
	}
}
