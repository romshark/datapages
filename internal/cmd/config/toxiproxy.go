package config

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// TemplierEventsPath is the reload endpoint of the script the dev server
// injects into every page. The Toxiproxy front forwards it without toxics.
const TemplierEventsPath = "/__templier/events"

// WatchToxiproxy configures the proxy in front of the dev server, which
// applies toxics to the requests sent to Host. Without the section, watch mode
// runs the proxy with the defaults.
type WatchToxiproxy struct {
	// Disabled turns the proxy off.
	Disabled bool `yaml:"disabled"`

	// Host is the address of the proxy, as in "localhost:9090". Empty means
	// the port after the dev server, "localhost:7332" by default.
	Host string `yaml:"host"`

	// UIHost is the address of the settings page. Empty means the port after Host.
	UIHost string `yaml:"ui-host"`

	// Presets are named sets of toxics and endpoint overrides the settings
	// page applies in 1 step. It lists them between the built-in
	// [PresetNoThrottling], which watch starts with, and [PresetUnreachable].
	Presets []ToxiproxyPreset `yaml:"presets"`
}

// Built-in presets. [WatchToxiproxy.Presets] cannot use their names.
const (
	PresetNoThrottling = "No throttling"
	PresetUnreachable  = "Unreachable"
)

// ToxiproxyPreset is a named set of toxics and endpoint overrides.
type ToxiproxyPreset struct {
	Name string `yaml:"name"`

	// Toxics apply to every endpoint that does not override them.
	Toxics Toxics `yaml:"toxics"`

	// Endpoints maps the route pattern the generated server registers, as in
	// "GET /post/{slug}/{$}", to toxics that replace the global ones of the
	// same type and stream.
	Endpoints map[EndpointPattern]Toxics `yaml:"endpoints"`
}

func (w *WatchToxiproxy) Validate() error {
	names := make(map[string]bool, len(w.Presets))
	for _, p := range w.Presets {
		if strings.TrimSpace(p.Name) == "" {
			return errors.New("a preset has no name")
		}
		if p.Name == PresetNoThrottling || p.Name == PresetUnreachable {
			return fmt.Errorf("the preset %q is built in", p.Name)
		}
		if names[p.Name] {
			return fmt.Errorf("2 presets are named %q", p.Name)
		}
		names[p.Name] = true
	}
	for _, h := range []struct{ name, addr string }{
		{name: "host", addr: w.Host},
		{name: "ui-host", addr: w.UIHost},
	} {
		if h.addr == "" {
			continue
		}
		if _, _, err := net.SplitHostPort(h.addr); err != nil {
			return fmt.Errorf("invalid %s %q: %w", h.name, h.addr, err)
		}
	}
	if w.Host != "" && w.Host == w.UIHost {
		return fmt.Errorf("host and ui-host are both %q", w.Host)
	}
	return nil
}

// EndpointPattern is the [net/http.ServeMux] pattern of 1 endpoint,
// method included, as in "GET /post/{slug}/{$}".
type EndpointPattern string

func (p EndpointPattern) Validate() error {
	method, _, ok := strings.Cut(string(p), " ")
	if !ok || method == "" || strings.HasPrefix(method, "/") {
		return fmt.Errorf("endpoint %q must start with a method, "+
			`as in "GET /post/{slug}/{$}"`, string(p))
	}
	if err := checkMuxPattern(string(p)); err != nil {
		return fmt.Errorf("endpoint %q: %w", string(p), err)
	}
	return nil
}

// checkMuxPattern reports the panic [net/http.ServeMux.Handle] raises for pattern.
func checkMuxPattern(pattern string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("invalid route pattern: %v", r)
		}
	}()
	http.NewServeMux().Handle(pattern, http.NotFoundHandler())
	return nil
}

// Toxics are the toxics of the global scope or of 1 endpoint. Each type has
// its own downstream (response) and upstream (request) toxic.
type Toxics struct {
	Latency   *ToxicStreams[ToxicLatency]   `yaml:"latency"`
	Bandwidth *ToxicStreams[ToxicBandwidth] `yaml:"bandwidth"`
	SlowClose *ToxicStreams[ToxicSlowClose] `yaml:"slow-close"`
	Timeout   *ToxicStreams[ToxicTimeout]   `yaml:"timeout"`
	ResetPeer *ToxicStreams[ToxicResetPeer] `yaml:"reset-peer"`
	Slicer    *ToxicStreams[ToxicSlicer]    `yaml:"slicer"`
	LimitData *ToxicStreams[ToxicLimitData] `yaml:"limit-data"`
	HTTPError *ToxicStreams[ToxicHTTPError] `yaml:"http-error"`
}

// ToxicStreams holds a toxic per direction. Nil means none, and on an endpoint
// it means the global toxic applies.
type ToxicStreams[T any] struct {
	// Downstream acts on the response.
	Downstream *T `yaml:"downstream"`

	// Upstream acts on the request.
	Upstream *T `yaml:"upstream"`
}

