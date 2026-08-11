// Wires the routing case into the shared contract suite.

package acceptance_test

import (
	"testing"

	"dpacceptance/app"
	"dpacceptance/datapagesgen"
	"dpacceptance/datapagesgen/href"

	"github.com/romshark/datapages/modules/msgbroker/inmem"
)

var contractCase = contract{
	newServer: func(
		t *testing.T, opts ...datapagesgen.ServerOption,
	) *datapagesgen.Server {
		t.Helper()
		return datapagesgen.NewServer(&app.App{}, inmem.New(8), opts...)
	},
	links: []string{
		href.PageIndex(),
		href.PagePath("a", 1, 2, 3.5, true),
		href.PageInts(1, 2, 3, 4, 5, 6, 7),
		href.PageQuery(href.QueryPageQuery{Term: "x", Limit: 1}),
		href.PageMixed("acme", 7, href.QueryPageMixed{Tab: "open"}),
		href.PageConflict(1, 2, "three"),
		href.PageTitled("welcome"),
		href.PageReflect(href.QueryPageReflect{Term: "x", Page: 1}),
	},
}
