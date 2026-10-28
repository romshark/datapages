package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"golang.org/x/mod/module"
	"golang.org/x/sync/errgroup"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/cmd/config"
	"github.com/romshark/datapages/internal/cmd/toxic"
	"github.com/romshark/datapages/internal/cmd/toxic/ui"
	"github.com/romshark/datapages/internal/cmd/toxic/ui/datapagesgen"
	"github.com/romshark/datapages/internal/cmd/toxic/ui/datapagesgen/assets"
	"github.com/romshark/datapages/internal/parser"
	"github.com/romshark/datapages/internal/serverscan"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

// toxiproxy is the Toxiproxy front of watch mode on 1 address and its
// settings page on another.
type toxiproxy struct {
	front   *toxic.Front
	frontLn net.Listener

	ui   http.Handler
	uiLn net.Listener

	tlsCert, tlsKey string
}

// serve serves the front and the settings page until ctx is done or 1 of
// them fails, which stops the other.
func (t *toxiproxy) serve(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return t.front.Serve(ctx, t.frontLn) })
	g.Go(func() error { return serveUntil(ctx, t.uiLn, t.ui, t.tlsCert, t.tlsKey) })
	return g.Wait()
}

// serveUntil serves h on ln until ctx is done.
func serveUntil(
	ctx context.Context, ln net.Listener, h http.Handler, tlsCert, tlsKey string,
) error {
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() {
		if tlsCert != "" {
			errc <- srv.ServeTLS(ln, tlsCert, tlsKey)
		} else {
			errc <- srv.Serve(ln)
		}
	}()
	var err error
	select {
	case <-ctx.Done():
		_ = srv.Close()
		err = <-errc
	case err = <-errc:
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// startToxiproxy binds the Toxiproxy front of the dev server at host and its
// settings page, which shows the app as name and Datapages as version.
// tp is watch.toxiproxy, nil without the section.
//
// With the section, a problem fails the start. Without it, a problem turns
// the proxy off with a warning and startToxiproxy returns nil, nil.
// An override of an endpoint the app lacks is a warning, because the app may
// be mid-edit.
func startToxiproxy(
	moduleDir string, app serverscan.App, name, version string,
	tp *config.WatchToxiproxy,
	host string, appHost *url.URL, tlsConf *config.WatchTLS,
	logger *slog.Logger, stderr io.Writer,
) (*toxiproxy, error) {
	configured := tp != nil
	if tp == nil {
		tp = &config.WatchToxiproxy{}
	}
	fail := func(err error) (*toxiproxy, error) {
		if configured {
			return nil, err
		}
		_, _ = fmt.Fprintf(stderr, "warning: toxiproxy is off: %v\n", err)
		return nil, nil
	}

	upstream := &url.URL{Scheme: "http", Host: host}
	if tlsConf != nil {
		upstream.Scheme = "https"
	}
	if upstream.Port() == "0" {
		// Tests run the dev server on a random port, which the proxy cannot
		// know. The default datapages.yaml has the section, which must not
		// fail such a test.
		if configured {
			_, _ = fmt.Fprintf(stderr, "warning: toxiproxy is off: it needs the port "+
				"of --host %s, which is picked at random\n", host)
		}
		return nil, nil
	}
	proxyHost := tp.Host
	if proxyHost == "" {
		var err error
		if proxyHost, err = nextPort(host); err != nil {
			return fail(fmt.Errorf("%w; set watch.toxiproxy.host", err))
		}
	}
	uiHost := tp.UIHost
	if uiHost == "" {
		var err error
		if uiHost, err = defaultUIHost(proxyHost); err != nil {
			return fail(fmt.Errorf("%w; set watch.toxiproxy.ui-host", err))
		}
	}
	// The proxy binds before the dev server and the app. On 1 of their ports it
	// makes them fail with an error that names another watch as the cause.
	for _, h := range []struct{ name, addr string }{
		{name: "watch.toxiproxy.host", addr: proxyHost},
		{name: "watch.toxiproxy.ui-host", addr: uiHost},
	} {
		_, port, _ := net.SplitHostPort(h.addr)
		for _, other := range []struct{ name, port string }{
			{name: "--host", port: upstream.Port()},
			{name: "watch.app-host", port: appHost.Port()},
		} {
			if port != "0" && port == other.port {
				return fail(fmt.Errorf(
					"%s %s uses the port of %s; set it to a free port",
					h.name, h.addr, other.name,
				))
			}
		}
	}

	appDir := filepath.Join(moduleDir, app.Dir)
	m, errs := parser.Parse(appDir)
	eps := toxic.Endpoints(m)
	if errs.Len() == 0 {
		if err := checkEndpointKeys(tp, eps); err != nil {
			_, _ = fmt.Fprintln(stderr, "warning:", err)
		}
	}

	frontLn, err := net.Listen("tcp", proxyHost)
	if err != nil {
		return fail(fmt.Errorf("listening on watch.toxiproxy.host: %w; "+
			"set it to a free port or set watch.toxiproxy.disabled", err))
	}
	uiLn, err := net.Listen("tcp", uiHost)
	if err != nil {
		_ = frontLn.Close()
		return fail(fmt.Errorf("listening on watch.toxiproxy.ui-host: %w; "+
			"set it to a free port or set watch.toxiproxy.disabled", err))
	}
	t := &toxiproxy{frontLn: frontLn, uiLn: uiLn}
	if tlsConf != nil {
		t.tlsCert, t.tlsKey = tlsConf.Cert, tlsConf.Key
	}
	closeListeners := func() {
		_ = frontLn.Close()
		_ = uiLn.Close()
	}

	var configPath string
	if name := config.FileName(moduleDir); name != "" {
		configPath = filepath.Join(moduleDir, name)
	}
	broker := inmem.New(messaging.DefaultBrokerChanBuffer)
	conf := toxic.Config{
		Host:       boundAddr(proxyHost, frontLn),
		HostSet:    tp.Host != "",
		UIHost:     boundAddr(uiHost, uiLn),
		UIHostSet:  tp.UIHost != "",
		AppName:    name,
		Version:    version,
		ConfigPath: configPath,
		Upstream:   upstream,
		TLSCert:    t.tlsCert,
		TLSKey:     t.tlsKey,
		Settings:   toxic.SettingsFromConfig(tp),
		Endpoints:  eps,
		ReloadSettings: func() (toxic.Settings, error) {
			c, _, err := config.Load(moduleDir)
			if err != nil {
				return toxic.Settings{}, err
			}
			var tp *config.WatchToxiproxy
			if c.Watch != nil {
				tp = c.Watch.Toxiproxy
			}
			return toxic.SettingsFromConfig(tp), nil
		},
		WriteConfig: func(section string) error {
			return config.SetToxiproxy(moduleDir, section)
		},
		GenFile: filepath.Join(moduleDir, app.GenDir, "app_gen.go"),
		LoadEndpoints: func() ([]toxic.Endpoint, error) {
			m, errs := parser.Parse(appDir)
			if errs.Len() > 0 {
				return nil, fmt.Errorf("parsing app package: %d error(s)", errs.Len())
			}
			return toxic.Endpoints(m), nil
		},
		// The settings page renders again on the event its own actions dispatch.
		EndpointsChanged: func() {
			err := broker.Publish(context.Background(), messaging.NoopMetrics{},
				datapagesgen.EvSubjChanged, []byte("{}"))
			if err != nil {
				logger.Error("toxiproxy: publishing the endpoints", "err", err)
			}
		},
		Logger: logger,
	}
	if t.front, err = toxic.New(conf); err != nil {
		closeListeners()
		return nil, err
	}
	t.ui, err = newSettingsServer(&ui.App{Front: t.front}, broker, logger)
	if err != nil {
		t.front.Close()
		closeListeners()
		return nil, fmt.Errorf("starting the Toxiproxy settings page: %w", err)
	}

	base := url.URL{Scheme: upstream.Scheme, Host: conf.Host}
	settings := url.URL{Scheme: upstream.Scheme, Host: conf.UIHost, Path: "/"}
	// Printed at every log level, because these addresses appear nowhere else.
	_, _ = fmt.Fprintf(stderr, "toxiproxy: %s (settings: %s)\n", &base, &settings)
	return t, nil
}

// appName returns the name the settings page shows for app, the last element
// of the module path without a major version, as in "todolist".
// With more than 1 app it adds the app directory, as in "counter/app/simple".
func appName(modulePath string, app serverscan.App, apps int) string {
	prefix, _, _ := module.SplitPathVersion(modulePath)
	name := path.Base(prefix)
	if apps > 1 {
		name += "/" + app.Dir
	}
	return name
}

// newSettingsServer returns the Datapages server of the settings page a.
func newSettingsServer(
	a *ui.App, broker messaging.Broker, logger *slog.Logger,
) (http.Handler, error) {
	static, err := fs.Sub(ui.StaticFS, "static")
	if err != nil {
		return nil, err
	}
	return datapages.NewServer[
		ui.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](
		a,
		broker,
		// Not WithAssets, which reads from the source tree in dev mode.
		// The user's shell can turn dev mode on for this process,
		// and an installed CLI has no source tree.
		datapages.WithAssetsFS(http.FS(static), false),
		datapages.WithDatastarJS(assets.Path("datastar-1.0.4.js")),
		datapages.WithLogger(logger),
		// The browser runs on the same machine, where compression saves nothing.
		datapages.WithNoResponseCompression(),
		datapages.WithNoAssetsCompression(),
	)
}

// boundAddr returns addr, or the address of ln when addr has port 0.
func boundAddr(addr string, ln net.Listener) string {
	if _, port, _ := net.SplitHostPort(addr); port == "0" {
		return ln.Addr().String()
	}
	return addr
}

// defaultUIHost returns proxyHost with the next port,
// or with port 0 when proxyHost has port 0.
func defaultUIHost(proxyHost string) (string, error) {
	if h, port, err := net.SplitHostPort(proxyHost); err == nil && port == "0" {
		return net.JoinHostPort(h, "0"), nil
	}
	return nextPort(proxyHost)
}

// nextPort returns host with the port after its own,
// as in "localhost:7332" for "localhost:7331".
func nextPort(host string) (string, error) {
	h, port, err := net.SplitHostPort(host)
	if err != nil {
		return "", fmt.Errorf("reading the port of %s: %w", host, err)
	}
	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil || n == 0 || n == 65535 {
		return "", fmt.Errorf("%s has no next port", host)
	}
	return net.JoinHostPort(h, strconv.FormatUint(n+1, 10)), nil
}

// checkEndpointKeys reports the override keys of
// the presets of tp that name no endpoint in eps.
func checkEndpointKeys(tp *config.WatchToxiproxy, eps []toxic.Endpoint) error {
	var errs []error
	for _, p := range tp.Presets {
		where := fmt.Sprintf("watch.toxiproxy.presets[%q].endpoints", p.Name)
		errs = append(errs,
			toxic.CheckEndpointKeys(where, endpointKeys(p.Endpoints), eps))
	}
	return errors.Join(errs...)
}

// endpointKeys returns the keys of e, sorted.
func endpointKeys(e map[config.EndpointPattern]config.Toxics) []string {
	keys := make([]string, 0, len(e))
	for k := range e {
		keys = append(keys, string(k))
	}
	slices.Sort(keys)
	return keys
}
