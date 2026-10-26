package toxic

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/romshark/datapages/internal/cmd/config"
)

// Config configures a [Front].
type Config struct {
	// Host is the address the front listens on.
	Host string

	// HostSet reports whether watch.toxiproxy.host sets Host. The YAML the
	// settings page shows repeats it only then, which keeps the default port,
	// the one after the dev server, following the dev server.
	HostSet bool

	// UIHost is the address the settings page listens on,
	// and UIHostSet reports whether watch.toxiproxy.ui-host sets it.
	UIHost    string
	UIHostSet bool

	// Upstream is the URL of the dev server.
	Upstream *url.URL

	// AppName is what the settings page calls the app, as in "todolist".
	AppName string

	// TLSCert and TLSKey, when set, make the front serve HTTPS with the
	// certificate the dev server serves.
	TLSCert, TLSKey string

	// Settings are the toxics the front starts with.
	Settings Settings

	// Endpoints are the endpoints of the app the front starts with.
	Endpoints []Endpoint

	// ConfigFile is the name of the configuration file, which the settings
	// page shows. Empty means "datapages.yaml".
	ConfigFile string

	// ReloadSettings returns the settings of the configuration file.
	// The settings page calls it to discard the changes made on it.
	ReloadSettings func() (Settings, error)

	// WriteConfig writes section, the keys of watch.toxiproxy as YAML without
	// indentation, into the configuration file. The settings page calls it to
	// save the changes made on it.
	WriteConfig func(section string) error

	// GenFile is a file the generator writes on every run. A change of its
	// modification time makes the front call LoadEndpoints.
	GenFile string

	// LoadEndpoints returns the endpoints of the app as its source declares them.
	LoadEndpoints func() ([]Endpoint, error)

	// EndpointsChanged, when set, runs after the front took the endpoints
	// LoadEndpoints returned.
	EndpointsChanged func()

	Logger *slog.Logger
}

// Front is the HTTP server of watch.toxiproxy.host. It passes each request
// through the Toxiproxy proxy of its endpoint to the dev server, and applies
// the [HTTPError] toxics itself. The settings page reads and changes its settings.
type Front struct {
	conf       Config
	log        *slog.Logger
	proxies    *proxies
	routes     atomic.Pointer[routeTable]
	httpErrors atomic.Pointer[httpErrors]

	lock     sync.Mutex
	settings Settings

	// genModTime is the modification time of conf.GenFile
	// the endpoints were last loaded at.
	genModTime time.Time

	forward *httputil.ReverseProxy
	direct  *httputil.ReverseProxy

	transportsLock sync.Mutex
	transports     map[string]*http.Transport // by proxy address
}

// addrKey is the context key of the address of the Toxiproxy proxy a request
// passes through.
type addrKey struct{}

// replaceKey is the context key of the [replacement] of the response
// to a request.
type replaceKey struct{}

// replacement is the status of the downstream [HTTPError] toxic that applies
// to a request. atHead is set for the stream of a page, whose response does
// not end.
type replacement struct {
	status int
	atHead bool
}

// httpErrors maps the route pattern of an endpoint with overrides to the
// [HTTPError] toxics of its requests, and "" to those of every other request.
type httpErrors map[string][]active

func newHTTPErrors(s Settings) *httpErrors {
	h := httpErrors{"": effective(s.Global, nil, true)}
	for p, o := range s.Endpoints {
		h[p] = effective(s.Global, o, true)
	}
	return &h
}

// status returns the status of the [HTTPError] toxic of stream st that applies
// to a request of the endpoint with the pattern, by its toxicity,
// or 0 for none.
func (h httpErrors) status(pattern string, st Stream) int {
	toxics, ok := h[pattern]
	if !ok {
		toxics = h[""]
	}
	for _, a := range toxics {
		if a.slot == (Slot{Type: HTTPError, Stream: st}) &&
			rand.Float32() < a.toxic.Toxicity {
			return int(a.toxic.Values[0])
		}
	}
	return 0
}

