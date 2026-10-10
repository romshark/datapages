// Wires the files case into the shared contract suite.

package acceptance_test

import (
	"testing"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/contract"
	"github.com/romshark/datapages/internal/acceptance/files/app"
	"github.com/romshark/datapages/internal/acceptance/files/app/datapagesgen"
	"github.com/romshark/datapages/internal/acceptance/files/app/datapagesgen/action"
	"github.com/romshark/datapages/internal/acceptance/files/app/datapagesgen/href"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

// TestContract must not use t.Parallel() because the generated Init
// sets the package-level logger of the href package,
// which [contract.Run]'s ExternalHref test reads.
func TestContract(t *testing.T) {
	contract.Run(t, contract.Case{
		NewServer: func(t *testing.T, opts ...any) contract.Server {
			t.Helper()
			return mustNewServer(t, &app.App{},
				inmem.New(messaging.DefaultBrokerChanBuffer),
				contract.Options[datapages.ServerOption](opts)...)
		},
		WithMiddleware: contract.OptVariadic(datapages.WithMiddleware),
		WithDatastarJS: contract.Opt(datapages.WithDatastarJS),
		WithBuildID:    contract.Opt(datapages.WithBuildID),
		WithHTTPServer: contract.Opt(datapages.WithHTTPServer),
		WithLogger:     contract.Opt(datapages.WithLogger),
		StreamSubjects: datapagesgen.MessageBrokerStreamSubjects,
		HrefExternal:   href.External,
		HrefSetLogger:  href.SetLogger,
		Links: []string{
			href.PageIndex(),
			href.PageDoc("x"),
			href.PageDoc.Export("x"),
			href.App.File("a.txt", href.App.FileQuery("", false)),
		},
		Actions: []string{
			action.App.Upper.POST(),
		},
	})
}
