package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"github.com/nats-io/nats.go"

	"github.com/romshark/datapages/internal/acceptance/wildcardsubjects/app"
	"github.com/romshark/datapages/internal/acceptance/wildcardsubjects/datapagesgen"
	"github.com/romshark/datapages/modules/msgbroker/natsjs"
)

func main() {
	loadEnvFile(".env")

	host := envOr("HOST", "localhost")
	port := envOr("PORT", "8080")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	var opts []datapagesgen.ServerOption
	withAccessLogger(&opts)

	messageBroker := connectNATS()

	// TODO: Initialize your app.
	a := &app.App{}
	s := datapagesgen.NewServer(a, messageBroker, opts...)
	listenAndServe(ctx, s, net.JoinHostPort(host, port))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// loadEnvFile reads a .env file and sets variables in the process environment.
// Existing variables are not overwritten. A missing file is not an error;
// anything else is reported, because a variable the file was
// supposed to carry is missing from here on.
func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
	// Scan stops on a read error and on a line too long for its buffer,
	// both of which leave the rest of the file unread.
	if err := s.Err(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "reading %s: %v\n", path, err)
	}
}

func withAccessLogger(opts *[]datapagesgen.ServerOption) {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	*opts = append(*opts, datapagesgen.WithMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger.Info("access",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path))
			next.ServeHTTP(w, r)
		})
	}))
}

func connectNATS() *natsjs.MessageBroker {
	u := os.Getenv("NATS_URL")
	if u == "" {
		slog.Error("NATS_URL not set")
		os.Exit(2)
	}

	conn, err := nats.Connect(u)
	if err != nil {
		slog.Error("opening NATS connection", slog.Any("err", err))
		os.Exit(1)
	}

	messageBroker, err := natsjs.New(conn, natsjs.Config{
		StreamConfig: &nats.StreamConfig{
			Name:    "DATAPAGES",
			Storage: nats.MemoryStorage,
		},
	})
	if err != nil {
		slog.Error("initializing message broker", slog.Any("err", err))
		os.Exit(1)
	}

	return messageBroker
}

func listenAndServe(ctx context.Context, s *datapagesgen.Server, host string) {
	pathCert := os.Getenv("PATH_TLS_CERT")
	pathKey := os.Getenv("PATH_TLS_KEY")

	var err error
	if pathCert == "" && pathKey == "" {
		err = s.ListenAndServe(ctx, host)
	} else {
		err = s.ListenAndServeTLS(ctx, host, pathCert, pathKey)
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("listening", slog.Any("err", err))
	}
}