// statusError is the status of a downstream [HTTPError] toxic,
// which [Front.replace] passes to [Front.abort].
type statusError int

func (e statusError) Error() string {
	return "http-error toxic: status " + strconv.Itoa(int(e))
}

// writeStatus answers with code the way the generated server answers an error
// that no error page or RecoverError handles, with
// [github.com/romshark/datapages/runtime/httpserve.WriteErrStatus].
func writeStatus(w http.ResponseWriter, code int) {
	http.Error(w, http.StatusText(code), code)
}

// New starts the Toxiproxy proxies and applies conf.Settings.
// [Front.Serve] or [Front.Close] stops them.
func New(conf Config) (*Front, error) {
	if conf.Logger == nil {
		conf.Logger = slog.New(slog.DiscardHandler)
	}
	p, err := newProxies(hostPort(conf.Upstream))
	if err != nil {
		return nil, err
	}
	f := &Front{
		conf:       conf,
		log:        conf.Logger,
		proxies:    p,
		settings:   conf.Settings.Clone(),
		transports: map[string]*http.Transport{},
	}
	f.routes.Store(newRouteTable(conf.Endpoints))
	f.httpErrors.Store(newHTTPErrors(f.settings))
	if fi, err := os.Stat(conf.GenFile); err == nil {
		f.genModTime = fi.ModTime()
	}
	if err := p.apply(f.settings); err != nil {
		p.close()
		return nil, fmt.Errorf("applying watch.toxiproxy: %w", err)
	}

	errLog := slog.NewLogLogger(f.log.Handler(), slog.LevelDebug)
	f.forward = &httputil.ReverseProxy{
		Rewrite:   f.rewrite,
		Transport: roundTripFunc(f.roundTrip),
		// Toxics such as bandwidth shape the stream of the response.
		// Buffering it would hide them.
		FlushInterval:  -1,
		ModifyResponse: f.replace,
		ErrorHandler:   f.abort,
		ErrorLog:       errLog,
	}
	f.direct = &httputil.ReverseProxy{
		Rewrite:   f.rewrite,
		Transport: f.transport(hostPort(conf.Upstream)),
		ErrorLog:  errLog,
	}
	return f, nil
}

// hostPort returns the address to dial for u.
func hostPort(u *url.URL) string {
	host, port := u.Hostname(), u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	return net.JoinHostPort(host, port)
}

// rewrite keeps the Host header of the browser. The app sees the origin
// the browser sees, which the cross-origin check of the generated server
// compares the Origin header against.
func (f *Front) rewrite(r *httputil.ProxyRequest) {
	r.SetURL(f.conf.Upstream)
	r.Out.Host = r.In.Host
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func (f *Front) roundTrip(r *http.Request) (*http.Response, error) {
	addr, _ := r.Context().Value(addrKey{}).(string)
	return f.transport(addr).RoundTrip(r)
}

// transport returns the transport that connects to addr only.
//
// Toxiproxy decides per connection whether a toxic applies, by its toxicity,
// and some toxics, such as limit-data, end the connection. Without keep-alive
// every request gets a connection of its own, and the toxics of its endpoint.
func (f *Front) transport(addr string) *http.Transport {
	f.transportsLock.Lock()
	defer f.transportsLock.Unlock()
	if t, ok := f.transports[addr]; ok {
		return t
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	t := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp", addr)
		},
		DisableKeepAlives: true,
		// The dev server presents the certificate of watch.tls, which may be
		// self-signed. It runs in this process, which serves the same
		// certificate to the browser.
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // see above
		},
	}
	f.transports[addr] = t
	return t
}

// replace returns the [statusError] of the [replacement] of the response,
// if any, for [Front.abort] to answer with. It first reads the response to its
// end, which lets the app finish an action that flushed its head before it
// ran. It does not wait for a stream, whose response does not end.
func (f *Front) replace(resp *http.Response) error {
	rep, ok := resp.Request.Context().Value(replaceKey{}).(replacement)
	if !ok {
		return nil
	}
	if !rep.atHead {
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			return err
		}
	}
	return statusError(rep.status)
}

