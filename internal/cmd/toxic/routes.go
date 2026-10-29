package toxic

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/romshark/datapages/internal/generator"
	"github.com/romshark/datapages/internal/parser/model"
)

// Endpoint is 1 route the generated server registers on its mux.
type Endpoint struct {
	// Pattern is the [net/http.ServeMux] pattern, method included.
	Pattern string

	// Owner is the page type, "App" for an app-level action,
	// or empty for the assets.
	Owner string

	// Handler is the method of the generated handlers that serves the route,
	// as in GET, GETStream or POSTLike. It is "assets" for the assets.
	Handler string
}

// Endpoints lists the routes the generated server of m registers, in
// registration order, the assets last.
func Endpoints(m *model.App) []Endpoint {
	if m == nil {
		return nil
	}
	routes := generator.Routes(m)
	eps := make([]Endpoint, 0, len(routes)+1)
	for _, r := range routes {
		eps = append(eps, Endpoint{
			Pattern: r.Pattern, Owner: r.Owner, Handler: r.Handler,
		})
	}
	if m.Assets.URLPrefix != "" {
		eps = append(eps, Endpoint{
			Pattern: "GET " + m.Assets.URLPrefix,
			Handler: "assets",
		})
	}
	return eps
}

// Kinds of endpoints, which the settings page filters by.
const (
	KindGET    = "get"    // the GET handler of a page
	KindStream = "stream" // the SSE stream of a page
	KindAction = "action" // an action other than a GET action
	KindFile   = "file"   // a GET action, which returns a file, or the assets
)

// Kind returns the kind of e.
func (e Endpoint) Kind() string {
	switch {
	case e.Handler == "GET":
		return KindGET
	case e.Handler == "GETStream" || e.Handler == "GETStreamAnon":
		return KindStream
	case e.Handler == "assets" || strings.HasPrefix(e.Handler, "GET"):
		return KindFile
	}
	return KindAction
}

// PatternKind guesses the kind of an endpoint from its pattern alone,
// for a pattern the app does not register.
func PatternKind(pattern string) string {
	method, _, _ := strings.Cut(pattern, " ")
	switch {
	case strings.Contains(pattern, "/_$/"):
		return KindStream
	case method == "GET":
		return KindGET
	}
	return KindAction
}

// Name returns the handler that serves e, as in PagePost.GET.
func (e Endpoint) Name() string {
	if e.Owner == "" {
		return e.Handler
	}
	return e.Owner + "." + e.Handler
}

// routeTable matches a request to its endpoint as the mux of the generated server does.
type routeTable struct {
	endpoints []Endpoint
	mux       *http.ServeMux
}

// endpointHandler marks the endpoint of a pattern in [routeTable.mux].
// Its type tells a match from a redirect, for which [net/http.ServeMux.Handler]
// returns a path in place of a pattern.
type endpointHandler struct{ Endpoint }

func (endpointHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}

func newRouteTable(eps []Endpoint) *routeTable {
	t := &routeTable{endpoints: eps, mux: http.NewServeMux()}
	for _, e := range eps {
		// The generated server fails at startup on a pattern that panics here.
		func() {
			defer func() { _ = recover() }()
			t.mux.Handle(e.Pattern, endpointHandler{e})
		}()
	}
	return t
}

// match returns the endpoint of r, or the zero Endpoint for none.
func (t *routeTable) match(r *http.Request) Endpoint {
	h, _ := t.mux.Handler(r)
	if e, ok := h.(endpointHandler); ok {
		return e.Endpoint
	}
	return Endpoint{}
}

// CheckEndpointKeys reports each key that names no endpoint in eps.
// where is the YAML path of the keys, as in "watch.toxiproxy.endpoints".
func CheckEndpointKeys(where string, keys []string, eps []Endpoint) error {
	var errs []error
	for _, k := range keys {
		if err := checkEndpointKey(where, k, eps); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func checkEndpointKey(where, key string, eps []Endpoint) error {
	for _, e := range eps {
		if e.Pattern == key {
			return nil
		}
	}
	err := fmt.Errorf("%s: no endpoint has the pattern %q", where, key)
	// The app declares its routes without the {$} the generator adds.
	trimmed := strings.TrimSuffix(key, "{$}")
	for _, e := range eps {
		if strings.TrimSuffix(e.Pattern, "{$}") == trimmed {
			return fmt.Errorf("%w; did you mean %q?", err, e.Pattern)
		}
	}
	return err
}
