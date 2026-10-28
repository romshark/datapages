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
	"path/filepath"
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

	// HostSet reports whether watch.toxiproxy.host sets Host. Only then does
	// the settings page save Host, which keeps the default port following the
	// port of the dev server.
	HostSet bool

	// UIHost is the address the settings page listens on,
	// and UIHostSet reports whether watch.toxiproxy.ui-host sets it.
	UIHost    string
	UIHostSet bool

	// Upstream is the URL of the dev server.
	Upstream *url.URL

	// AppName is what the settings page calls the app, as in "todolist".
	AppName string

	// Version is the version of Datapages the settings page shows.
	Version string

	// TLSCert and TLSKey make the front serve HTTPS with the certificate of
	// the dev server.
	TLSCert, TLSKey string

	// Settings are the toxics the front starts with.
	Settings Settings

	// Endpoints are the endpoints of the app the front starts with.
	Endpoints []Endpoint

	// ConfigPath is the absolute path of the configuration file the settings
	// page shows. Empty means "datapages.yaml" in an unknown directory.
	ConfigPath string

	// ReloadSettings reads the settings from the configuration file.
	ReloadSettings func() (Settings, error)

	// WriteConfig writes section, the unindented YAML of the keys of
	// watch.toxiproxy, into the configuration file.
	WriteConfig func(section string) error

	// GenFile is a file the generator writes on every run. The front calls
	// [Config.LoadEndpoints] when its modification time changes.
	GenFile string

	// LoadEndpoints returns the endpoints of the app as its source declares them.
	LoadEndpoints func() ([]Endpoint, error)

	// EndpointsChanged, when set, runs after the front applies reloaded endpoints.
	EndpointsChanged func()

	Logger *slog.Logger
}

// Front is the HTTP server of watch.toxiproxy.host. It passes each request to
// the dev server through the Toxiproxy proxy of its endpoint and applies the
// [Bandwidth] and [HTTPError] toxics itself.
type Front struct {
	conf    Config
	log     *slog.Logger
	proxies *proxies
	links   *links
	routes  atomic.Pointer[routeTable]
	toxics  atomic.Pointer[frontToxics]

	lock     sync.Mutex
	settings Settings

	// genModTime is the modification time of conf.GenFile when [New] ran.
	genModTime time.Time

	forward *httputil.ReverseProxy
	direct  *httputil.ReverseProxy

	transportsLock sync.Mutex
	transports     map[string]*http.Transport // by proxy address
}

// addrKey is the context key of the Toxiproxy proxy address of a request.
type addrKey struct{}

// replaceKey is the context key of the [replacement] of a response.
type replaceKey struct{}

// replacement is the status of the downstream [HTTPError] toxic of a request.
// atHead is set for the stream of a page, whose response does not end.
type replacement struct {
	status int
	atHead bool
}

// frontToxics maps the route pattern of an endpoint with overrides to the toxics
// the [Front] applies to it, and "" to those of every other request.
type frontToxics map[string][]frontToxic

// frontToxic is 1 toxic the [Front] applies. scope is the route pattern of
// the endpoint that overrides it, empty for a global toxic.
type frontToxic struct {
	active
	scope string
}

func newFrontToxics(s Settings) *frontToxics {
	t := frontToxics{"": scoped("", nil, effective(s.Global, nil, true))}
	for p, o := range s.Endpoints {
		t[p] = scoped(p, o, effective(s.Global, o, true))
	}
	return &t
}

// scoped returns a with the scope pattern on each toxic that o overrides.
func scoped(pattern string, o Set, a []active) []frontToxic {
	t := make([]frontToxic, len(a))
	for i, x := range a {
		t[i].active = x
		if _, ok := o[x.slot]; ok {
			t[i].scope = pattern
		}
	}
	return t
}

// toxic returns the toxic of slot that applies to the endpoint with pattern.
func (t frontToxics) toxic(pattern string, slot Slot) (_ frontToxic, ok bool) {
	toxics, ok := t[pattern]
	if !ok {
		toxics = t[""]
	}
	for _, x := range toxics {
		if x.slot == slot {
			return x, true
		}
	}
	return frontToxic{}, false
}

