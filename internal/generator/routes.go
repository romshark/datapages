package generator

import (
	"strings"

	"github.com/romshark/datapages/internal/parser/model"
	"github.com/romshark/datapages/internal/routepattern"
)

// Route is 1 handler the generated setupHandlers registers on the mux of the server.
type Route struct {
	// Pattern is the [net/http.ServeMux] pattern, method included,
	// as in "GET /post/{slug}/{$}".
	Pattern string

	// Owner is the type the handler belongs to: a page type,
	// or "App" for an app-level action.
	Owner string

	// Handler is the method of the generated handlers type that serves the route:
	// GET, GETStream, GETStreamAnon, or the method and name of an action, as in POSTLike.
	Handler string
}

// Routes lists the routes setupHandlers registers for m, in registration order.
// The assets route is not among them, [httpserve.Core.Build] registers it.
func Routes(m *model.App) []Route {
	eventByName := make(map[string]*model.Event, len(m.Events))
	for _, e := range m.Events {
		eventByName[e.TypeName] = e
	}

	var routes []Route
	for _, p := range m.Pages {
		if p.GET == nil || p.GET.OutputBody == nil {
			continue
		}

		var pattern string
		switch {
		case p.PageSpecialization == model.PageTypeIndex:
			pattern = "GET /"
		case routepattern.EndsInWildcard(p.Route):
			// A {name...} wildcard runs to the end of the path already.
			// Marking the end after it puts the wildcard in the middle,
			// which is a pattern net/http will not parse.
			pattern = "GET " + p.Route
		default:
			pattern = "GET " + routepattern.WithTrailingSlash(p.Route) + "{$}"
		}
		routes = append(routes, Route{
			Pattern: pattern, Owner: p.TypeName, Handler: "GET",
		})

		if pageHasStream(p) {
			streamPath := routepattern.StreamPath(p.Route)
			routes = append(routes, Route{
				Pattern: "GET " + streamPath + "{$}",
				Owner:   p.TypeName,
				Handler: "GETStream",
			})
			if pageHasAnonStream(p, eventByName) {
				routes = append(routes, Route{
					Pattern: "GET " + streamPath + "anon/{$}",
					Owner:   p.TypeName,
					Handler: "GETStreamAnon",
				})
			}
		}
	}

	for _, h := range m.Actions {
		routes = append(routes, actionRoute("App", h))
	}
	for _, p := range m.Pages {
		for _, h := range p.Actions {
			routes = append(routes, actionRoute(p.TypeName, h))
		}
	}
	return routes
}

func actionRoute(owner string, h *model.Handler) Route {
	method := strings.ToUpper(h.HTTPMethod)
	return Route{
		Pattern: method + " " + routepattern.WithTrailingSlash(h.Route) + "{$}",
		Owner:   owner,
		Handler: method + h.Name,
	}
}
