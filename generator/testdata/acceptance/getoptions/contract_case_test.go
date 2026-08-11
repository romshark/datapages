// Wires the get-options case into the shared contract suite.

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
		href.PageMaybe(href.QueryPageMaybe{}),
		href.PageBackground(),
		href.PageNoRefresh(),
	},
}