// abort answers with the status of a [statusError]. Any other error ends the
// connection to the browser, as when a toxic reset the one to the dev server.
// A 502 page would read as a response of the app to the request.
func (f *Front) abort(w http.ResponseWriter, r *http.Request, err error) {
	if code, ok := errors.AsType[statusError](err); ok {
		writeStatus(w, int(code))
		return
	}
	if r.Context().Err() == nil {
		f.log.Debug("toxiproxy: request failed",
			"method", r.Method, "path", r.URL.Path, "err", err)
	}
	panic(http.ErrAbortHandler)
}

func (f *Front) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case config.TemplierEventsPath:
		// A toxic on the reload connection would show the reconnect overlay
		// of the dev server and reload the page in a loop.
		f.direct.ServeHTTP(w, r)
	default:
		e := f.routes.Load().match(r)
		errs := f.httpErrors.Load()
		if code := errs.status(e.Pattern, Upstream); code != 0 {
			writeStatus(w, code)
			return
		}
		ctx := context.WithValue(r.Context(), addrKey{}, f.proxies.addr(e.Pattern))
		if code := errs.status(e.Pattern, Downstream); code != 0 {
			ctx = context.WithValue(ctx, replaceKey{}, replacement{
				status: code, atHead: e.Kind() == KindStream,
			})
		}
		f.forward.ServeHTTP(w, r.WithContext(ctx))
	}
}

// Close stops the Toxiproxy proxies, which [New] started.
// [Front.Serve] calls it when it returns.
func (f *Front) Close() { f.proxies.close() }

