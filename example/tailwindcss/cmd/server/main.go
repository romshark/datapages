package main

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"github.com/romshark/datapages/example/tailwindcss/app"
	"github.com/romshark/datapages/example/tailwindcss/datapagesgen"
	"github.com/romshark/datapages/modules/msgbroker/inmem"
)

func main() {
	loadEnvFile(".env")

	host := envOr("HOST", "localhost")
	port := envOr("PORT", "8080")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	var opts []datapagesgen.ServerOption
	withAccessLogger(&opts)
	withStaticFS(&opts)

	messageBroker := inmem.New(0)

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

// loadEnvFile reads a .env file and sets variables in the process
// environment. Existing variables are not overwritten.
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

func withStaticFS(opts *[]datapagesgen.ServerOption) {
	fsStatic, err := app.FSStatic()
	if err != nil {
		slog.Error("preparing static fs", slog.Any("err", err))
		os.Exit(1)
	}
	*opts = append(*opts,
		datapagesgen.WithStaticFS("/static/", fsStatic, app.FSStaticDev()))
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
