package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"

	"github.com/romshark/datapages/example/webcomponents/app"
	"github.com/romshark/datapages/example/webcomponents/datapagesgen"
	"github.com/romshark/datapages/modules/msgbroker/inmem"
)

func main() {
	host := envOr("HOST", "localhost")
	port := envOr("PORT", "8080")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	opts := []datapagesgen.ServerOption{
		datapagesgen.WithAssets(app.StaticFS),
	}

	s := datapagesgen.NewServer(&app.App{}, inmem.New(0), opts...)

	addr := net.JoinHostPort(host, port)
	slog.Info("listening", slog.String("addr", addr))
	if err := s.ListenAndServe(ctx, addr); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {
		slog.Error("listening", slog.Any("err", err))
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
