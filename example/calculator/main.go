package main

// #cgo darwin LDFLAGS: -framework UniformTypeIdentifiers
import "C"

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/romshark/datapages/example/calculator/app"
	"github.com/romshark/datapages/example/calculator/datapagesgen"
	"github.com/romshark/datapages/modules/msgbroker/inmem"
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

	// In desktop app mode we don't need neither the HMAC secret,
	// nor the NATS message broker, since it's a single-user system.
	a := app.NewApp([32]byte{})
	msgBroker := inmem.New(8)
	s := datapagesgen.NewServer(a, msgBroker,
		datapagesgen.WithAssets(app.StaticFS),
	)

	serverCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := s.ListenAndServe(serverCtx, addr); err != nil {
			panic(err)
		}
	}()

	// Wait for the HTTP server to be ready before launching the window.
	for {
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