// ToxicBase holds the settings every toxic type has.
type ToxicBase struct {
	// Disabled turns the toxic off and makes its other settings optional. On
	// an endpoint it turns off the global toxic of the same type and stream.
	Disabled bool `yaml:"disabled"`

	// Toxicity is the probability, from 0 to 1, that the toxic applies to
	// a connection. Each request opens its own connection. Nil means 1.
	Toxicity *float32 `yaml:"toxicity"`
}

func (b ToxicBase) Validate() error {
	if b.Toxicity != nil && (*b.Toxicity < 0 || *b.Toxicity > 1) {
		return fmt.Errorf("toxicity %v must be from 0 to 1", *b.Toxicity)
	}
	return nil
}

// ToxicLatency delays the data by Latency, plus or minus up to Jitter.
type ToxicLatency struct {
	ToxicBase `yaml:",inline"`
	Latency   time.Duration `yaml:"latency"`
	Jitter    time.Duration `yaml:"jitter"`
}

func (t ToxicLatency) Validate() error {
	if t.Disabled {
		return nil
	}
	return errors.Join(
		checkMillis("latency", t.Latency),
		checkMillis("jitter", t.Jitter),
	)
}

// ToxicBandwidth limits the data to Rate KB/s, shared by the requests of 1
// browser. An endpoint that overrides it has a rate of its own. The proxy
// applies it, not Toxiproxy.
type ToxicBandwidth struct {
	ToxicBase `yaml:",inline"`
	Rate      int64 `yaml:"rate"`
}

func (t ToxicBandwidth) Validate() error {
	if !t.Disabled && t.Rate < 1 {
		return fmt.Errorf("rate %d must be at least 1 KB/s", t.Rate)
	}
	return nil
}

// ToxicSlowClose delays closing the connection by Delay.
type ToxicSlowClose struct {
	ToxicBase `yaml:",inline"`
	Delay     time.Duration `yaml:"delay"`
}

func (t ToxicSlowClose) Validate() error {
	if t.Disabled {
		return nil
	}
	return checkMillis("delay", t.Delay)
}

// ToxicTimeout stops all data and closes the connection after Timeout.
// A zero Timeout holds the connection open until the client gives up.
type ToxicTimeout struct {
	ToxicBase `yaml:",inline"`
	Timeout   time.Duration `yaml:"timeout"`
}

func (t ToxicTimeout) Validate() error {
	if t.Disabled {
		return nil
	}
	return checkMillis("timeout", t.Timeout)
}

// ToxicResetPeer resets the connection with a TCP RST once data arrives, after Timeout.
type ToxicResetPeer struct {
	ToxicBase `yaml:",inline"`
	Timeout   time.Duration `yaml:"timeout"`
}

func (t ToxicResetPeer) Validate() error {
	if t.Disabled {
		return nil
	}
	return checkMillis("timeout", t.Timeout)
}

// ToxicSlicer splits the data into chunks of AverageSize bytes,
// plus or minus up to SizeVariation, and delays each chunk by Delay.
type ToxicSlicer struct {
	ToxicBase     `yaml:",inline"`
	AverageSize   int64         `yaml:"average-size"`
	SizeVariation int64         `yaml:"size-variation"`
	Delay         time.Duration `yaml:"delay"`
}

func (t ToxicSlicer) Validate() error {
	if t.Disabled {
		return nil
	}
	var errs []error
	if t.AverageSize < 1 {
		errs = append(errs,
			fmt.Errorf("average-size %d must be at least 1", t.AverageSize))
	}
	if t.SizeVariation < 0 || t.SizeVariation > t.AverageSize {
		errs = append(errs, fmt.Errorf(
			"size-variation %d must be from 0 to average-size", t.SizeVariation,
		))
	}
	// Toxiproxy takes the slicer delay in microseconds.
	if t.Delay < 0 || t.Delay%time.Microsecond != 0 {
		errs = append(errs, fmt.Errorf(
			"delay %s must be 0 or more whole microseconds", t.Delay,
		))
	}
	return errors.Join(errs...)
}

// ToxicLimitData closes the connection after Bytes bytes.
type ToxicLimitData struct {
	ToxicBase `yaml:",inline"`
	Bytes     int64 `yaml:"bytes"`
}

func (t ToxicLimitData) Validate() error {
	if !t.Disabled && t.Bytes < 0 {
		return fmt.Errorf("bytes %d must be 0 or more", t.Bytes)
	}
	return nil
}

// ToxicHTTPError answers with Status and its standard text. Upstream, the
// request never reaches the app. Downstream, the app answers it and the client
// gets Status instead. The proxy applies it, not Toxiproxy.
type ToxicHTTPError struct {
	ToxicBase `yaml:",inline"`
	Status    int64 `yaml:"status"`
}

func (t ToxicHTTPError) Validate() error {
	if !t.Disabled && (t.Status < 400 || t.Status > 599) {
		return fmt.Errorf("status %d must be from 400 to 599", t.Status)
	}
	return nil
}

// checkMillis reports a duration other than 0 or more whole milliseconds,
// which Toxiproxy requires.
func checkMillis(name string, d time.Duration) error {
	if d < 0 || d%time.Millisecond != 0 {
		return fmt.Errorf("%s %s must be 0 or more whole milliseconds", name, d)
	}
	return nil
}
