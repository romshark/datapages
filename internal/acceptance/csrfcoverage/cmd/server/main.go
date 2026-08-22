package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"github.com/nats-io/nats.go"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/csrfcoverage/app"
	"github.com/romshark/datapages/internal/acceptance/csrfcoverage/app/datapagesgen"
	csrfhmac "github.com/romshark/datapages/modules/csrf/hmac"
	"github.com/romshark/datapages/modules/messaging/natscore"
	"github.com/romshark/datapages/modules/sessions"
	"github.com/romshark/datapages/modules/sessions/natskv"
)

func main() {
	loadEnvFile(".env")

	host := envOr("HOST", "localhost")
	port := envOr("PORT", "8080")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	var opts []datapages.ServerOption
	withAccessLogger(&opts)
	withSessions(&opts)
	withCSRFProtection(&opts)

	messageBroker, sessionManager := connectNATS()

	// TODO: Initialize your app.
	a := &app.App{}
	opts = append(opts, datapages.WithSessionManager[struct{}](sessionManager))

	s, err := datapages.NewServer[
		app.App,
		struct{},
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](a, messageBroker, opts...)
	if err != nil {
		slog.Error("creating server", slog.Any("err", err))
		os.Exit(1)
	}
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

func withAccessLogger(opts *[]datapages.ServerOption) {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	*opts = append(*opts, datapages.WithMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger.Info("access",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path))
			next.ServeHTTP(w, r)
		})
	}))
}

func withSessions(opts *[]datapages.ServerOption) {
	*opts = append(*opts, datapages.WithSessions(datapages.SessionsConfig{}))
}

func withCSRFProtection(opts *[]datapages.ServerOption) {
	secret := os.Getenv("CSRF_SECRET")
	tm, err := csrfhmac.New([]byte(secret))
	if err != nil {
		slog.Error("initializing CSRF token manager", slog.Any("err", err))
		os.Exit(1)
	}
	*opts = append(*opts, datapages.WithCSRFProtection(datapages.CSRFConfig{
		Tokens:         tm,
		DevBypassToken: os.Getenv("CSRF_DEV_BYPASS"),
	}))
}

func connectNATS() (
	*natscore.MessageBroker,
	*natskv.SessionManager[struct{}],
) {
	u := os.Getenv("NATS_URL")
	if u == "" {
		slog.Error("NATS_URL not set")
		os.Exit(2)
	}

	sessionEncryptionKeyHex := os.Getenv("SESSION_ENCRYPTION_KEY")
	if sessionEncryptionKeyHex == "" {
		slog.Error("SESSION_ENCRYPTION_KEY not set")
		os.Exit(2)
	}
	sessionEncryptionKey, err := hex.DecodeString(sessionEncryptionKeyHex)
	if err != nil {
		slog.Error("decoding SESSION_ENCRYPTION_KEY", slog.Any("err", err))
		os.Exit(1)
	}

	conn, err := nats.Connect(u)
	if err != nil {
		slog.Error("opening NATS connection", slog.Any("err", err))
		os.Exit(1)
	}

	sessionManager, err := natskv.New[struct{}](
		conn,
		sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
		natskv.Config{EncryptionKey: sessionEncryptionKey},
	)
	if err != nil {
		slog.Error("initializing session manager", slog.Any("err", err))
		os.Exit(1)
	}

	messageBroker := natscore.New(conn, natscore.Config{})

	return messageBroker, sessionManager
}

func listenAndServe(ctx context.Context, s datapages.Server, host string) {
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