// Serve serves the front on ln until ctx is done, then stops the proxies.
func (f *Front) Serve(ctx context.Context, ln net.Listener) error {
	defer f.Close()

	srv := &http.Server{
		Handler:           f,
		ReadHeaderTimeout: 10 * time.Second,
		ErrorLog:          slog.NewLogLogger(f.log.Handler(), slog.LevelDebug),
	}
	pollCtx, stopPoll := context.WithCancel(ctx)
	defer stopPoll()
	go f.pollEndpoints(pollCtx, time.Second)

	errc := make(chan error, 1)
	go func() {
		if f.conf.TLSCert != "" {
			errc <- srv.ServeTLS(ln, f.conf.TLSCert, f.conf.TLSKey)
		} else {
			errc <- srv.Serve(ln)
		}
	}()
	var err error
	select {
	case <-ctx.Done():
		// Close, not Shutdown: SSE streams would hold Shutdown until its timeout.
		_ = srv.Close()
		err = <-errc
	case err = <-errc:
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// pollEndpoints reloads the endpoints whenever conf.GenFile changes.
// The generator rewrites the file after every change to the app package.
func (f *Front) pollEndpoints(ctx context.Context, interval time.Duration) {
	if f.conf.GenFile == "" || f.conf.LoadEndpoints == nil {
		return
	}
	last := f.genModTime
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		fi, err := os.Stat(f.conf.GenFile)
		if err != nil || fi.ModTime().Equal(last) {
			continue
		}
		last = fi.ModTime()
		eps, err := f.conf.LoadEndpoints()
		if err != nil {
			// The dev server shows the errors of the app.
			f.log.Debug("toxiproxy: keeping the endpoints", "err", err)
			continue
		}
		f.setEndpoints(eps)
		if f.conf.EndpointsChanged != nil {
			f.conf.EndpointsChanged()
		}
	}
}

func (f *Front) setEndpoints(eps []Endpoint) {
	f.routes.Store(newRouteTable(eps))
}

// Endpoints returns the endpoints of the app in registration order.
func (f *Front) Endpoints() []Endpoint { return f.routes.Load().endpoints }

// Info is what the settings page shows about the front.
type Info struct {
	// AppName is what the page calls the app, as in "todolist".
	AppName string

	// Host and UIHost are the addresses of the front and of the settings page.
	Host, UIHost string

	// HostSet and UIHostSet report whether datapages.yaml sets the address.
	HostSet, UIHostSet bool

	// Upstream is the address of the dev server.
	Upstream string

	// URL and UpstreamURL are the URLs of the front and of the dev server.
	URL, UpstreamURL string

	// ConfigFile is the name of the configuration file.
	ConfigFile string
}

// Info returns what the settings page shows about f.
func (f *Front) Info() Info {
	file := f.conf.ConfigFile
	if file == "" {
		file = "datapages.yaml"
	}
	up := f.conf.Upstream
	return Info{
		AppName:     f.conf.AppName,
		Host:        f.conf.Host,
		UIHost:      f.conf.UIHost,
		HostSet:     f.conf.HostSet,
		UIHostSet:   f.conf.UIHostSet,
		Upstream:    up.Host,
		URL:         (&url.URL{Scheme: up.Scheme, Host: f.conf.Host}).String(),
		UpstreamURL: (&url.URL{Scheme: up.Scheme, Host: up.Host}).String(),
		ConfigFile:  file,
	}
}

// Reload replaces the settings with those of the configuration file.
func (f *Front) Reload() error {
	if f.conf.ReloadSettings == nil {
		return errors.New("no configuration to reload")
	}
	s, err := f.conf.ReloadSettings()
	if err != nil {
		return err
	}
	return f.Update(func(dst *Settings) error {
		*dst = s.Clone()
		return nil
	})
}

// Save writes the settings into the configuration file.
func (f *Front) Save() error {
	if f.conf.WriteConfig == nil {
		return errors.New("no configuration to save to")
	}
	return f.conf.WriteConfig(f.section(0))
}

// Saved reports whether the configuration file holds the settings. It reads
// the file, which the user may also have changed.
func (f *Front) Saved() bool {
	if f.conf.ReloadSettings == nil {
		return false
	}
	s, err := f.conf.ReloadSettings()
	if err != nil {
		return false
	}
	patterns := f.patterns()
	return s.YAML(0, patterns) == f.Settings().YAML(0, patterns)
}

// section returns the keys of watch.toxiproxy at indent spaces: the addresses
// the configuration file sets, and the settings. Empty for none.
func (f *Front) section(indent int) string {
	var b strings.Builder
	pad := strings.Repeat(" ", indent)
	if f.conf.HostSet {
		writeLine(&b, pad, "host: ", yamlString(f.conf.Host))
	}
	if f.conf.UIHostSet {
		writeLine(&b, pad, "ui-host: ", yamlString(f.conf.UIHost))
	}
	b.WriteString(f.Settings().YAML(indent, f.patterns()))
	return b.String()
}

// patterns returns the patterns of the endpoints, in the order of the YAML.
func (f *Front) patterns() []string {
	eps := f.Endpoints()
	patterns := make([]string, len(eps))
	for i, e := range eps {
		patterns[i] = e.Pattern
	}
	return patterns
}

// yamlString quotes s when it holds a character YAML gives a meaning to.
func yamlString(s string) string {
	if s == "" || strings.ContainsAny(s, ": #'\"{}[],&*!|>%@`") {
		return strconv.Quote(s)
	}
	return s
}

// Update applies change to a copy of the settings, and the copy to the proxies
// unless change returns an error.
func (f *Front) Update(change func(*Settings) error) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	s := f.settings.Clone()
	if err := change(&s); err != nil {
		return err
	}
	if err := f.proxies.apply(s); err != nil {
		// Toxiproxy validated each toxic the way Validate does,
		// so this reports a bug rather than input. Keep what it accepted.
		f.log.Error("toxiproxy: " + err.Error())
	}
	f.settings = s
	f.httpErrors.Store(newHTTPErrors(s))
	return nil
}

// Settings returns a copy of the settings.
func (f *Front) Settings() Settings {
	f.lock.Lock()
	defer f.lock.Unlock()
	return f.settings.Clone()
}
