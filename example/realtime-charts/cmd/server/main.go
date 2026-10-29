package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/realtime-charts/app"
	"github.com/romshark/datapages/example/realtime-charts/app/datapagesgen"
	"github.com/romshark/datapages/example/realtime-charts/app/datapagesgen/assets"
	"github.com/romshark/datapages/example/realtime-charts/market"
	"github.com/romshark/datapages/example/realtime-charts/tsdb"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func main() {
	host := envOr("HOST", "localhost")
	port := envOr("PORT", "8080")
	// History bounds retained prices and reachable paused links.
	history, err := time.ParseDuration(envOr("HISTORY", "1h"))
	if err != nil || history <= 0 {
		slog.Error("HISTORY is not a positive duration",
			slog.String("HISTORY", os.Getenv("HISTORY")))
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// The market prefills history before its goroutine starts.
	db := tsdb.New(history)
	m := market.New(db, time.Now(), history)

	// The database and streams share 1 process.
	// The buffer holds a full tick,
	// preventing a busy stream from dropping later index events.
	broker := inmem.New(4 * len(market.Indexes))

	// Pages and Rocket chart elements share this Datastar bundle.
	s, err := datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](app.New(db, m), broker,
		datapages.WithAssets(app.StaticFS, false),
		datapages.WithDatastarJS(assets.Path("datastar-rocket.js")))
	if err != nil {
		slog.Error("creating server", slog.Any("err", err))
		os.Exit(1)
	}

	go runMarket(ctx, m, broker)
	listenAndServe(ctx, s, net.JoinHostPort(host, port))
}

// runMarket publishes 1 event per index after each tick.
// It uses the broker directly because it runs outside a handler.
// Symbols need no subject escaping; see [market.Indexes].
func runMarket(ctx context.Context, m *market.Market, p messaging.Publisher) {
	m.Run(ctx, func(symbol string) {
		// Marshaling a struct of a string cannot fail.
		ev, _ := json.Marshal(app.EventIndexTicked{Index: datapages.Subject(symbol)})
		subj := datapagesgen.EvPrefixIndexTicked + symbol
		err := p.Publish(ctx, messaging.NoopMetrics{}, subj, ev)
		if err != nil && ctx.Err() == nil {
			slog.Error("publishing index tick",
				slog.String("index", symbol), slog.Any("err", err))
		}
	})
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func listenAndServe(ctx context.Context, s datapages.Server, host string) {
	pathCert := os.Getenv("PATH_TLS_CERT")
	pathKey := os.Getenv("PATH_TLS_KEY")

	slog.Info("listening", slog.String("addr", host))
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
