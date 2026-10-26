package toxic

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"

	toxiproxy "github.com/Shopify/toxiproxy/v2"
	"github.com/rs/zerolog"
)

// proxies runs 1 Toxiproxy proxy for the global toxics and 1 for each endpoint
// that overrides a toxic Toxiproxy applies. Toxiproxy applies toxics to a proxy,
// not to a request, which is why an endpoint with toxics of its own needs
// a proxy of its own.
type proxies struct {
	server   *toxiproxy.ApiServer
	upstream string

	lock     sync.Mutex
	global   *toxiproxy.Proxy
	override map[string]*toxiproxy.Proxy // by route pattern
	nextID   int

	// addrs maps the route pattern of an endpoint with a proxy of its own
	// to the address that proxy listens on. Every request reads it.
	addrs      atomic.Pointer[map[string]string]
	globalAddr string
}

// newProxies starts the global proxy, which forwards to upstream.
func newProxies(upstream string) (*proxies, error) {
	// The bandwidth toxic logs to the global zerolog logger,
	// which writes JSON to stderr next to the log of watch mode.
	zerolog.SetGlobalLevel(zerolog.Disabled)

	p := &proxies{
		server: toxiproxy.NewServer(
			toxiproxy.NewMetricsContainer(nil), zerolog.Nop(),
		),
		upstream: upstream,
		override: map[string]*toxiproxy.Proxy{},
	}
	g, err := p.start("global")
	if err != nil {
		return nil, err
	}
	p.global, p.globalAddr = g, g.Listen
	p.addrs.Store(&map[string]string{})
	return p, nil
}

// start starts a proxy on a free port of the loopback interface.
func (p *proxies) start(name string) (*toxiproxy.Proxy, error) {
	x := toxiproxy.NewProxy(p.server, name, "127.0.0.1:0", p.upstream)
	if err := p.server.Collection.Add(x, true); err != nil {
		return nil, fmt.Errorf("starting toxiproxy proxy %s: %w", name, err)
	}
	return x, nil
}

// addr returns the address of the proxy the requests of the endpoint
// with the given route pattern pass through.
func (p *proxies) addr(pattern string) string {
	if a, ok := (*p.addrs.Load())[pattern]; ok {
		return a
	}
	return p.globalAddr
}

// apply makes the toxics of every proxy those of s. It starts a proxy for an
// endpoint that gained overrides and stops that of an endpoint that lost them.
func (p *proxies) apply(s Settings) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	var errs []error
	if err := syncToxics(p.global, effective(s.Global, nil, false)); err != nil {
		errs = append(errs, fmt.Errorf("global: %w", err))
	}
	addrs := make(map[string]string, len(s.Endpoints))
	for pattern, o := range s.Endpoints {
		if !o.overridesProxy() {
			continue
		}
		x, ok := p.override[pattern]
		if !ok {
			p.nextID++
			var err error
			x, err = p.start("endpoint-" + strconv.Itoa(p.nextID))
			if err != nil {
				errs = append(errs, err)
				continue
			}
			p.override[pattern] = x
		}
		if err := syncToxics(x, effective(s.Global, o, false)); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", pattern, err))
		}
		addrs[pattern] = x.Listen
	}
	for pattern, x := range p.override {
		if _, ok := addrs[pattern]; ok {
			continue
		}
		if err := p.server.Collection.Remove(x.Name); err != nil {
			errs = append(errs,
				fmt.Errorf("stopping toxiproxy proxy %s: %w", x.Name, err))
		}
		delete(p.override, pattern)
	}
	p.addrs.Store(&addrs)
	return errors.Join(errs...)
}

// syncToxics adds, updates and removes the toxics of x until it has want.
// It updates a toxic that stays in place rather than recreating it,
// which keeps its effect on an open connection, such as an SSE stream, continuous.
func syncToxics(x *toxiproxy.Proxy, want []active) error {
	var errs []error
	wanted := make(map[Slot]bool, len(want))
	for _, a := range want {
		wanted[a.slot] = true
		var err error
		if x.Toxics.GetToxic(a.slot.Name()) != nil {
			_, err = x.Toxics.UpdateToxicJson(
				a.slot.Name(), bytes.NewReader(a.updateJSON()))
		} else {
			_, err = x.Toxics.AddToxicJson(bytes.NewReader(a.createJSON()))
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", a.slot.Name(), err))
		}
	}
	for _, s := range Slots {
		if wanted[s] || x.Toxics.GetToxic(s.Name()) == nil {
			continue
		}
		if err := x.Toxics.RemoveToxic(context.Background(), s.Name()); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", s.Name(), err))
		}
	}
	return errors.Join(errs...)
}

// close stops every proxy and closes their connections.
func (p *proxies) close() {
	p.lock.Lock()
	defer p.lock.Unlock()
	_ = p.server.Collection.Clear()
}