// status rolls the toxicity of the [HTTPError] toxic of st for the endpoint
// with pattern and returns its status, or 0 for none.
func (t frontToxics) status(pattern string, st Stream) int {
	x, ok := t.toxic(pattern, Slot{Type: HTTPError, Stream: st})
	if !ok || rand.Float32() >= x.toxic.Toxicity {
		return 0
	}
	return int(x.toxic.Values[0])
}

// statusError is the status of a downstream [HTTPError] toxic,
// which [Front.replace] passes to [Front.abort].
type statusError int

func (e statusError) Error() string {
	return "http-error toxic: status " + strconv.Itoa(int(e))
}

// writeStatus answers with code as
// [github.com/romshark/datapages/runtime/httpserve.WriteErrStatus] does.
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
		links:      newLinks(),
		settings:   conf.Settings.Clone(),
		transports: map[string]*http.Transport{},
	}
	f.routes.Store(newRouteTable(conf.Endpoints))
	f.toxics.Store(newFrontToxics(f.settings))
	if fi, err := os.Stat(conf.GenFile); err == nil {
		f.genModTime = fi.ModTime()
	}
	if err := p.apply(f.settings, patternsOf(conf.Endpoints)); err != nil {
		p.close()
		return nil, fmt.Errorf("applying watch.toxiproxy: %w", err)
	}

	errLog := slog.NewLogLogger(f.log.Handler(), slog.LevelDebug)
	f.forward = &httputil.ReverseProxy{
		Rewrite:   f.rewrite,
		Transport: roundTripFunc(f.roundTrip),
		// Buffering would hide how toxics such as slicer shape the response.
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

// rewrite keeps the Host header of the browser, which the cross-origin check
// of the generated server compares the Origin header against.
// It removes the profile cookie.
func (f *Front) rewrite(r *httputil.ProxyRequest) {
	r.SetURL(f.conf.Upstream)
	r.Out.Host = r.In.Host
	removeCookie(r.Out.Header, profileCookie)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func (f *Front) roundTrip(r *http.Request) (*http.Response, error) {
	addr, _ := r.Context().Value(addrKey{}).(string)
	return f.transport(addr).RoundTrip(r)
}

// transport returns the transport that connects to addr only.
// It disables keep-alive because Toxiproxy rolls the toxicity per connection
// and some toxics, such as limit-data, end the connection.
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
		// The dev server runs in this process and presents the certificate of
		// watch.tls, which may be self-signed.
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // see above
		},
	}
	f.transports[addr] = t
	return t
}

// replace returns the [statusError] of the [replacement] of resp, if any.
// It first reads resp to its end, which lets the app finish an action that
// flushed its head before it ran. It does not read a stream, which never ends.
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
// connection to the browser, because a 502 page would look like a response of the app.
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
		// Toxics on the reload connection would reload the page in a loop.
		f.direct.ServeHTTP(w, r)
	default:
		id, set := profile(r)
		if set {
			http.SetCookie(w, &http.Cookie{
				Name: profileCookie, Value: id, Path: "/",
				HttpOnly: true, SameSite: http.SameSiteLaxMode,
				Secure: f.conf.TLSCert != "",
			})
		}
		e := f.routes.Load().match(r)
		toxics := f.toxics.Load()
		if code := toxics.status(e.Pattern, Upstream); code != 0 {
			writeStatus(w, code)
			return
		}
		ctx := context.WithValue(r.Context(), addrKey{}, f.proxies.addr(e.Pattern))
		if code := toxics.status(e.Pattern, Downstream); code != 0 {
			ctx = context.WithValue(ctx, replaceKey{}, replacement{
				status: code, atHead: e.Kind() == KindStream,
			})
		}
		pace := func(st Stream) *pacer {
			return &pacer{
				f: f, ctx: r.Context(), pattern: e.Pattern, profile: id,
				stream: st, roll: rand.Float32(),
			}
		}
		r = r.WithContext(ctx)
		if r.Body != nil && r.Body != http.NoBody {
			r.Body = &pacedBody{ReadCloser: r.Body, p: pace(Upstream)}
		}
		f.forward.ServeHTTP(&pacedWriter{ResponseWriter: w, p: pace(Downstream)}, r)
	}
}

