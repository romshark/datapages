package main

// #cgo darwin LDFLAGS: -framework UniformTypeIdentifiers
import "C"

import (
	"context"
	"embed"
	"fmt"
	"net"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsrun "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/romshark/datapages/example/calculator/app"
	"github.com/romshark/datapages/example/calculator/datapagesgen"
	"github.com/romshark/datapages/modules/msgbroker/inmem"
)

//go:embed index.html
var assets embed.FS

func main() {
	// Find a free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	// Create and start the server in the background.
	a := new(app.App)
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

	// Launch Wails window; on DOM ready point the iframe at the real HTTP
	// server so SSE streams work without going through the buffered asset handler.
	err = wails.Run(&options.App{
		Title:     "Calculator",
		Width:     400,
		Height:    600,
		MinWidth:  260,
		MinHeight: 540,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnDomReady: func(ctx context.Context) {
			wailsrun.WindowExecJS(ctx,
				fmt.Sprintf(`document.getElementById("app").src="http://%s"`, addr))
		},
		OnShutdown: func(_ context.Context) {
			cancel()
		},
	})
	if err != nil {
		panic(err)
	}
}
