package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	sqinn "github.com/cvilsmeier/sqinn-go/v2"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/sqlitesessions/app"
	"github.com/romshark/datapages/example/sqlitesessions/app/datapagesgen"
	"github.com/romshark/datapages/example/sqlitesessions/app/sessionstore"
	"github.com/romshark/datapages/example/sqlitesessions/app/sqdb"
	"github.com/romshark/datapages/example/sqlitesessions/app/userstore"
	csrfhmac "github.com/romshark/datapages/modules/csrf/hmac"
	"github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/sessions"
)

func main() {
	host := envOr("HOST", "localhost")
	port := envOr("PORT", "8080")
	dbPath := envOr("SESSION_DB_PATH", "./sqlitesessions.db")
	// Override in production.
	csrfSecret := envOr("CSRF_SECRET", "dev-csrf-secret-change-me-in-production")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	sq, err := sqinn.Launch(sqinn.Options{
		Sqinn: resolveSqinnPath(),
		Db:    dbPath,
		Log:   func(m string) { slog.Debug("sqinn", slog.String("msg", m)) },
	})
	if err != nil {
		slog.Error("launching sqinn", slog.Any("err", err))
		os.Exit(1)
	}
	defer func() { _ = sq.Close() }()

	for _, p := range []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA synchronous = NORMAL`,
	} {
		if err := sq.ExecSql(p); err != nil {
			slog.Error("applying pragma",
				slog.String("pragma", p), slog.Any("err", err))
			os.Exit(1)
		}
	}

	conn := sqdb.New(sq, slog.Default())

	userStore, err := userstore.New(conn)
	if err != nil {
		slog.Error("initializing user store", slog.Any("err", err))
		os.Exit(1)
	}
	if err := seedDemoUsers(ctx, userStore); err != nil {
		slog.Error("seeding demo users", slog.Any("err", err))
		os.Exit(1)
	}
	sessionStore, err := sessionstore.New(
		conn,
		sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
		7*24*time.Hour,
		slog.Default(),
	)
	if err != nil {
		slog.Error("initializing session store", slog.Any("err", err))
		os.Exit(1)
	}

	csrfTM, err := csrfhmac.New([]byte(csrfSecret))
	if err != nil {
		slog.Error("initializing CSRF token manager", slog.Any("err", err))
		os.Exit(1)
	}

	a := app.NewApp(userStore)
	s, err := datapages.NewServer[
		app.App,
		app.SessionData,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](
		a,
		inmem.New(0),
		datapages.WithSessionManager[app.SessionData](sessionStore),
		datapages.WithSessions(datapages.SessionsConfig{}),
		datapages.WithCSRFProtection(datapages.CSRFConfig{
			Tokens:         csrfTM,
			DevBypassToken: os.Getenv("CSRF_DEV_BYPASS"),
		}),
		datapages.WithMiddleware(accessLog()),
	)
	if err != nil {
		slog.Error("creating server", slog.Any("err", err))
		os.Exit(1)
	}

	addr := net.JoinHostPort(host, port)
	slog.Info("listening", slog.String("addr", addr))
	if err := s.ListenAndServe(ctx, addr); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {
		slog.Error("listening", slog.Any("err", err))
		os.Exit(1)
	}
}

// resolveSqinnPath honors SQINN_PATH, otherwise uses the prebuilt
// binary shipped with sqinn-go v2.
func resolveSqinnPath() string {
	if p := os.Getenv("SQINN_PATH"); p != "" {
		return p
	}
	return sqinn.Prebuilt
}

func envOr(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

// seedDemoUsers gives the example something to log in with on first run.
func seedDemoUsers(ctx context.Context, users *userstore.Store) error {
	seed := []struct{ name, email, password string }{
		{"Alice Ada", "alice@example.com", "password123"},
		{"Bob Builder", "bob@example.com", "password123"},
		{"Carol Codes", "carol@example.com", "password123"},
	}
	inserted := 0
	for _, u := range seed {
		_, err := users.Register(ctx, u.name, u.email, u.password)
		switch {
		case err == nil:
			inserted++
		case errors.Is(err, userstore.ErrEmailAlreadyInUse):
			// Already present.
		default:
			return fmt.Errorf("seeding %s: %w", u.email, err)
		}
	}
	if inserted > 0 {
		slog.Info("seeded demo users", slog.Int("inserted", inserted))
	}
	return nil
}

func accessLog() func(next http.Handler) http.Handler {
	l := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			l.Info("access",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path))
			next.ServeHTTP(w, r)
		})
	}
}
