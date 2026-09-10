// Wires the hreflocals case into the shared contract suite.

package acceptance_test

import (
	"testing"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/contract"
	"github.com/romshark/datapages/internal/acceptance/hreflocals/app"
	"github.com/romshark/datapages/internal/acceptance/hreflocals/app/datapagesgen"
	"github.com/romshark/datapages/internal/acceptance/hreflocals/app/datapagesgen/action"
	"github.com/romshark/datapages/internal/acceptance/hreflocals/app/datapagesgen/href"
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
			href.PageItem(true),
			href.PageMix(1, 2, "three", href.QueryPageMix{AnyQuery: "yes", Page: 4}),
			href.PageTags(href.QueryPageTags{PageSize: 25, Term: "go"}),
			href.PageParams("a", "b", href.QueryPageParams{Term: "x"}),
			href.PageLocals("1", "2", "3", "4", "5"),
			href.PageImports("one", "two", 3, app.Slug("FOUR")),
			href.PageExpr("seven"),
		},
		// Every action of the case: the assertion requests each and fails on
		// one the router does not serve by that method.
		Actions: []string{
			action.PageTags.Select.POST(action.PageTags.Select.POSTQuery(7)),
			action.PageMix.Store.POST(1, 2, "three",
				action.PageMix.Store.POSTQuery("yes")),
			action.PageParams.Save.POST("a", "b"),
			action.PageLocals.Save.POST("1", "2", "3", "4", "5"),
			action.PageImports.Save.POST("one", "two", 3, app.Slug("FOUR"),
				action.PageImports.Save.POSTQuery("x")),
			action.PageExpr.Run.POST("seven"),
		},
	})
}
