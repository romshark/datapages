package datapages

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"

	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/sessions"
)

// IsDevMode returns true when in the development environment.
// Returns false for production environments.
//
// Only templier sets the variable, which is what "datapages watch" runs the app under.
func IsDevMode() bool { return os.Getenv("TEMPL_DEV_MODE") != "" }

// DisableSessions disables session-based authentication code generation and
// hence makes [NewServer] reject [WithSessionManager]. To enable it, use either
// a custom struct type, or an empty struct{} for sessions without data payloads.
type DisableSessions struct{}

// EnablePrometheus enables Prometheus code generation and hence makes
// [NewServer] require [WithPrometheus].
type EnablePrometheus struct{}

// DisablePrometheus disables Prometheus code generation and hence makes
// [NewServer] reject [WithPrometheus].
type DisablePrometheus struct{}

// MetricsMode constrains the Metrics type argument of [NewServer].
type MetricsMode interface {
	EnablePrometheus | DisablePrometheus
}

// Server serves a generated application returned by [NewServer].
type Server interface {
	http.Handler

	// ListenAndServe serves until ctx is canceled.
	ListenAndServe(ctx context.Context, addr string) error

	// ListenAndServeTLS serves TLS until ctx is canceled.
	ListenAndServeTLS(ctx context.Context, addr, certFile, keyFile string) error

	// Shutdown stops the server, waiting for the open requests.
	Shutdown(ctx context.Context) error
}

// ServerInitializer is the interface the generated Server type implements and
// the only way [NewServer] reaches it.
//
// Init names the application's session data type.
// A wrong SessionData type argument doesn't compile.
type ServerInitializer[App, SessionData any] interface {
	// Init wires the zero server. It runs exactly once, inside [NewServer],
	// and the server is not handed out until it returned.
	Init(
		cfg ServerConfig,
		app *App,
		broker messaging.Broker,
		sessions sessions.Manager[SessionData],
	) error
}

// NewServer creates the server S generated for application App.
// S is the Server type of the generated package, SessionData is the session
// payload type the application declares or [DisableSessions] when it declares
// none, Metrics is [EnablePrometheus] or [DisablePrometheus]:
//
//	s, err := datapages.NewServer[
//		app.App,
//		datapages.DisableSessions,
//		datapages.DisablePrometheus,
//		datapagesgen.Server,
//	](
//		a, broker, datapages.WithLogger(logger),
//	)
//
// PS is inferred from S and is never written at the call site.
//
// Options are applied in the order they are given.
func NewServer[App, SessionData any, Metrics MetricsMode, S any, PS interface {
	*S
	Server
	ServerInitializer[App, SessionData]
}](
	app *App, broker messaging.Broker, opts ...ServerOption,
) (Server, error) {
	if app == nil {
		return nil, errors.New("nil app")
	}
	if broker == nil {
		return nil, errors.New("nil message broker")
	}
	var cfg ServerConfig
	for i, opt := range opts {
		if opt == nil {
			return nil, fmt.Errorf("nil server option at index %d", i)
		}
		if err := opt(&cfg); err != nil {
			return nil, fmt.Errorf("applying server option: %w", err)
		}
	}
	sessions, err := asSessionManager[SessionData](cfg.sessionManager)
	if err != nil {
		return nil, err
	}
	s := PS(new(S))
	if err := s.Init(cfg, app, broker, sessions); err != nil {
		return nil, err
	}
	return s, nil
}

// asSessionManager asserts what [WithSessionManager] carried to the manager the
// application needs. A nil m yields a nil manager, which the generated Init
// rejects when the application declares a session data type.
//
// The assertion matches on the method set,
// not on the type argument WithSessionManager was called with.
func asSessionManager[SessionData any](
	m any,
) (sessions.Manager[SessionData], error) {
	if m == nil {
		return nil, nil
	}
	sm, ok := m.(sessions.Manager[SessionData])
	if !ok {
		return nil, fmt.Errorf(
			"WithSessionManager: %T is not a %s",
			m, reflect.TypeFor[sessions.Manager[SessionData]](),
		)
	}
	return sm, nil
}
