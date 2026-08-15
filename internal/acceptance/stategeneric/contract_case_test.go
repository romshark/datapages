// Wires the generic per-page state case into the shared contract suite.

package acceptance_test

import (
	"crypto/sha256"
	"testing"

	"github.com/romshark/datapages/internal/acceptance/contract"
	"github.com/romshark/datapages/internal/acceptance/stategeneric/app"
	"github.com/romshark/datapages/internal/acceptance/stategeneric/datapagesgen"
	"github.com/romshark/datapages/internal/acceptance/stategeneric/datapagesgen/action"
	"github.com/romshark/datapages/internal/acceptance/stategeneric/datapagesgen/href"
	"github.com/romshark/datapages/modules/msgbroker/inmem"
)

func TestContract(t *testing.T) {
	contract.Run(t, contract.Case{
		NewServer: func(t *testing.T, opts ...any) contract.Server {
			t.Helper()
			key := sha256.Sum256([]byte("acceptance"))
			opts = append(opts, datapagesgen.WithStateConfig(
				datapagesgen.StateConfig{
					HMACKey: key[:],
				},
			))
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
		Links: []string{
			href.PageIndex(),
			href.PageCount(),
			href.PageLabel(),
			href.PageEmbedOnly(),
		},
		Actions: []string{
			action.POSTPageCountBump(),
			action.POSTPageLabelSet(),
		},
		SignalActions: []string{action.POSTPageLabelSet()},
		// optionedAction carries every option at once.
		// The keys and their order are asserted by the contract suite.
		OptionedAction: action.POSTPageCountBump(
			action.WithBefore("$busy = true"),
			action.WithContentType(action.ContentTypeForm),
			action.WithSelector("#it's"),
			action.WithHeaders(map[string]string{
				"X-Trace": "abc",
				// A value with a quote in it must not close the string it sits in,
				// and a second header must be separated from the first.
				"X-Note": "it's here",
			}),
			action.WithFilterSignals("name", "secret"),
			action.WithOpenWhenHidden(true),
			action.WithPayload("{id: $id}"),
			action.WithRetry(action.RetryAlways),
			action.WithRetryInterval(500),
			action.WithRetryScaler(1.5),
			action.WithRetryMaxWaitMs(30000),
			action.WithRetryMaxCount(3),
			action.WithRequestCancellation(action.RequestCancellationDisabled),
			action.WithAfter("$busy = false"),
		),
		// The instance id a page load hands out belongs to that page.
		// The contract therefore loads the page whose stream it opens.
		Index:          "/count/",
		StreamPath:     "/count/_$/",
		DispatchAction: action.POSTPageCountBump(),
		StateAction:    action.POSTPageCountBump(),
	})
}
