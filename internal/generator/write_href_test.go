package generator

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/parser"
	"github.com/romshark/datapages/internal/routepattern"
)

// TestReservedNamesHaveAFixtureRoute tests that every name in [pathParamReserved]
// is written as a route wildcard by the fixture named after the case.
//
// The fixture provokes the collision by naming a wildcard after something the
// URL writers resolve. Nothing else ties the two together: an entry added here
// without a route there leaves generated code nobody compiles under that name,
// and the fixture stays green while covering one name less. This closes that
// loop in the one direction a test can.
//
// The other direction has no anchor. The names [newHrefLocals] picks are
// literals in that function rather than a list, and a rename there leaves both
// the fixture and internal/acceptance/hreflocals passing while they cover nothing.
// See the comment on [hrefLocals].
func TestReservedNamesHaveAFixtureRoute(t *testing.T) {
	t.Parallel()

	const fixture = "routevar_shadows_import"
	m, errs := parser.Parse(
		filepath.Join("..", "parser", "testdata", fixture),
	)
	for _, err := range errs.All() {
		t.Errorf("parser: %v", err)
	}
	require.Zero(t, errs.Len())
	require.NotNil(t, m)

	routed := map[string]bool{}
	collect := func(route string) {
		for v := range routepattern.Vars(route) {
			routed[v] = true
		}
	}
	for _, h := range m.Actions {
		collect(h.Route)
	}
	for _, p := range m.Pages {
		collect(p.Route)
		for _, h := range p.Actions {
			collect(h.Route)
		}
	}

	for name := range pathParamReserved {
		require.True(t, routed[name],
			"pathParamReserved holds %q and no route in the %s fixture names "+
				"a wildcard after it, hence no build ever writes a parameter "+
				"that shadows it", name, fixture)
	}
}
