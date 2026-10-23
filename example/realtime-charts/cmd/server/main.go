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
	// The history is how far back the database keeps prices, which is as far back as a
	// paused link and a tab that resumes can reach.
	history, err := time.ParseDuration(envOr("HISTORY", "1h"))
	if err != nil || history <= 0 {
		slog.Error("HISTORY is not a positive duration",
			slog.String("HISTORY", os.Getenv("HISTORY")))
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// The market prefills the database with the history and writes into it from its own
	// goroutine once the server runs.
	db := tsdb.New(history)
	m := market.New(db, time.Now(), history)

	// The database and every stream that shows it live in this process. A broker between
	// processes would carry the ticks of databases that differ. The market publishes the
	// events of all indexes at once. A buffer for all of them keeps a busy stream from
	// dropping the events of the last indexes, which a tab that shows only those would
	// miss pushes for.
	broker := inmem.New(4 * len(market.Indexes))

	// The chart elements are Rocket components. The pages load Datastar with Rocket from
	// the file the elements import, which keeps one Datastar on the page.
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

// runMarket ticks m every second and publishes an event for every index after each tick,
// each on the subject of its index. The market runs outside of any handler, which is why
// it publishes on the broker directly instead of through a [datapages.Dispatcher]. A
// dispatcher escapes the subject value. Symbols need no escaping, see [market.Indexes].
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
