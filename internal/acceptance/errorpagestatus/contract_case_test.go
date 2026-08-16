// Wires the errorpagestatus case into the shared contract suite.

package acceptance_test

import (
	"testing"

	"github.com/romshark/datapages/internal/acceptance/contract"
	"github.com/romshark/datapages/internal/acceptance/errorpagestatus/app"
	"github.com/romshark/datapages/internal/acceptance/errorpagestatus/datapagesgen"
	"github.com/romshark/datapages/internal/acceptance/errorpagestatus/datapagesgen/href"
	"github.com/romshark/datapages/modules/msgbroker/inmem"
)

func TestContract(t *testing.T) {
	contract.Run(t, contract.Case{
		NewServer: func(t *testing.T, opts ...any) contract.Server {
			t.Helper()
			return datapagesgen.NewServer(&app.App{}, inmem.New(8),
				contract.Options[datapagesgen.ServerOption](opts)...)
		},
		WithAssets:     contract.Opt(datapagesgen.WithAssets),
		WithMiddleware: contract.Opt(datapagesgen.WithMiddleware),
		WithDatastarJS: contract.Opt(datapagesgen.WithDatastarJS),
		WithHTTPServer: contract.Opt(datapagesgen.WithHTTPServer),
		WithLogger:     contract.Opt(datapagesgen.WithLogger),
		StreamSubjects: datapagesgen.MessageBrokerStreamSubjects,
		HrefExternal:   href.External,
		HrefSetLogger:  href.SetLogger,
		Links:          []string{href.PageIndex(), href.PageError404()},
	})
}
