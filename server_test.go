package datapages_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/sessions"
)

// testApp stands in for the application package's App type.
type testApp struct{}

// testSessionData stands in for the session data an application declares.
type testSessionData struct{ Name string }

// testServing stands in for the HTTP server the generated code embeds.
// The tests call Init, nothing here ever listens.
type testServing struct{}

func (testServing) ServeHTTP(http.ResponseWriter, *http.Request) {}

func (testServing) ListenAndServe(context.Context, string) error { return nil }

func (testServing) ListenAndServeTLS(context.Context, string, string, string) error {
	return nil
}

func (testServing) Shutdown(context.Context) error { return nil }

// testServer stands in for a generated Server without sessions.
type testServer struct {
	testServing
	initCalls int
	cfg       datapages.ServerConfig
	app       *testApp
	broker    messaging.Broker
	sessions  sessions.Manager[datapages.DisableSessions]
}

func (s *testServer) Init(
	cfg datapages.ServerConfig, app *testApp, broker messaging.Broker,
	sessions sessions.Manager[datapages.DisableSessions],
) error {
	s.initCalls++
	s.cfg, s.app, s.broker, s.sessions = cfg, app, broker, sessions
	return nil
}

// testSessionServer stands in for a generated Server with sessions.
type testSessionServer struct {
	testServing
	sessions sessions.Manager[testSessionData]
}

func (s *testSessionServer) Init(
	_ datapages.ServerConfig, _ *testApp, _ messaging.Broker,
	sessions sessions.Manager[testSessionData],
) error {
	if sessions == nil {
		return errors.New("missing option WithSessionManager")
	}
	s.sessions = sessions
	return nil
}

// failingServer stands in for a generated Server rejecting its configuration.
var errInit = errors.New("init failed")

type failingServer struct{ testServing }

func (failingServer) Init(
	datapages.ServerConfig, *testApp, messaging.Broker,
	sessions.Manager[datapages.DisableSessions],
) error {
	return errInit
}

func TestNewServer(t *testing.T) {
	app, broker := new(testApp), inmem.New(1)
	s, err := datapages.NewServer[
		testApp,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		testServer,
	](
		app, broker, datapages.WithDatastarJS("/ds.js"),
	)
	require.NoError(t, err)
	srv, ok := s.(*testServer)
	require.True(t, ok)
	require.Equal(t, 1, srv.initCalls)
	require.Same(t, app, srv.app)
	require.Equal(t, broker, srv.broker)
	require.Nil(t, srv.sessions)
	require.Equal(t, "/ds.js", srv.cfg.DatastarJS)
}

func TestNewServerSessions(t *testing.T) {
	m := new(testSessionManager)
	s, err := datapages.NewServer[
		testApp,
		testSessionData,
		datapages.DisablePrometheus,
		testSessionServer,
	](
		new(testApp), inmem.New(1), datapages.WithSessionManager(m),
	)
	require.NoError(t, err)
	srv, ok := s.(*testSessionServer)
	require.True(t, ok)
	require.Same(t, m, srv.sessions)
}

func TestNewServerErr(t *testing.T) {
	app, broker := new(testApp), inmem.New(1)
	for name, tt := range map[string]struct {
		exec func() error
		msg  string
	}{
		"nil app": {func() error {
			_, err := datapages.NewServer[
				testApp,
				datapages.DisableSessions,
				datapages.DisablePrometheus,
				testServer,
			](nil, broker)
			return err
		}, "nil app"},
		"nil broker": {func() error {
			_, err := datapages.NewServer[
				testApp,
				datapages.DisableSessions,
				datapages.DisablePrometheus,
				testServer,
			](app, nil)
			return err
		}, "nil message broker"},
		"nil option": {func() error {
			_, err := datapages.NewServer[
				testApp,
				datapages.DisableSessions,
				datapages.DisablePrometheus,
				testServer,
			](app, broker, nil)
			return err
		}, "nil server option at index 0"},
		"failing option": {func() error {
			_, err := datapages.NewServer[
				testApp,
				datapages.DisableSessions,
				datapages.DisablePrometheus,
				testServer,
			](app, broker, datapages.WithMiddleware(nil))
			return err
		}, "applying server option: WithMiddleware: nil middleware at index 0"},
		"failing init": {func() error {
			_, err := datapages.NewServer[
				testApp,
				datapages.DisableSessions,
				datapages.DisablePrometheus,
				failingServer,
			](app, broker)
			return err
		}, errInit.Error()},
		"session manager of another type": {func() error {
			_, err := datapages.NewServer[
				testApp,
				datapages.DisableSessions,
				datapages.DisablePrometheus,
				testServer,
			](app, broker, datapages.WithSessionManager(new(testSessionManager)))
			return err
		}, "WithSessionManager: *datapages_test.testSessionManager is not a " +
			"sessions.Manager[github.com/romshark/datapages." +
			"DisableSessions]"},
		"missing session manager": {func() error {
			_, err := datapages.NewServer[
				testApp,
				testSessionData,
				datapages.DisablePrometheus,
				testSessionServer,
			](app, broker)
			return err
		}, "missing option WithSessionManager"},
	} {
		t.Run(name, func(t *testing.T) {
			err := tt.exec()
			require.Error(t, err)
			require.Equal(t, tt.msg, err.Error())
		})
	}
}

func TestWithSessionManagerErr(t *testing.T) {
	for name, tt := range map[string]struct {
		opts []datapages.ServerOption
		msg  string
	}{
		"nil": {
			[]datapages.ServerOption{
				datapages.WithSessionManager[testSessionData](nil),
			},
			"applying server option: WithSessionManager: nil session manager",
		},
		"twice": {
			[]datapages.ServerOption{
				datapages.WithSessionManager(new(testSessionManager)),
				datapages.WithSessionManager(new(testSessionManager)),
			},
			"applying server option: WithSessionManager: " +
				"session manager already set",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := datapages.NewServer[
				testApp,
				testSessionData,
				datapages.DisablePrometheus,
				testSessionServer,
			](new(testApp), inmem.New(1), tt.opts...)
			require.Error(t, err)
			require.Equal(t, tt.msg, err.Error())
		})
	}
}

// testSessionManager implements sessions.Manager[testSessionData].
type testSessionManager struct{}

func (*testSessionManager) ReadSessionFromCookie(string) (
	sessions.Record[testSessionData], string, bool, error,
) {
	return sessions.Record[testSessionData]{}, "", false, nil
}

func (*testSessionManager) CreateSession(
	context.Context, sessions.Record[testSessionData],
) (string, error) {
	return "", nil
}

func (*testSessionManager) NotifyClosed(context.Context, string, func()) error {
	return nil
}

func (*testSessionManager) CloseSession(context.Context, string) error { return nil }

func (*testSessionManager) DeleteExpired(context.Context) (int, error) { return 0, nil }
