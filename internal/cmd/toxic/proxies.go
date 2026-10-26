package toxic

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"

	toxiproxy "github.com/Shopify/toxiproxy/v2"
	"github.com/rs/zerolog"
)

// proxies runs 1 Toxiproxy proxy for each endpoint of the app and 1, the
// global one, for the requests that match none. Toxiproxy applies toxics to
// a proxy, not to a request, which is why an endpoint with toxics of its own
// needs a proxy of its own. Each endpoint has one from the start, which applies
// a change of its overrides to its open connections, such as an SSE stream,
// in place. A proxy started for the first override would miss the connections
// open on the global proxy, and stopping it with the last would close its own.
type proxies struct {
	server   *toxiproxy.ApiServer
	upstream string

	lock      sync.Mutex
	global    *proxy
	endpoints map[string]*proxy // by route pattern
	nextID    int

	// addrs maps the route pattern of an endpoint to the address its proxy
	// listens on. Every request reads it.
	addrs      atomic.Pointer[map[string]string]
	globalAddr string
}

// proxy is 1 Toxiproxy proxy and the toxics [proxy.sync] gave it last.
type proxy struct {
	*toxiproxy.Proxy
	applied []active
	synced  bool
}

// sync gives x the toxics want, unless it has them. Updating a toxic that
// did not change would interrupt it on every open connection.
func (x *proxy) sync(want []active) error {
	if x.synced && slices.Equal(x.applied, want) {
		return nil
	}
	err := syncToxics(x.Proxy, want)
	x.applied, x.synced = want, err == nil
	return err
}

// newProxies starts the global proxy, which forwards to upstream.
func newProxies(upstream string) (*proxies, error) {
	// Toxiproxy logs the updates of toxics to the global zerolog logger,
	// which writes JSON to stderr next to the log of watch mode.
	zerolog.SetGlobalLevel(zerolog.Disabled)

	p := &proxies{
		server: toxiproxy.NewServer(
			toxiproxy.NewMetricsContainer(nil), zerolog.Nop(),
		),
		upstream:  upstream,
		endpoints: map[string]*proxy{},
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
func (p *proxies) start(name string) (*proxy, error) {
	x := toxiproxy.NewProxy(p.server, name, "127.0.0.1:0", p.upstream)
	if err := p.server.Collection.Add(x, true); err != nil {
		return nil, fmt.Errorf("starting toxiproxy proxy %s: %w", name, err)
	}
	return &proxy{Proxy: x}, nil
}

// addr returns the address of the proxy the requests of the endpoint
// with the given route pattern pass through.
func (p *proxies) addr(pattern string) string {
	if a, ok := (*p.addrs.Load())[pattern]; ok {
		return a
	}
	return p.globalAddr
}

// apply gives the global proxy and the proxy of each endpoint pattern in
// patterns the toxics of s. It starts the proxy of a pattern new to patterns
// and stops that of a pattern gone from it, whose endpoint the app no longer has.
func (p *proxies) apply(s Settings, patterns []string) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	var errs []error
	if err := p.global.sync(effective(s.Global, nil, false)); err != nil {
		errs = append(errs, fmt.Errorf("global: %w", err))
	}
	addrs := make(map[string]string, len(patterns))
	for _, pattern := range patterns {
		x, ok := p.endpoints[pattern]
		if !ok {
			p.nextID++
			var err error
			x, err = p.start("endpoint-" + strconv.Itoa(p.nextID))
			if err != nil {
				errs = append(errs, err)
				continue
			}
			p.endpoints[pattern] = x
		}
		if err := x.sync(effective(s.Global, s.Endpoints[pattern], false)); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", pattern, err))
		}
		addrs[pattern] = x.Listen
	}
	for pattern, x := range p.endpoints {
		if _, ok := addrs[pattern]; ok {
			continue
		}
		if err := p.server.Collection.Remove(x.Name); err != nil {
			errs = append(errs,
				fmt.Errorf("stopping toxiproxy proxy %s: %w", x.Name, err))
		}
		delete(p.endpoints, pattern)
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
				a.slot.Name(), bytes.NewReader(a.updateJSON()),
			)
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
