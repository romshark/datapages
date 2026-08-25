// Wires the routing case into the shared contract suite.

package acceptance_test

import (
	"testing"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/contract"
	"github.com/romshark/datapages/internal/acceptance/routing/app"
	"github.com/romshark/datapages/internal/acceptance/routing/app/datapagesgen"
	"github.com/romshark/datapages/internal/acceptance/routing/app/datapagesgen/href"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

// TestContract must not use t.Parallel() because the generated Init sets the
// package-level logger of the href package, which contract.Run's ExternalHref test reads.
func TestContract(t *testing.T) {
	contract.Run(t, contract.Case{
		NewServer: func(t *testing.T, opts ...any) contract.Server {
			t.Helper()
			return mustNewServer(t, &app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer),
				contract.Options[datapages.ServerOption](opts)...)
		},
		WithMiddleware: contract.OptVariadic(datapages.WithMiddleware),
		WithDatastarJS: contract.Opt(datapages.WithDatastarJS),
		WithHTTPServer: contract.Opt(datapages.WithHTTPServer),
		WithLogger:     contract.Opt(datapages.WithLogger),
		StreamSubjects: datapagesgen.MessageBrokerStreamSubjects,
		HrefExternal:   href.External,
		HrefSetLogger:  href.SetLogger,
		Links: []string{
			href.PageIndex(),
			href.PagePath("a", 1, 2, 3.5, true),
			href.PageInts(1, 2, 3, 4, 5, 6, 7),
			href.PageQuery(href.QueryPageQuery{Term: "x", Limit: 1}),
			href.PageMixed("acme", 7, href.QueryPageMixed{Tab: "open"}),
			href.PageConflict(1, 2, "three"),
			href.PageTitled("welcome"),
			href.PageReflect(href.QueryPageReflect{Term: "x", Page: 1}),
		},
	})
}