// Close stops the Toxiproxy proxies. [Front.Serve] calls it on return.
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
		// Open SSE streams would block Shutdown.
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

// setEndpoints routes requests by eps. It starts the proxies of new endpoints
// before a request can match them.
func (f *Front) setEndpoints(eps []Endpoint) {
	f.lock.Lock()
	defer f.lock.Unlock()
	if err := f.proxies.apply(f.settings, patternsOf(eps)); err != nil {
		f.log.Error("toxiproxy: " + err.Error())
	}
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

	// ConfigFile is the name of the configuration file, and ConfigPath its
	// absolute path or empty when unknown.
	ConfigFile, ConfigPath string

	// Version and ToxiproxyVersion are the versions of Datapages and Toxiproxy.
	Version, ToxiproxyVersion string
}

// Info returns what the settings page shows about f.
func (f *Front) Info() Info {
	file := "datapages.yaml"
	if f.conf.ConfigPath != "" {
		file = filepath.Base(f.conf.ConfigPath)
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
		ConfigPath:  f.conf.ConfigPath,

		Version:          f.conf.Version,
		ToxiproxyVersion: toxiproxyVersion(),
	}
}

// Reload replaces the presets with those of the configuration file and
// applies the active preset again, or No throttling when the file lacks it.
func (f *Front) Reload() error {
	if f.conf.ReloadSettings == nil {
		return errors.New("no configuration to reload")
	}
	s, err := f.conf.ReloadSettings()
	if err != nil {
		return err
	}
	return f.Update(func(dst *Settings) error {
		active := dst.Active
		*dst = s.Clone()
		if dst.ApplyPreset(active) != nil {
			return dst.ApplyPreset(config.PresetNoThrottling)
		}
		return nil
	})
}

// ErrUnnamed reports toxics no preset holds, which the config file can't hold either.
var ErrUnnamed = errors.New("no preset holds the toxics; create one to save them")

// Save writes the presets into the configuration file.
func (f *Front) Save() error {
	if f.conf.WriteConfig == nil {
		return errors.New("no configuration to save to")
	}
	if f.Settings().Active == "" {
		return ErrUnnamed
	}
	return f.conf.WriteConfig(f.section(0))
}

// Saved reports whether the configuration file holds the presets and a
// preset the toxics in effect. It reads the file, which the user may also have changed.
func (f *Front) Saved() bool {
	if f.conf.ReloadSettings == nil {
		return false
	}
	s, err := f.conf.ReloadSettings()
	if err != nil {
		return false
	}
	cur, patterns := f.Settings(), f.patterns()
	return cur.Active != "" && s.YAML(0, patterns) == cur.YAML(0, patterns)
}

// section returns the keys of watch.toxiproxy indented by indent spaces,
// the addresses only where the configuration file sets them.
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
func (f *Front) patterns() []string { return patternsOf(f.Endpoints()) }

func patternsOf(eps []Endpoint) []string {
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

// Update applies change to a copy of the settings, records it in the active
// preset with [Settings.record], and applies the copy to the proxies unless
// change returns an error.
func (f *Front) Update(change func(*Settings) error) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	s := f.settings.Clone()
	if err := change(&s); err != nil {
		return err
	}
	s.record()
	if err := f.proxies.apply(s, f.patterns()); err != nil {
		// [Toxic.Validate] matches the checks of Toxiproxy, which makes this
		// error a bug. Keep the toxics Toxiproxy accepted.
		f.log.Error("toxiproxy: " + err.Error())
	}
	f.settings = s
	f.toxics.Store(newFrontToxics(s))
	return nil
}

// Settings returns a copy of the settings.
func (f *Front) Settings() Settings {
	f.lock.Lock()
	defer f.lock.Unlock()
	return f.settings.Clone()
}
