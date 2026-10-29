package toxic

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func endpoints(patterns ...string) []Endpoint {
	eps := make([]Endpoint, len(patterns))
	for i, p := range patterns {
		eps[i] = Endpoint{Pattern: p, Owner: "PageX", Handler: "GET"}
	}
	return eps
}

// TestCheckEndpointKeys tests the error for an override of an unknown route,
// which suggests the pattern with {$}.
func TestCheckEndpointKeys(t *testing.T) {
	eps := endpoints("GET /", "GET /post/{slug}/{$}")
	const where = "watch.toxiproxy.endpoints"
	require.NoError(t,
		CheckEndpointKeys(where, []string{"GET /", "GET /post/{slug}/{$}"}, eps))

	err := CheckEndpointKeys(where,
		[]string{"GET /post/{slug}/", "DELETE /gone/{$}"}, eps)
	require.ErrorContains(t, err, `watch.toxiproxy.endpoints: `+
		`no endpoint has the pattern "GET /post/{slug}/"; `+
		`did you mean "GET /post/{slug}/{$}"?`)
	require.ErrorContains(t, err, `no endpoint has the pattern "DELETE /gone/{$}"`)
}

// TestRouteTableMatch tests that a request matches the endpoint the generated
// server would serve it with, and none for a redirect.
func TestRouteTableMatch(t *testing.T) {
	table := newRouteTable(endpoints(
		"GET /",
		"GET /post/{slug}/{$}",
		"GET /post/new/{$}",
		"POST /post/{slug}/like/{$}",
		"GET /static/",
	))
	for name, tc := range map[string]struct {
		method, target, want string
	}{
		"index":        {method: "GET", target: "/", want: "GET /"},
		"unknown path": {method: "GET", target: "/nope", want: "GET /"},
		"wildcard": {
			method: "GET", target: "/post/hello/", want: "GET /post/{slug}/{$}",
		},
		"more specific": {
			method: "GET", target: "/post/new/", want: "GET /post/new/{$}",
		},
		"action": {
			method: "POST", target: "/post/x/like/", want: "POST /post/{slug}/like/{$}",
		},
		"head as get": {
			method: "HEAD", target: "/post/x/", want: "GET /post/{slug}/{$}",
		},
		"assets":       {method: "GET", target: "/static/app.css", want: "GET /static/"},
		"redirect":     {method: "GET", target: "/post/hello", want: ""},
		"wrong method": {method: "PUT", target: "/post/x/like/", want: ""},
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.target, nil)
			require.Equal(t, tc.want, table.match(r).Pattern)
		})
	}
}

// TestEndpointKind tests the kind of each handler.
func TestEndpointKind(t *testing.T) {
	for handler, want := range map[string]string{
		"GET":           KindGET,
		"GETStream":     KindStream,
		"GETStreamAnon": KindStream,
		"GETExport":     KindFile,
		"assets":        KindFile,
		"POSTLike":      KindAction,
		"QUERYSearch":   KindAction,
		"DELETEItem":    KindAction,
	} {
		require.Equal(t, want, Endpoint{Handler: handler}.Kind(), handler)
	}
}

// TestPatternKind tests the kind guessed for a pattern the app does not register.
func TestPatternKind(t *testing.T) {
	for pattern, want := range map[string]string{
		"GET /gone/{$}":         KindGET,
		"GET /gone/_$/{$}":      KindStream,
		"GET /gone/_$/anon/{$}": KindStream,
		"POST /gone/{$}":        KindAction,
	} {
		require.Equal(t, want, PatternKind(pattern), pattern)
	}
}
