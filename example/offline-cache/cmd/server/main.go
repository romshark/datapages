package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"

	"github.com/romshark/datapages/example/offline-cache/app"
	"github.com/romshark/datapages/example/offline-cache/datapagesgen"
	"github.com/romshark/datapages/example/offline-cache/datapagesgen/assets"
	csrfhmac "github.com/romshark/datapages/modules/csrf/hmac"
	msgbrokerinmem "github.com/romshark/datapages/modules/msgbroker/inmem"
	sessinmem "github.com/romshark/datapages/modules/sessmanager/inmem"
	"github.com/romshark/datapages/modules/sesstokgen"
)

func main() {
	fHost := flag.String("host", "localhost:8080", "server host address")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// The entire demo runs on in-memory infrastructure — no external services
	// (NATS, databases) are required. All data is lost when the process exits.
	repo := NewRepository()
	a := app.NewApp(repo)

	messageBroker := msgbrokerinmem.New(8)
	sessionManager := sessinmem.New[struct{}](sesstokgen.Generator{
		Length: sesstokgen.DefaultLength,
	})

	opts := []datapagesgen.ServerOption{
		datapagesgen.WithAuth(datapagesgen.AuthConfig{}),
		datapagesgen.WithAssets(app.StaticFS),
		// Self-host Datastar so the app also works offline.
		datapagesgen.WithDatastarJS(assets.Path("datastar.js")),
		// Service-worker-based offline support: serves the worker and injects
		// its registration into every page. The PageOffline route is supplied
		// by the generated option.
		datapagesgen.WithOffline(app.OfflineConfig()),
	}
	withCSRFProtection(&opts)

	s := datapagesgen.NewServer(a, messageBroker, sessionManager, opts...)

	slog.Info("listening", slog.String("addr", *fHost))
	if err := s.ListenAndServe(ctx, *fHost); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {
		slog.Error("serving", slog.Any("err", err))
		os.Exit(1)
	}
}

func withCSRFProtection(opts *[]datapagesgen.ServerOption) {
	secret := os.Getenv("CSRF_SECRET")
	if secret == "" {
		// Development fallback. Set CSRF_SECRET in real deployments.
		secret = "dev-only-csrf-secret-change-me"
	}
	tm, err := csrfhmac.New([]byte(secret))
	if err != nil {
		slog.Error("initializing CSRF token manager", slog.Any("err", err))
		os.Exit(1)
	}
	*opts = append(*opts, datapagesgen.WithCSRFProtection(datapagesgen.CSRFConfig{
		TokenManager:   tm,
		DevBypassToken: os.Getenv("CSRF_DEV_BYPASS"),
	}))
}
